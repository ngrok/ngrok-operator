package compute

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	ngrok "github.com/ngrok/ngrok-api-go/v7"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRemoteAccessRegistersBearerKeyAndStoresOnlyVerifier(t *testing.T) {
	ctx := context.Background()
	var requests []RunnerStatusRequest
	var registrations []runnerRegisterRequest
	httpClient := &http.Client{Transport: computeRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/register") {
			var request runnerRegisterRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			registrations = append(registrations, request)
			return jsonResponse(t, runnerRegisterResponse{RunnerID: "rnr_XYZ789"}), nil
		}
		require.Equal(t, http.MethodPut, r.Method)
		// Publication is addressed to the registered runner identity, not the
		// operator ID, and rides the status exchange.
		require.Equal(t, "/v1/runners/rnr_XYZ789/status", r.URL.Path)
		var request RunnerStatusRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		requests = append(requests, request)
		if request.KubernetesAccessKey != "" {
			return jsonResponse(t, RunnerStatusResponse{
				KubernetesAccessEndpoint: "https://rnr-xyz789.k8s.compute.internal",
			}), nil
		}
		return jsonResponse(t, RunnerStatusResponse{}), nil
	})}

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&ngrokv1alpha1.KubernetesOperator{
			ObjectMeta: metav1.ObjectMeta{Name: "operator", Namespace: "ngrok"},
			Status:     ngrokv1alpha1.KubernetesOperatorStatus{ID: "k8sop_ABC123"},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "operator-compute-api", Namespace: "ngrok"},
			Status:     appsv1.DeploymentStatus{AvailableReplicas: 1},
		},
	).Build()
	const computeBaseURL = "https://compute.example"
	apiClient := ngrok.NewBaseClient(ngrok.NewClientConfig(
		"api-key", ngrok.WithBaseURL(computeBaseURL), ngrok.WithHTTPClient(httpClient),
	))
	remote := &RemoteAccess{
		Client: k8sClient, Log: logr.Discard(),
		RunnerAPI: &RunnerClient{NgrokBaseClient: apiClient, ComputeBaseURL: computeBaseURL},
		Namespace: "ngrok", K8sOpName: "operator",
		GatewayName: "operator-compute-api", ReplicaNamespace: "ngrok-compute",
	}

	require.NoError(t, remote.reconcile(ctx))
	require.Len(t, registrations, 1)
	require.Equal(t, "k8sop_ABC123", registrations[0].KubernetesOperatorID)
	require.Len(t, requests, 2)
	// First publication mints the key; readiness is not yet claimed.
	require.Len(t, requests[0].KubernetesAccessKey, 43)
	require.Equal(t, "ngrok-compute", requests[0].KubernetesAccessNamespace)
	require.False(t, requests[0].KubernetesAccessReady)
	// The steady-state report restates namespace + readiness, never the key.
	require.Empty(t, requests[1].KubernetesAccessKey)
	require.Equal(t, "ngrok-compute", requests[1].KubernetesAccessNamespace)
	// Not ready yet: the gateway has only just been rolled onto this key, and
	// its running pods still hold the previous one.
	require.False(t, requests[1].KubernetesAccessReady)

	var config corev1.ConfigMap
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{
		Name: "operator-compute-api", Namespace: "ngrok",
	}, &config))
	require.Equal(t, "https://rnr-xyz789.k8s.compute.internal", config.Data[remoteEndpointKey])
	hash := sha256.Sum256([]byte(requests[0].KubernetesAccessKey))
	require.Equal(t, base64.RawURLEncoding.EncodeToString(hash[:]), config.Data[remoteTokenHashKey])

	var secrets corev1.SecretList
	require.NoError(t, k8sClient.List(ctx, &secrets, client.InNamespace("ngrok")))
	require.Empty(t, secrets.Items)

	restarted := &RemoteAccess{
		Client: k8sClient, Log: logr.Discard(),
		RunnerAPI: &RunnerClient{NgrokBaseClient: apiClient, ComputeBaseURL: computeBaseURL},
		Namespace: "ngrok", K8sOpName: "operator",
		GatewayName: "operator-compute-api", ReplicaNamespace: "ngrok-compute",
	}
	require.NoError(t, restarted.reconcile(ctx))
	require.Len(t, requests, 4)
	require.NotEqual(t, requests[0].KubernetesAccessKey, requests[2].KubernetesAccessKey)
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{
		Name: "operator-compute-api", Namespace: "ngrok",
	}, &config))
	restartedHash := sha256.Sum256([]byte(requests[2].KubernetesAccessKey))
	require.Equal(t, base64.RawURLEncoding.EncodeToString(restartedHash[:]), config.Data[remoteTokenHashKey])
}

