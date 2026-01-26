# Uninstalling the ngrok Operator

This guide explains how to safely uninstall the ngrok-operator while ensuring:
- All ngrok API resources (endpoints, domains, etc.) are properly cleaned up
- Kubernetes resources don't get stuck in `Terminating` state
- No orphaned resources remain in your cluster

## Overview

The ngrok-operator uses Kubernetes finalizers to ensure that ngrok API resources are deleted before the corresponding Kubernetes resources are removed. This creates a dependency: the operator must be running to process finalizer removal.

## Quick Start

### Helm (Recommended)

If you installed via Helm with `cleanupHook.enabled: true` (the default), simply run:

```bash
helm uninstall ngrok-operator -n ngrok-operator
```

The pre-delete hook will:
1. Delete the `KubernetesOperator` CR (which triggers drain mode)
2. Wait for the operator to drain all managed resources
3. Complete the uninstall

### Manual / Non-Helm

For non-Helm installations, follow these steps:

```bash
# Step 1: Trigger drain by deleting the KubernetesOperator CR
kubectl delete kubernetesoperator <name> -n <namespace>

# Step 2: Wait for drain to complete
kubectl wait --for=delete kubernetesoperator/<name> -n <namespace> --timeout=300s

# Step 3: Delete the operator deployment and other resources
kubectl delete -f operator-manifests.yaml
```

## Drain Policies

The operator supports two drain policies, configured via the `drainPolicy` Helm value:

### Retain (Default)

```yaml
drainPolicy: "Retain"
```

In Retain mode:
- Finalizers are removed from all managed resources
- Kubernetes CRs (CloudEndpoint, AgentEndpoint, Domain, etc.) are NOT deleted
- **ngrok API resources are preserved** - endpoints, domains, etc. remain in your ngrok account
- Best for production where you want to keep your ngrok configuration

### Delete

```yaml
drainPolicy: "Delete"
```

In Delete mode:
- Finalizers are removed from all managed resources
- Kubernetes CRs are deleted, triggering their controllers to clean up ngrok API resources
- **ngrok API resources are deleted** - endpoints, domains, etc. are removed from your ngrok account
- Best for development/testing where you want a clean slate

## Drain Mode

The operator supports a "drain mode" that cleans up all resources it manages. Drain mode is triggered when:

1. The `KubernetesOperator` CR is deleted (has a `DeletionTimestamp`)
2. The `status.drainStatus` is set to `draining`

Once drain mode is detected, it is cached and never resets (to prevent race conditions).

### What Happens During Drain

When drain mode is triggered:

1. **All controllers stop adding finalizers** - The `DrainState` interface is checked before `RegisterAndSyncFinalizer()` in all controllers. Non-delete reconciles are skipped.

2. **The Driver stops syncing** - `Driver.Sync()` returns early to prevent creating new operator CRs.

3. **The BoundEndpointPoller stops polling** - No new BoundEndpoints are created from the ngrok API.

