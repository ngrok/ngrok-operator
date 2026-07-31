package compute

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	ngrok "github.com/ngrok/ngrok-api-go/v7"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// fakeShipServer is a scriptable httptest implementation of the ship runner
// protocol that records every request it receives.
type fakeShipServer struct {
	t *testing.T

	mu            sync.Mutex
	registerCalls []map[string]any
	statusCalls   []statusCall

	// registerStatus, when non-zero, makes register fail with that HTTP status.
	registerStatus int
	// registerRunnerID is the runner ID returned by successful registrations.
	registerRunnerID string

	// statusHandler produces the reply for a status call; when nil, an empty
	// replica set is returned.
	statusHandler func(call statusCall) (int, RunnerStatusResponse)
}

type statusCall struct {
	RunnerID string
	Body     map[string]any
}

func (s *fakeShipServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	require.Equal(s.t, "Bearer test-key", r.Header.Get("Authorization"))

	rest, ok := strings.CutPrefix(r.URL.Path, "/v1/runners/")
	require.True(s.t, ok, "unexpected path %s", r.URL.Path)

	switch {
	case rest == "register" && r.Method == http.MethodPost:
		var body map[string]any
		require.NoError(s.t, json.NewDecoder(r.Body).Decode(&body))
		s.mu.Lock()
		s.registerCalls = append(s.registerCalls, body)
		status := s.registerStatus
		runnerID := s.registerRunnerID
		s.mu.Unlock()
		if status != 0 {
			http.Error(w, "registration rejected", status)
			return
		}
		writeTestJSON(s.t, w, map[string]string{"runner_id": runnerID})
	case strings.HasSuffix(rest, "/status") && r.Method == http.MethodPut:
		var body map[string]any
		require.NoError(s.t, json.NewDecoder(r.Body).Decode(&body))
		call := statusCall{RunnerID: strings.TrimSuffix(rest, "/status"), Body: body}
		s.mu.Lock()
		s.statusCalls = append(s.statusCalls, call)
		handler := s.statusHandler
		s.mu.Unlock()
		if handler == nil {
			writeTestJSON(s.t, w, RunnerStatusResponse{Replicas: []computeReplica{}})
			return
		}
		status, resp := handler(call)
		if status != http.StatusOK {
			http.Error(w, "status rejected", status)
			return
		}
		writeTestJSON(s.t, w, resp)
	case strings.HasSuffix(rest, "/decommission") && r.Method == http.MethodPost:
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(v))
}

func newRunnerProtocolTestHarness(t *testing.T, server *fakeShipServer) (*AppReplicaPoller, *RunnerClient) {
	t.Helper()

	ts := httptest.NewServer(server)
	t.Cleanup(ts.Close)

	runnerClient := &RunnerClient{
		NgrokBaseClient: ngrok.NewBaseClient(ngrok.NewClientConfig("test-key")),
		ComputeBaseURL:  ts.URL,
		JoinKey:         "pool-join-key",
		Version:         "v1.2.3",
	}

	poller := newAppReplicaPollerTestHarness(t)
	poller.RunnerAPI = runnerClient
	poller.ComputeMeta = ComputeMetadata{SchedulerProps: map[string]any{"zone": "us-test-1"}}
	return poller, runnerClient
}

func (s *fakeShipServer) statusBodies() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	bodies := make([]map[string]any, len(s.statusCalls))
	for i, call := range s.statusCalls {
		bodies[i] = call.Body
	}
	return bodies
}