// TestRemoteAccessRollsGatewayAndGatesReadiness covers the handoff between the
// two components: the manager publishes the endpoint and key, and the gateway
// reads them once at startup. Republishing is only meaningful if the gateway's
// pods restart, and Ship must not be told the runner is reachable until they
// have.
func TestRemoteAccessRollsGatewayAndGatesReadiness(t *testing.T) {
	ctx := context.Background()
	var requests []RunnerStatusRequest
	endpoints := []string{
		"https://rnr-first.k8s.compute.internal",
		"https://rnr-first.k8s.compute.internal",
	}
	httpClient := &http.Client{Transport: computeRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/register") {
			return jsonResponse(t, runnerRegisterResponse{RunnerID: "rnr_XYZ789"}), nil
		}
		var request RunnerStatusRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		requests = append(requests, request)
		if request.KubernetesAccessKey != "" {
			endpoint := endpoints[0]
			if len(endpoints) > 1 {
				endpoints = endpoints[1:]
			}
			return jsonResponse(t, RunnerStatusResponse{KubernetesAccessEndpoint: endpoint}), nil
		}
		return jsonResponse(t, RunnerStatusResponse{}), nil
	})}

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&ngrokv1alpha1.KubernetesOperator{
			ObjectMeta: metav1.ObjectMeta{Name: "operator", Namespace: "ngrok"},
			Status:     ngrokv1alpha1.KubernetesOperatorStatus{ID: "k8sop_ABC123"},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "operator-compute-api", Namespace: "ngrok"},
			// A pod is up, but it predates the key about to be published.
			Status: appsv1.DeploymentStatus{Replicas: 1, UpdatedReplicas: 1, AvailableReplicas: 1},
		},
	).Build()
	const computeBaseURL = "https://compute.example"
	remote := &RemoteAccess{
		Client: k8sClient, Log: logr.Discard(),
		RunnerAPI: &RunnerClient{
			NgrokBaseClient: ngrok.NewBaseClient(ngrok.NewClientConfig(
				"api-key", ngrok.WithBaseURL(computeBaseURL), ngrok.WithHTTPClient(httpClient),
			)),
			ComputeBaseURL: computeBaseURL,
		},
		Namespace: "ngrok", K8sOpName: "operator",
		GatewayName: "operator-compute-api", ReplicaNamespace: "ngrok-compute",
	}

	gatewayKey := client.ObjectKey{Name: "operator-compute-api", Namespace: "ngrok"}
	configKey := gatewayKey

	// First tick: publish, roll the gateway, and withhold readiness.
	require.NoError(t, remote.reconcile(ctx))
	var gateway appsv1.Deployment
	require.NoError(t, k8sClient.Get(ctx, gatewayKey, &gateway))
	firstRevision := gateway.Spec.Template.Annotations[accessRevisionAnnotation]
	require.NotEmpty(t, firstRevision)
	require.False(t, requests[len(requests)-1].KubernetesAccessReady)

	// The ConfigMap dies with the gateway rather than outliving `helm uninstall`.
	var config corev1.ConfigMap
	require.NoError(t, k8sClient.Get(ctx, configKey, &config))
	require.Len(t, config.OwnerReferences, 1)
	require.Equal(t, "Deployment", config.OwnerReferences[0].Kind)
	require.Equal(t, "operator-compute-api", config.OwnerReferences[0].Name)
	require.Equal(t, firstRevision, accessRevision(config.Data))

	// The rollout finishes: the serving pods now carry the published key.
	gateway.Status = appsv1.DeploymentStatus{
		Replicas: 1, UpdatedReplicas: 1, AvailableReplicas: 1,
		ObservedGeneration: gateway.Generation,
	}
	require.NoError(t, k8sClient.Status().Update(ctx, &gateway))

	require.NoError(t, remote.reconcile(ctx))
	require.True(t, requests[len(requests)-1].KubernetesAccessReady)
	require.NoError(t, k8sClient.Get(ctx, gatewayKey, &gateway))
	require.Equal(t, firstRevision, gateway.Spec.Template.Annotations[accessRevisionAnnotation],
		"a settled gateway must not be rolled again")

	// A manager restart mints a fresh key. The running gateway still verifies
	// against the old one, so it is rolled again and readiness drops.
	remote.registered = false
	require.NoError(t, remote.reconcile(ctx))
	require.NoError(t, k8sClient.Get(ctx, gatewayKey, &gateway))
	require.NotEqual(t, firstRevision, gateway.Spec.Template.Annotations[accessRevisionAnnotation])
	require.False(t, requests[len(requests)-1].KubernetesAccessReady)
}

