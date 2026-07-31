package compute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	ngrok "github.com/ngrok/ngrok-api-go/v7"
)

// ComputeMetadata is the parsed form of the free-form --compute-metadata JSON
// block passed to the operator via helm. Unknown keys are ignored.
type ComputeMetadata struct {
	// Enabled gates the in-cluster app-replica poller; nil means enabled.
	Enabled *bool `json:"enabled"`
	// PoolJoinKey selects the compute pool to register with; empty selects
	// the account's default pool.
	PoolJoinKey string `json:"pool_join_key"`
	// SchedulerProps is free-form scheduling metadata reported to ship.
	SchedulerProps map[string]any `json:"scheduler_props"`
	// Description is an optional human-readable runner description.
	Description string `json:"description"`
	// Metadata is an optional string map attached to the runner.
	Metadata map[string]string `json:"metadata"`
}

// ParseComputeMetadata leniently parses the --compute-metadata JSON block.
// Empty or invalid JSON yields the zero value, mirroring how the poller
// enablement check treats malformed metadata.
func ParseComputeMetadata(raw string) ComputeMetadata {
	var meta ComputeMetadata
	if raw == "" {
		return meta
	}
	_ = json.Unmarshal([]byte(raw), &meta)
	return meta
}

// runnerRegisterRequest is the body for POST /v1/runners/register.
type runnerRegisterRequest struct {
	JoinKey              string            `json:"join_key"`
	KubernetesOperatorID string            `json:"kubernetes_operator_id"`
	Runtime              string            `json:"runtime"`
	Description          string            `json:"description,omitempty"`
	Metadata             map[string]string `json:"metadata,omitempty"`
	Version              string            `json:"version,omitempty"`
}

type runnerRegisterResponse struct {
	RunnerID string `json:"runner_id"`
}

// RunnerStatusRequest is the body for PUT /v1/runners/{id}/status. All fields
// are optional; empty/omitted fields leave the server-side value unchanged,
// so steady-state ticks send an empty body.
type RunnerStatusRequest struct {
	Version                  string            `json:"version,omitempty"`
	Description              string            `json:"description,omitempty"`
	Metadata                 map[string]string `json:"metadata,omitempty"`
	SchedulerProps           map[string]any    `json:"scheduler_props,omitempty"`
	DecommissionAcknowledged bool              `json:"decommission_acknowledged,omitempty"`
}

// RunnerStatusResponse is the server's reply to a status exchange: the desired
// replica set plus any decommission instruction.
type RunnerStatusResponse struct {
	Replicas              []computeReplica `json:"replicas"`
	DecommissionRequested bool             `json:"decommission_requested"`
}

// RunnerClient speaks the ship runner protocol under
// {ComputeBaseURL}/v1/runners/, authenticated with the ngrok API key carried
// by the underlying base client.
type RunnerClient struct {
	// NgrokBaseClient performs the HTTP requests (sets the bearer token).
	NgrokBaseClient *ngrok.BaseClient

	// ComputeBaseURL is the absolute base URL of the compute service; being
	// absolute it wins over the SDK's BaseURL via ResolveReference.
	ComputeBaseURL string

	// JoinKey selects the pool to register with; empty selects the account
	// default pool.
	JoinKey string

	// Description, Metadata, and Version describe this runner at
	// registration time.
	Description string
	Metadata    map[string]string
	Version     string
}

func (c *RunnerClient) runnerURL(pathSuffix string) (*url.URL, error) {
	return url.Parse(fmt.Sprintf("%s/v1/runners/%s", strings.TrimSuffix(c.ComputeBaseURL, "/"), pathSuffix))
}