func TestRunnerRegisterAndStatusHappyPath(t *testing.T) {
	ctx := context.Background()
	server := &fakeShipServer{t: t, registerRunnerID: "run_123"}
	server.statusHandler = func(statusCall) (int, RunnerStatusResponse) {
		return http.StatusOK, RunnerStatusResponse{
			Replicas: []computeReplica{{
				ID:              "dplyrep_UPPER",
				ContainerImage:  "docker.io/library/nginx:latest",
				EnvironmentName: "Prod",
				ReplicaIndex:    0,
				Endpoints:       []replicaEndpoint{{Name: "web", URL: "https://example.ngrok.app:8080"}},
			}},
		}
	}
	poller, _ := newRunnerProtocolTestHarness(t, server)

	require.NoError(t, poller.register(ctx, logr.Discard(), "ko_abc"))
	require.Equal(t, "run_123", poller.runnerID)

	require.Len(t, server.registerCalls, 1)
	reg := server.registerCalls[0]
	require.Equal(t, "pool-join-key", reg["join_key"])
	require.Equal(t, "ko_abc", reg["kubernetes_operator_id"])
	require.Equal(t, "kube", reg["runtime"])
	require.Equal(t, "v1.2.3", reg["version"])

	// First status after registration uploads version + scheduler props and
	// converges the replica set (with the replica ID lowercased).
	require.False(t, poller.statusTick(ctx, logr.Discard(), "ko_abc"))

	var deploy appsv1.Deployment
	require.NoError(t, poller.Get(ctx, client.ObjectKey{Namespace: "default", Name: "prod-0-dplyrep-upper"}, &deploy))

	// Subsequent ticks send an empty body; the server keeps prior values.
	require.False(t, poller.statusTick(ctx, logr.Discard(), "ko_abc"))

	bodies := server.statusBodies()
	require.Len(t, bodies, 2)
	require.Equal(t, "v1.2.3", bodies[0]["version"])
	require.Equal(t, map[string]any{"zone": "us-test-1"}, bodies[0]["scheduler_props"])
	require.Empty(t, bodies[1])
	require.Equal(t, "run_123", server.statusCalls[0].RunnerID)
}

func TestRunnerDecommissionRequestedConvergesAndAcks(t *testing.T) {
	ctx := context.Background()
	server := &fakeShipServer{t: t, registerRunnerID: "run_123"}
	server.statusHandler = func(call statusCall) (int, RunnerStatusResponse) {
		if call.Body["decommission_acknowledged"] == true {
			return http.StatusOK, RunnerStatusResponse{}
		}
		return http.StatusOK, RunnerStatusResponse{DecommissionRequested: true}
	}
	poller, _ := newRunnerProtocolTestHarness(t, server)
	poller.runnerID = "run_123"

	// Seed cluster state with an existing replica the drain must tear down.
	replica := computeReplica{
		ID:              "dplyrep_old",
		ContainerImage:  "docker.io/library/nginx:latest",
		EnvironmentName: "Prod",
		ReplicaIndex:    0,
		Endpoints:       []replicaEndpoint{{Name: "web", URL: "https://example.ngrok.app:8080"}},
	}
	require.NoError(t, poller.createResources(ctx, logr.Discard(), replica))

	// The tick converges to empty, acks, and stops the loop.
	require.True(t, poller.statusTick(ctx, logr.Discard(), "ko_abc"))

	var deployList appsv1.DeploymentList
	require.NoError(t, poller.List(ctx, &deployList, client.InNamespace("default")))
	require.Empty(t, deployList.Items)

	bodies := server.statusBodies()
	require.Len(t, bodies, 2)
	require.Equal(t, true, bodies[1]["decommission_acknowledged"])
}

func TestRunnerStatusGoneIsTerminal(t *testing.T) {
	ctx := context.Background()
	server := &fakeShipServer{t: t, registerRunnerID: "run_123"}
	server.statusHandler = func(statusCall) (int, RunnerStatusResponse) {
		return http.StatusGone, RunnerStatusResponse{}
	}
	poller, _ := newRunnerProtocolTestHarness(t, server)
	poller.runnerID = "run_123"

	require.True(t, poller.statusTick(ctx, logr.Discard(), "ko_abc"))
	// Terminal: no re-registration is attempted.
	require.Empty(t, server.registerCalls)
}

func TestRunnerRegisterGoneIsTerminal(t *testing.T) {
	ctx := context.Background()
	server := &fakeShipServer{t: t, registerStatus: http.StatusGone}
	poller, _ := newRunnerProtocolTestHarness(t, server)

	err := poller.register(ctx, logr.Discard(), "ko_abc")
	require.ErrorIs(t, err, errRunnerDecommissioned)
	require.Len(t, server.registerCalls, 1)
}