// TestRemoteAccessWithdrawsReadinessWhenGatewayIsGone asserts the absent case,
// which the previous AvailableReplicas check also got right but for the wrong
// reason.
func TestRemoteAccessWithdrawsReadinessWhenGatewayIsGone(t *testing.T) {
	require.False(t, gatewayRolloutComplete(&appsv1.Deployment{
		Status: appsv1.DeploymentStatus{Replicas: 1, UpdatedReplicas: 1},
	}), "no available replica")
	require.False(t, gatewayRolloutComplete(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Generation: 4},
		Status: appsv1.DeploymentStatus{
			Replicas: 1, UpdatedReplicas: 1, AvailableReplicas: 1, ObservedGeneration: 3,
		},
	}), "status predates the template it is being compared against")
	require.False(t, gatewayRolloutComplete(&appsv1.Deployment{
		Status: appsv1.DeploymentStatus{Replicas: 2, UpdatedReplicas: 1, AvailableReplicas: 1},
	}), "a pod from the previous template is still running")
	require.True(t, gatewayRolloutComplete(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Generation: 4},
		Status: appsv1.DeploymentStatus{
			Replicas: 1, UpdatedReplicas: 1, AvailableReplicas: 1, ObservedGeneration: 4,
		},
	}))
}

func TestRemoteAccessReprovisionsDeletedConfigMap(t *testing.T) {
	ctx := context.Background()
	var requests []RunnerStatusRequest
	httpClient := &http.Client{Transport: computeRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/register") {
			return jsonResponse(t, runnerRegisterResponse{RunnerID: "rnr_XYZ789"}), nil
		}
		var request RunnerStatusRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		requests = append(requests, request)
		if request.KubernetesAccessKey != "" {
			return jsonResponse(t, RunnerStatusResponse{
				KubernetesAccessEndpoint: "https://rnr-xyz789.k8s.compute.internal",
			}), nil
		}
		return jsonResponse(t, RunnerStatusResponse{}), nil
	})}

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&ngrokv1alpha1.KubernetesOperator{
			ObjectMeta: metav1.ObjectMeta{Name: "operator", Namespace: "ngrok"},
			Status:     ngrokv1alpha1.KubernetesOperatorStatus{ID: "k8sop_ABC123"},
		},
	).Build()
	const computeBaseURL = "https://compute.example"
	remote := &RemoteAccess{
		Client: k8sClient, Log: logr.Discard(),
		RunnerAPI: &RunnerClient{
			NgrokBaseClient: ngrok.NewBaseClient(ngrok.NewClientConfig(
				"api-key", ngrok.WithBaseURL(computeBaseURL), ngrok.WithHTTPClient(httpClient),
			)),
			ComputeBaseURL: computeBaseURL,
		},
		Namespace: "ngrok", K8sOpName: "operator",
		GatewayName: "operator-compute-api", ReplicaNamespace: "ngrok-compute",
	}

	require.NoError(t, remote.reconcile(ctx))
	var config corev1.ConfigMap
	key := client.ObjectKey{Name: "operator-compute-api", Namespace: "ngrok"}
	require.NoError(t, k8sClient.Get(ctx, key, &config))
	firstHash := config.Data[remoteTokenHashKey]
	require.NoError(t, k8sClient.Delete(ctx, &config))

	require.NoError(t, remote.reconcile(ctx))
	require.NoError(t, k8sClient.Get(ctx, key, &config))
	require.NotEqual(t, firstHash, config.Data[remoteTokenHashKey])
	require.Len(t, requests, 4)
	require.NotEmpty(t, requests[2].KubernetesAccessKey)
}

func TestNewAccessKey(t *testing.T) {
	key, encodedHash, err := newAccessKey()
	require.NoError(t, err)
	require.Len(t, key, 43)
	hash, err := base64.RawURLEncoding.DecodeString(encodedHash)
	require.NoError(t, err)
	require.Equal(t, sha256.Size, len(hash))
	expected := sha256.Sum256([]byte(key))
	require.Equal(t, expected[:], hash)
}

func TestValidateEndpointURL(t *testing.T) {
	require.NoError(t, validateEndpointURL("https://runner.k8s.compute.internal"))
	require.Error(t, validateEndpointURL("tls://runner.k8s.compute.internal"))
	require.Error(t, validateEndpointURL("https://example.ngrok.app"))
}

type computeRoundTripFunc func(*http.Request) (*http.Response, error)

func (f computeRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(t *testing.T, value any) *http.Response {
	t.Helper()
	body, err := json.Marshal(value)
	require.NoError(t, err)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(body))),
	}
}