// Register registers this operator identity with ship and returns the runner
// ID. Registration is idempotent on (pool, kubernetes_operator_id).
func (c *RunnerClient) Register(ctx context.Context, kubernetesOperatorID string) (string, error) {
	reqURL, err := c.runnerURL("register")
	if err != nil {
		return "", fmt.Errorf("parsing compute URL: %w", err)
	}
	req := runnerRegisterRequest{
		JoinKey:              c.JoinKey,
		KubernetesOperatorID: kubernetesOperatorID,
		Runtime:              "kube",
		Description:          c.Description,
		Metadata:             c.Metadata,
		Version:              c.Version,
	}
	var resp runnerRegisterResponse
	if err := c.NgrokBaseClient.Do(ctx, http.MethodPost, reqURL, req, &resp); err != nil {
		return "", fmt.Errorf("registering runner: %w", err)
	}
	return resp.RunnerID, nil
}

// Status performs one bidirectional status exchange: uploads any runner facts
// in req and returns the desired replica set plus any decommission
// instruction. Replica IDs are lowercased for use in Kubernetes names.
func (c *RunnerClient) Status(ctx context.Context, runnerID string, req RunnerStatusRequest) (*RunnerStatusResponse, error) {
	reqURL, err := c.runnerURL(url.PathEscape(runnerID) + "/status")
	if err != nil {
		return nil, fmt.Errorf("parsing compute URL: %w", err)
	}
	var resp RunnerStatusResponse
	if err := c.NgrokBaseClient.Do(ctx, http.MethodPut, reqURL, req, &resp); err != nil {
		return nil, fmt.Errorf("exchanging runner status: %w", err)
	}
	for i := range resp.Replicas {
		resp.Replicas[i].ID = strings.ToLower(resp.Replicas[i].ID)
	}
	return &resp, nil
}

// PublishKubernetesAccess reports the runner's remote Kubernetes access
// (replacement-style: empty fields leave previous values in place).
func (c *RunnerClient) PublishKubernetesAccess(ctx context.Context, runnerID string, request remoteAccessRequest, response *remoteAccessResponse) error {
	reqURL, err := c.runnerURL(url.PathEscape(runnerID) + "/kubernetes-access")
	if err != nil {
		return fmt.Errorf("parsing compute URL: %w", err)
	}
	// Passing a typed nil pointer as interface{} produces a non-nil interface.
	// Pass a literal nil so ngrok-api-go does not try to decode an intentionally
	// empty lifecycle response.
	if response == nil {
		return c.NgrokBaseClient.Do(ctx, http.MethodPut, reqURL, request, nil)
	}
	return c.NgrokBaseClient.Do(ctx, http.MethodPut, reqURL, request, response)
}

// Decommission tells ship this runner's workloads are gone and unassigns its
// replicas. It is idempotent and must only be called on permanent removal —
// never on ordinary pod restarts, since the workloads keep running and the
// re-registering operator would find its identity gone.
func (c *RunnerClient) Decommission(ctx context.Context, runnerID string) error {
	reqURL, err := c.runnerURL(url.PathEscape(runnerID) + "/decommission")
	if err != nil {
		return fmt.Errorf("parsing compute URL: %w", err)
	}
	if err := c.NgrokBaseClient.Do(ctx, http.MethodPost, reqURL, nil, nil); err != nil {
		return fmt.Errorf("decommissioning runner: %w", err)
	}
	return nil
}

// DecommissionByKubernetesOperatorID resolves the runner for this operator
// identity via idempotent registration, then decommissions it. Statuses that
// mean there is nothing left to decommission (register 400 — bad join key or
// pool deleting — or 410 — already decommissioned; decommission 404) are
// treated as success so permanent removal is never blocked on them.
func (c *RunnerClient) DecommissionByKubernetesOperatorID(ctx context.Context, kubernetesOperatorID string) error {
	runnerID, err := c.Register(ctx, kubernetesOperatorID)
	if err != nil {
		switch httpStatusOf(err) {
		case http.StatusBadRequest, http.StatusGone:
			return nil
		}
		return err
	}
	if err := c.Decommission(ctx, runnerID); err != nil && httpStatusOf(err) != http.StatusNotFound {
		return err
	}
	return nil
}

// httpStatusOf extracts the HTTP status code carried by an ngrok API error,
// or 0 for non-API errors.
func httpStatusOf(err error) int {
	var apiErr *ngrok.Error
	if errors.As(err, &apiErr) {
		return int(apiErr.StatusCode)
	}
	return 0
}