4. **The Drainer processes all managed resources**:

   **User-owned resources** (Ingress, Service, Gateway, HTTPRoute, TCPRoute, TLSRoute):
   - Removes the `k8s.ngrok.com/finalizer`
   - Does NOT delete the Kubernetes resource (preserves user's intent)

   **Operator-managed resources** (Domain, IPPolicy, CloudEndpoint, AgentEndpoint, BoundEndpoint):
   - Removes the `k8s.ngrok.com/finalizer`
   - If `drainPolicy: Delete`: Deletes the Kubernetes CR, which triggers the controller to delete the ngrok API resource
   - If `drainPolicy: Retain`: Leaves the CR in place (it will be removed when CRDs are deleted)

5. **KubernetesOperator CR is cleaned up**:
   - Deletes the KubernetesOperator from the ngrok API
   - Removes the finalizer from the CR
   - CR is deleted by Kubernetes

### Monitoring Drain Progress

You can monitor drain progress via the `KubernetesOperator` status:

```bash
kubectl get kubernetesoperator <name> -n <namespace> -o yaml
```

Status fields:
- `status.drainStatus`: `pending`, `draining`, `completed`, or `failed`
- `status.drainMessage`: Human-readable status message
- `status.drainProgress`: Progress indicator (e.g., `5/10` where X is processed count and Y is total)
- `status.drainErrors`: Array of error messages from the most recent drain attempt

### Triggering Drain

To trigger drain, delete the KubernetesOperator CR:

```bash
kubectl delete kubernetesoperator <name> -n <namespace>
```

Watch the status while draining:

```bash
kubectl get kubernetesoperator <name> -n <namespace> -w
```

## Multi-Instance Installations

If you have multiple ngrok-operator instances (e.g., in different namespaces or managing different IngressClasses), drain only affects resources managed by that specific instance.

**How resources are filtered:**

| Resource Type | Filtering Method |
|---------------|------------------|
| Ingress | By `IngressClass` - only drains Ingresses using an IngressClass managed by this operator |
| Gateway, HTTPRoute, TCPRoute, TLSRoute | By `GatewayClass` - only drains resources using a GatewayClass managed by this operator |
| Service, CloudEndpoint, AgentEndpoint, etc. | By namespace (if `--ingress-watch-namespace` is set) and finalizer presence |

**Example multi-operator scenario:**
- Operator A manages `IngressClass: ngrok-a` with `drainPolicy: Delete`
- Operator B manages `IngressClass: ngrok-b`
- When Operator A is uninstalled, only Ingresses using `ngrok-a` are drained
- Ingresses using `ngrok-b` are unaffected

This ensures that uninstalling one operator instance doesn't affect resources managed by another.

## Troubleshooting

### Resources Stuck in Terminating State

If resources are stuck in `Terminating` after uninstall:

1. **Check if operator is running**:
   ```bash
   kubectl get pods -n ngrok-operator
   ```
   If not running, the finalizer cannot be removed automatically.

2. **Manual finalizer removal** (use with caution - may orphan ngrok resources):
   ```bash
   kubectl patch ingress <name> -n <namespace> \
     --type=json -p='[{"op":"remove","path":"/metadata/finalizers"}]'
   ```

3. **Re-install the operator temporarily** to let it clean up properly:
   ```bash
   helm install ngrok-operator ngrok/ngrok-operator -n ngrok-operator
   # Wait for cleanup
   helm uninstall ngrok-operator -n ngrok-operator
   ```

### Drain Timeout

If drain takes too long, the Helm hook may time out. Increase the timeout:

```yaml
# values.yaml
cleanupHook:
  timeout: 600  # 10 minutes
```

### Orphaned ngrok Resources

If Kubernetes resources were deleted before the operator could clean up ngrok API resources, those resources may be orphaned in the ngrok dashboard. You can:

1. Manually delete them from the [ngrok Dashboard](https://dashboard.ngrok.com)
2. Use the ngrok API or CLI to list and delete them

## Cleanup Hook Configuration

The Helm chart includes a pre-delete hook that automates the drain process.

```yaml
# values.yaml
cleanupHook:
  enabled: true      # Enable the cleanup hook (default: true)
  timeout: 300       # Timeout in seconds (default: 300)
  image:
    repository: bitnami/kubectl
    tag: latest
  resources:
    limits:
      cpu: 100m
      memory: 128Mi
    requests:
      cpu: 50m
      memory: 64Mi

drainPolicy: "Retain"  # "Delete" or "Retain" (default: Retain)
```

### Disabling the Cleanup Hook

If you prefer manual cleanup or have a custom uninstall process:

```yaml
cleanupHook:
  enabled: false
```

## Architecture

### Components Involved in Drain

| Component | Role |
|-----------|------|
| `KubernetesOperatorReconciler` | Uses `StateChecker.IsDraining()` to detect drain, calls `SetDraining(true)` to trigger, runs Drainer, updates status |
| `StateChecker` | Single source of truth for drain state, caches state once draining starts |
| `Drainer` | Removes finalizers from all managed resources |
| `BaseController` | Skips non-delete reconciles when draining |
| `Driver` | Skips `Sync()` when draining |
| `BoundEndpointPoller` | Skips polling when draining |

### Key Files

- `internal/drainstate/drainstate.go` - Shared `State` interface
- `internal/drain/state.go` - `StateChecker` implementation
- `internal/drain/drain.go` - `Drainer` implementation (includes RBAC tags for drained resources)
- `internal/util/k8s.go` - Finalizer utilities (`HasFinalizer`, `RemoveFinalizer`, etc.)
- `internal/controller/ngrok/kubernetesoperator_controller.go` - Drain orchestration
- `helm/ngrok-operator/templates/cleanup-hook/` - Helm pre-delete hook
- `tools/make/_common.mk` - `CONTROLLER_GEN_PATHS` includes `./internal/drain/...` for RBAC generation

## Best Practices

1. **Always drain before uninstall** - Ensure the `KubernetesOperator` CR is deleted and drain completes before removing the operator deployment

2. **Monitor drain status** - Check the CR status to confirm all resources are cleaned up

3. **Don't delete the operator deployment first** - If the deployment is deleted before drain completes, resources may be stuck with finalizers

4. **Use Helm hooks** - The default cleanup hook automates the correct uninstall order

5. **Plan for timeout** - For large installations with many resources, increase the `cleanupHook.timeout`

6. **Choose the right policy** - Use `Retain` for production (preserves ngrok resources), `Delete` for dev/test (clean slate)

---

## Technical Implementation Details

This section documents the full implementation of the drain feature for developers and code reviewers.

### Summary of Changes

The drain feature adds ~1,500 lines of new code across 24 files. Key changes:

| Category | Files Modified | Description |
|----------|----------------|-------------|
| **CRD Types** | `api/ngrok/v1alpha1/kubernetesoperator_types.go` | Added `DrainConfig` struct with `Enabled` and `Policy` fields, drain status constants |
| **Drain Package** | `internal/drain/state.go`, `internal/drain/drain.go` | New package with `StateChecker` and `Drainer` implementations |
| **Base Controller** | `internal/controller/base_controller.go` | Added `DrainState` interface and field, drain check in `Reconcile()` |
| **Controllers** | Multiple controller files | Added `DrainState` field wiring |
| **Gateway Controllers** | `internal/controller/gateway/*.go` | Added explicit drain checks (don't use BaseController) |
| **Driver** | `pkg/managerdriver/driver.go` | Added `DrainState` field and `WithDrainState()` option, early return in `Sync()` |
| **Command Wiring** | `cmd/api-manager.go`, `cmd/agent-manager.go`, `cmd/bindings-forwarder-manager.go` | Create and wire `StateChecker` to all components |
| **Helm Chart** | `helm/ngrok-operator/templates/cleanup-hook/`, `values.yaml` | Pre-delete hook and `drainPolicy` value |
| **RBAC** | `helm/ngrok-operator/templates/agent/rbac.yaml` | Added `kubernetesoperators` read permission for agent-manager |

### CRD Changes

**DrainPolicy Type:**
```go
// DrainPolicy determines how ngrok API resources are handled during drain
// +kubebuilder:validation:Enum=Delete;Retain
type DrainPolicy string

const (
    // DrainPolicyDelete deletes the CR, triggering controllers to clean up ngrok API resources
    DrainPolicyDelete DrainPolicy = "Delete"
    // DrainPolicyRetain removes finalizers but preserves ngrok API resources
    DrainPolicyRetain DrainPolicy = "Retain"
)
```

**DrainConfig Spec:**
```go
type DrainConfig struct {
    // Policy determines whether to delete ngrok API resources or just remove finalizers
    // +kubebuilder:default=Retain
    Policy DrainPolicy `json:"policy,omitempty"`
}
```

**Helper Method:**
```go
// GetDrainPolicy returns the configured drain policy, defaulting to Retain if not set.
func (ko *KubernetesOperator) GetDrainPolicy() DrainPolicy
```

**KubernetesOperator Status:**
```go
// DrainStatus indicates the current state of the drain process
// +kubebuilder:validation:Enum=pending;draining;completed;failed
DrainStatus string `json:"drainStatus,omitempty"`
DrainMessage string `json:"drainMessage,omitempty"`
DrainProgress string `json:"drainProgress,omitempty"`  // Format: "X/Y" (processed/total)
DrainErrors []string `json:"drainErrors,omitempty"`    // Error messages from drain attempts
```

**DrainStatus Constants:**
```go
const (
    DrainStatusPending   = "pending"
    DrainStatusDraining  = "draining"
    DrainStatusCompleted = "completed"
    DrainStatusFailed    = "failed"
)
```

### DrainState Interface

Defined in `internal/drainstate/drainstate.go`:

```go
// State is an interface for checking if the operator is in drain mode.
type State interface {
    IsDraining(ctx context.Context) bool
}

// IsDraining is a helper function that safely checks if drain mode is active.
// Returns false if state is nil, avoiding the need for nil checks at every call site.
func IsDraining(ctx context.Context, state State) bool
```

This interface is re-exported as type aliases in:
- `internal/controller` as `DrainState`
- `internal/drain` as `State`
- `pkg/managerdriver` as `DrainState`

### StateChecker Implementation

Located in `internal/drain/state.go`:

```go
type StateChecker struct {
    client             client.Client
    operatorNamespace  string
    operatorConfigName string  // KubernetesOperator CR name (release name)
    
    mu       sync.RWMutex
    draining bool  // Cached - once true, never resets
}

func (s *StateChecker) IsDraining(ctx context.Context) bool {
    // Fast path: check cache
    // Slow path: query KubernetesOperator CR by name
    // Draining if: DeletionTimestamp set OR status.drainStatus == "draining"
}
```

Key design decisions:
- Looks up KubernetesOperator by **name** (not list+filter) using `operatorConfigName`
- **Caches** draining state - once detected, never resets (prevents race conditions)
- All manager pods need `--release-name` flag to know the CR name

### Drainer Implementation

Located in `internal/drain/drain.go`:

```go
type Drainer struct {
    Client         client.Client
    Log            logr.Logger
    Policy         DrainPolicy  // "Delete" or "Retain"
    WatchNamespace string       // If set, only drain in this namespace
}

func (d *Drainer) DrainAll(ctx context.Context) (*DrainResult, error)
```

**Resource handling:**

| Resource Type | Retain Mode | Delete Mode |
|---------------|-------------|-------------|
| HTTPRoute, TCPRoute, TLSRoute | Remove finalizer | Remove finalizer (user resource) |
| Ingress, Service, Gateway | Remove finalizer | Remove finalizer (user resource) |
| CloudEndpoint, AgentEndpoint | Remove finalizer | Delete CR (controller cleans up API) |
| Domain, IPPolicy | Remove finalizer | Delete CR (controller cleans up API) |
| BoundEndpoint | Remove finalizer | Delete CR (controller cleans up API) |

**Note:** In Delete mode, operator-managed resources are deleted *without* removing the finalizer first. This ensures the controller has a chance to clean up ngrok API resources during the delete reconcile.

**Resource filtering:**
- **Namespace filtering**: If `WatchNamespace` is set, only resources in that namespace are drained
- **Finalizer filtering**: For all resource types, only resources with the `k8s.ngrok.com/finalizer` are processed

**Multi-operator note:** For user-owned resources (Ingress, Gateway, Routes), the drain only removes finalizers—it does not delete. If a finalizer is removed from a resource owned by a different operator that is not draining, that operator will simply re-add the finalizer during its next reconcile.

### Controller Integration

**BaseController users** (automatic drain check):
- `AgentEndpointReconciler`
- `ForwarderReconciler`
- `DomainReconciler`
- `IPPolicyReconciler`
- `CloudEndpointReconciler`
- `ServiceReconciler`

**Gateway controllers** (explicit drain check before `RegisterAndSyncFinalizer`):
- `GatewayReconciler`
- `HTTPRouteReconciler`
- `TCPRouteReconciler`
- `TLSRouteReconciler`

**KubernetesOperatorReconciler** - Uses the shared `StateChecker` as source of truth:
```go
func (r *KubernetesOperatorReconciler) Reconcile(...) {
    // Use the shared DrainState as the single source of truth
    if r.DrainState != nil && r.DrainState.IsDraining(ctx) {
        return r.handleDrain(ctx, ko, log)  // Runs Drainer
    }
    return r.controller.Reconcile(...)  // Normal reconcile
}

func (r *KubernetesOperatorReconciler) handleDrain(...) {
    // Immediately notify all other controllers
    r.DrainState.SetDraining(true)
    // ... run Drainer
}
```

### Driver Integration

In `pkg/managerdriver/driver.go`:

```go
func (d *Driver) Sync(ctx context.Context, client client.Client) error {
    if d.drainState != nil && d.drainState.IsDraining(ctx) {
        d.log.V(1).Info("Draining, skipping sync")
        return nil
    }
    // ... normal sync
}
```

### BoundEndpointPoller Integration

In `internal/controller/bindings/boundendpoint_poller.go`:

```go
// Helper method to check drain state and cancel any active reconciliation
func (r *BoundEndpointPoller) cancelIfDraining(ctx context.Context, log logr.Logger, operation string) bool

func (r *BoundEndpointPoller) reconcileBoundEndpointsFromAPI(ctx context.Context) error {
    if r.cancelIfDraining(ctx, log, "poll") {
        return nil
    }
    // ... normal polling
}
```

### Helm Hook Implementation

**Job** (`templates/cleanup-hook/job.yaml`):
```bash
# Delete and wait for drain to complete
kubectl delete kubernetesoperator "$KO_NAME" -n {{ .Release.Namespace }} \
  --wait=true --timeout={{ .Values.cleanupHook.timeout }}s
```

**RBAC** (`templates/cleanup-hook/rbac.yaml`):
- Namespace-scoped `Role` (not ClusterRole)
- Permissions: `get`, `list`, `watch`, `delete` on `kubernetesoperators`

### Command-Line Flags

The operator uses several identity-related flags. Understanding their differences is important:

| Flag | Purpose | Helm Value |
|------|---------|------------|
| `--release-name` | Name of the `KubernetesOperator` CR (used by `StateChecker` to detect drain mode) | `{{ .Release.Name }}` |
| `--manager-name` | Unique identifier for the manager deployment, used in `k8s.ngrok.com/controller-name` labels to identify resource ownership | `{{ fullname }}-<suffix>` |
| `--ingress-controller-name` | Value to match in `IngressClass.spec.controller` (Kubernetes Ingress API concept) | `{{ .Values.ingress.controllerName }}` |

**Why separate flags?**

- `--release-name` identifies the shared `KubernetesOperator` CR that all managers reference for drain detection
- `--manager-name` is unique per manager deployment (e.g., `ngrok-operator-manager`, `ngrok-operator-agent-manager`) for multi-instance resource ownership tracking
- These serve different purposes and cannot be consolidated

**Drain-specific flags:**

| Manager | Flag | Default | Purpose |
|---------|------|---------|---------|
| api-manager | `--release-name` | `ngrok-operator` | KubernetesOperator CR name |
| api-manager | `--drain-policy` | `Retain` | Drain policy to set on CR |
| agent-manager | `--release-name` | `ngrok-operator` | KubernetesOperator CR name |
| bindings-forwarder | `--release-name` | `ngrok-operator` | KubernetesOperator CR name |

### Helm Values

```yaml
drainPolicy: "Retain"  # "Delete" or "Retain"

cleanupHook:
  enabled: true
  timeout: 300
  image:
    repository: bitnami/kubectl
    tag: latest
```

### Test Coverage

- `internal/drain/state_test.go` - StateChecker tests (caching, CR states)
- `internal/drain/drain_test.go` - Drainer tests (Delete/Retain modes, error handling)