func TestRunnerStatusNotFoundReregistersOnce(t *testing.T) {
	ctx := context.Background()
	server := &fakeShipServer{t: t, registerRunnerID: "run_new"}
	server.statusHandler = func(call statusCall) (int, RunnerStatusResponse) {
		if call.RunnerID == "run_old" {
			return http.StatusNotFound, RunnerStatusResponse{}
		}
		return http.StatusOK, RunnerStatusResponse{Replicas: []computeReplica{}}
	}
	poller, _ := newRunnerProtocolTestHarness(t, server)
	poller.runnerID = "run_old"
	poller.needsInitialStatus = false

	// 404 → single re-registration, resume on the next tick.
	require.False(t, poller.statusTick(ctx, logr.Discard(), "ko_abc"))
	require.Equal(t, "run_new", poller.runnerID)
	require.Len(t, server.registerCalls, 1)

	// The next tick re-uploads runner facts under the new runner ID.
	require.False(t, poller.statusTick(ctx, logr.Discard(), "ko_abc"))
	require.Equal(t, "run_new", server.statusCalls[1].RunnerID)
	require.Equal(t, "v1.2.3", server.statusCalls[1].Body["version"])
}

func TestRunnerStatusNotFoundThenRegisterGoneIsTerminal(t *testing.T) {
	ctx := context.Background()
	server := &fakeShipServer{t: t, registerStatus: http.StatusGone}
	server.statusHandler = func(statusCall) (int, RunnerStatusResponse) {
		return http.StatusNotFound, RunnerStatusResponse{}
	}
	poller, _ := newRunnerProtocolTestHarness(t, server)
	poller.runnerID = "run_old"

	require.True(t, poller.statusTick(ctx, logr.Discard(), "ko_abc"))
}

func TestDecommissionByKubernetesOperatorID(t *testing.T) {
	ctx := context.Background()

	t.Run("resolves runner and decommissions", func(t *testing.T) {
		server := &fakeShipServer{t: t, registerRunnerID: "run_123"}
		_, runnerClient := newRunnerProtocolTestHarness(t, server)
		require.NoError(t, runnerClient.DecommissionByKubernetesOperatorID(ctx, "ko_abc"))
		require.Len(t, server.registerCalls, 1)
	})

	t.Run("already decommissioned is success", func(t *testing.T) {
		server := &fakeShipServer{t: t, registerStatus: http.StatusGone}
		_, runnerClient := newRunnerProtocolTestHarness(t, server)
		require.NoError(t, runnerClient.DecommissionByKubernetesOperatorID(ctx, "ko_abc"))
	})

	t.Run("pool rejected registration is success", func(t *testing.T) {
		server := &fakeShipServer{t: t, registerStatus: http.StatusBadRequest}
		_, runnerClient := newRunnerProtocolTestHarness(t, server)
		require.NoError(t, runnerClient.DecommissionByKubernetesOperatorID(ctx, "ko_abc"))
	})

	t.Run("server error propagates", func(t *testing.T) {
		server := &fakeShipServer{t: t, registerStatus: http.StatusInternalServerError}
		_, runnerClient := newRunnerProtocolTestHarness(t, server)
		require.Error(t, runnerClient.DecommissionByKubernetesOperatorID(ctx, "ko_abc"))
	})
}

func TestParseComputeMetadata(t *testing.T) {
	t.Run("full block with unknown keys", func(t *testing.T) {
		meta := ParseComputeMetadata(`{
			"enabled": true,
			"pool_join_key": "jk",
			"scheduler_props": {"zone": "a", "weight": 2},
			"description": "my runner",
			"metadata": {"team": "platform"},
			"unknown_key": ["ignored"]
		}`)
		require.NotNil(t, meta.Enabled)
		require.True(t, *meta.Enabled)
		require.Equal(t, "jk", meta.PoolJoinKey)
		require.Equal(t, map[string]any{"zone": "a", "weight": float64(2)}, meta.SchedulerProps)
		require.Equal(t, "my runner", meta.Description)
		require.Equal(t, map[string]string{"team": "platform"}, meta.Metadata)
	})

	t.Run("empty string", func(t *testing.T) {
		require.Equal(t, ComputeMetadata{}, ParseComputeMetadata(""))
	})

	t.Run("invalid JSON", func(t *testing.T) {
		require.Equal(t, ComputeMetadata{}, ParseComputeMetadata("{not json"))
	})

	t.Run("join key only", func(t *testing.T) {
		meta := ParseComputeMetadata(`{"pool_join_key": "jk"}`)
		require.Nil(t, meta.Enabled)
		require.Equal(t, "jk", meta.PoolJoinKey)
	})
}
