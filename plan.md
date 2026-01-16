# Operator Uninstall Drain Feature - Implementation Plan

## Overview

This plan details the work needed to complete the operator uninstall drain feature. The goal is to provide a smooth uninstall experience where:

1. **Delete mode**: Resources are removed from the ngrok API before uninstall (good for development/iteration)
2. **Retain mode** (default): Resources are preserved in the ngrok API, only finalizers are removed (good for production)

## Current State

The branch has partially implemented:
- ✅ KubernetesOperator CRD with `DrainMode` spec field and drain status fields
- ✅ Helm pre-delete hook that deletes KubernetesOperator CR
- ✅ Basic `Drainer` struct that loops over resource types
- ✅ `StateChecker` with `IsDraining()` method
- ✅ Controller labels for multi-operator support

## Known Issues to Fix

### StateChecker Name Mismatch Bug

The current `StateChecker.IsDraining()` iterates through all KubernetesOperator CRs and matches by `spec.deployment.Name == controllerName`. But different pods pass different names:

| Pod | `controllerName` passed | KubernetesOperator `spec.deployment.Name` |
|-----|------------------------|-------------------------------------------|
| api-manager | "ngrok-ingress-controller-manager" | "ngrok-operator" (releaseName) |
| agent-manager | "agent-manager" | "ngrok-operator" (releaseName) |
| bindings-forwarder | "bindings-forwarder-manager" | "ngrok-operator" (releaseName) |

**Result**: agent-manager and bindings-forwarder will never detect drain mode.

**Fix**: StateChecker should lookup by the known KubernetesOperator CR name (`releaseName`) directly, not match by deployment name. All pods need to know the `releaseName` (passed via flag/env).

### DrainResult.Failed Not Tracked

The `DrainResult.Failed` field is never incremented, causing incorrect status reporting and `IsComplete()` logic.

**Fix**: Count errors properly in each drain handler.

---

## Important Concepts

### Two Different Name Concepts

There are two different naming concepts that serve different purposes:

| Concept | Value Example | Purpose |
|---------|---------------|---------|
| `releaseName` | "ngrok-operator" | Name of the KubernetesOperator CR, used for drain state lookup |
| `managerName` / ControllerLabels | "ngrok-ingress-controller-manager" | Labels on resources to identify which controller instance owns them |

The **StateChecker** needs `releaseName` to find the KubernetesOperator CR.
The **Drainer** needs `ControllerLabels` to find resources owned by this operator instance.

Both are already available in `apiManagerOpts`:
- `opts.releaseName` - for KubernetesOperator CR name
- `opts.managerName` - for controller labels (used in `labels.NewControllerLabelValues()`)

---

## Milestone 1: Helm Configuration & CRD Changes ✅ COMPLETED

**Goal**: Add helm values and CRD fields to support Delete vs Retain mode

### Tasks

#### 1.1 Add Helm value for drain policy
- [x] Add `drainPolicy` to `values.yaml` with default `Retain`
- [x] Wire into api-manager deployment template as `--drain-policy` flag
- [x] Update values.yaml documentation

```yaml
# values.yaml
drainPolicy: "Retain"  # "Delete" or "Retain"
```

#### 1.2 Update KubernetesOperator CRD
- [x] Replace `DrainMode bool` with proper drain config:
  ```go
  // DrainConfig configures the drain behavior
  DrainConfig *DrainConfig `json:"drain,omitempty"`

  type DrainConfig struct {
      // Enabled triggers drain when true
      Enabled bool `json:"enabled,omitempty"`
      // Policy determines whether to delete ngrok API resources or just remove finalizers
      // +kubebuilder:validation:Enum=Delete;Retain
      Policy string `json:"policy,omitempty"`
  }
  ```
- [x] Run `make generate manifests`
- [x] Update helm CRD template

#### 1.3 Update api-manager to set drain policy
- [x] Add `--drain-policy` flag to api-manager.go
- [x] Pass policy when creating KubernetesOperator CR
- [x] Pass policy to KubernetesOperatorReconciler

#### 1.4 Helm Hook
- [x] While we could make our helm hook update this spec field, instead we should keep it doing what it does today by doing a delete. Its much simpler to just wait for that delete to go through than it is to patch a resource and wait on a specific status field.

---

## Milestone 2: Drain State Propagation ✅ COMPLETED

**Goal**: All controllers respect drain state and stop adding finalizers

### Tasks

#### 2.1 Improve StateChecker implementation
- [x] Change StateChecker to lookup KubernetesOperator by name (not list+filter)
- [x] Constructor takes `operatorNamespace` and `operatorConfigName` (the releaseName)
- [x] Cache draining state with proper locking (set to true once detected, never resets)
- [x] All pods need `--release-name` flag or env var to know the KubernetesOperator CR name

```go
type StateChecker struct {
    client              client.Client
    operatorNamespace   string
    operatorConfigName  string  // e.g., "ngrok-operator" (release name)

    mu       sync.RWMutex
    draining bool
}

func NewStateChecker(c client.Client, operatorNamespace, operatorConfigName string) *StateChecker

func (s *StateChecker) IsDraining(ctx context.Context) bool {
    // Fast path: already draining (cached)
    s.mu.RLock()
    if s.draining {
        s.mu.RUnlock()
        return true
    }
    s.mu.RUnlock()

    // Query the specific KubernetesOperator CR by name
    ko := &ngrokv1alpha1.KubernetesOperator{}
    if err := s.client.Get(ctx, types.NamespacedName{
        Namespace: s.operatorNamespace,
        Name:      s.operatorConfigName,
    }, ko); err != nil {
        return false
    }

    isDraining := !ko.DeletionTimestamp.IsZero() ||
                  ko.Spec.DrainMode ||
                  ko.Status.DrainStatus == DrainStatusDraining

    if isDraining {
        s.mu.Lock()
        s.draining = true
        s.mu.Unlock()
    }
    return isDraining
}
```

#### 2.2 Create drain.State interface and implementations
- [x] Ensure `State` interface is clean:
  ```go
  type State interface {
      IsDraining(ctx context.Context) bool
  }
  ```
- [x] Keep `NeverDraining` and `AlwaysDraining` for testing

#### 2.3 Update BaseController to respect drain state
- [x] Add `DrainState drain.State` field to `BaseController`
- [x] In `Reconcile()`, check drain state before `RegisterAndSyncFinalizer()`:
  ```go
  if self.DrainState != nil && self.DrainState.IsDraining(ctx) {
      if !IsDelete(obj) {
          log.V(1).Info("Draining, skipping non-delete reconcile")
          return ctrl.Result{}, nil
      }
  }
  ```

#### 2.4 Update api-manager to wire drain state
- [x] Create `StateChecker` early in `startOperator()` using `opts.releaseName`
- [x] Pass to all controllers that use `BaseController`:
  - DomainReconciler
  - IPPolicyReconciler
  - CloudEndpointReconciler
  - ServiceReconciler

#### 2.5 Update gateway controllers to respect drain state
Gateway controllers don't use BaseController, so update each:
- [x] Add `DrainState drain.State` field to each reconciler
- [x] GatewayReconciler - check before `RegisterAndSyncFinalizer()`
- [x] HTTPRouteReconciler - check before `RegisterAndSyncFinalizer()`
- [x] TCPRouteReconciler - check before `RegisterAndSyncFinalizer()`
- [x] TLSRouteReconciler - check before `RegisterAndSyncFinalizer()`

Pattern for each:
```go
func (r *GatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Early drain check
    if r.DrainState != nil && r.DrainState.IsDraining(ctx) {
        if !controller.IsDelete(gw) {
            return ctrl.Result{}, nil
        }
    }
    // ... rest of reconcile
}
```

---

## Milestone 3: Stop Driver/Store During Drain ✅ COMPLETED

**Goal**: Prevent managerdriver from creating new operator CRs during drain

### Design Options

There are two approaches to stop the driver during drain:

**Option A: Early return in Sync()** (simpler) ← IMPLEMENTED
- Gate `Driver.Sync()` to return early if draining
- Drainer handles deleting all operator CRs
- Pro: Simple, minimal changes
- Con: Drainer must handle all cleanup

**Option B: Purge store to trigger cleanup** (more elegant)
- When drain starts, clear ingress/gateway objects from store
- Driver calculates empty desired state and deletes operator CRs automatically
- Pro: Leverages existing reconciliation logic
- Con: More complex, potential race conditions

**Recommendation**: Start with Option A, consider Option B as optimization.

### Tasks

#### 3.1 Add drain state to Driver
- [x] Add `DrainState drain.State` field to `managerdriver.Driver`
- [x] Add `WithDrainState(state drain.State) DriverOpt`

#### 3.2 Gate driver Sync operations
- [x] In `Driver.Sync()`, return early if draining:
  ```go
  func (d *Driver) Sync(ctx context.Context, client client.Client) error {
      if d.drainState != nil && d.drainState.IsDraining(ctx) {
          d.log.V(1).Info("Draining, skipping sync")
          return nil
      }
      // ... normal sync
  }
  ```

#### 3.3 Gate driver create/update operations (if needed)
- [x] Review if Sync() gating is sufficient
- [x] If not, gate individual create methods as well

---

## Milestone 4: Agent Manager Drain Awareness ✅ COMPLETED

**Goal**: Agent-manager pod respects drain state

### Tasks

#### 4.1 Add StateChecker to agent-manager
- [x] Add `--release-name` flag to agent-manager (default: "ngrok-operator")
- [x] Add RBAC for agent-manager to read KubernetesOperator CRs
- [x] Create StateChecker in `runAgentController()` using release name and namespace

#### 4.2 Update AgentEndpointReconciler
- [x] Add `DrainState drain.State` field
- [x] Check drain state before adding finalizers (similar pattern to BaseController)
- [x] Allow delete reconciles to proceed (for cleanup)

---

## Milestone 5: Bindings Forwarder Drain Awareness ✅ COMPLETED

**Goal**: Bindings-forwarder-manager respects drain state

### Tasks

#### 5.1 Add StateChecker to bindings-forwarder-manager
- [x] Uses existing `--release-name` flag (already present)
- [x] Add RBAC to read KubernetesOperator CRs
- [x] Create StateChecker in `runController()`

#### 5.2 Update ForwarderReconciler
- [x] Add `DrainState drain.State` field
- [x] Gate reconcile when draining (skip non-delete reconciles)

#### 5.3 Update BoundEndpointPoller (in api-manager)
- [x] Add `DrainState drain.State` field
- [x] Stop polling when draining:
  ```go
  func (p *BoundEndpointPoller) poll(ctx context.Context) {
      if p.DrainState != nil && p.DrainState.IsDraining(ctx) {
          p.Log.V(1).Info("Draining, skipping poll")
          return
      }
      // ... normal poll
  }
  ```

---

## Milestone 6: Drainer Improvements ✅ COMPLETED

**Goal**: Fix drainer correctness and implement Delete vs Retain

### Tasks

#### 6.1 Fix DrainResult.Failed tracking
- [x] Count errors in each drain handler
- [x] Set `result.Failed = len(errs)` after each handler
- [x] Fix `IsComplete()` to use proper counts

#### 6.2 Implement Delete vs Retain modes
- [x] Add `Policy string` field to Drainer (passed from KubernetesOperator spec)
- [x] For **Retain mode**: only remove finalizers, don't delete CRs
- [x] For **Delete mode**: remove finalizer, then delete CR
  - Controllers will reconcile the delete and clean up ngrok API resources

```go
func (d *Drainer) drainOperatorResource(ctx context.Context, obj client.Object) error {
    // Remove finalizer first
    if controller.HasFinalizer(obj) {
        controller.RemoveFinalizer(obj)
        if err := d.Client.Update(ctx, obj); err != nil {
            return err
        }
    }

    if d.Policy == "Delete" {
        // Delete the CR - controller will handle ngrok API cleanup
        if err := d.Client.Delete(ctx, obj); err != nil {
            return client.IgnoreNotFound(err)
        }
    }
    // For Retain mode, finalizer is removed - CR will be deleted when CRD is removed
    return nil
}
```

#### 6.3 Remove unused NgrokClientset from Drainer
- [x] The drainer doesn't need to call ngrok API directly
- [x] Controllers handle ngrok API cleanup when reconciling deletes
- [x] Remove `NgrokClientset` field if unused

#### 6.4 Verify deletion ordering
Current order should work. Key points:
- User resources (Ingress/Gateway/Routes): just remove finalizers, no delete
- Operator resources (CloudEndpoint/AgentEndpoint/Domain): remove finalizer, optionally delete

No need to add NgrokTrafficPolicy - it has no finalizer.

---

## Milestone 7: Helm Chart Cleanup ✅ COMPLETED

**Goal**: Clean up RBAC and finalize helm configuration

### Tasks

#### 7.1 Convert cleanup hook to namespace-scoped Role
- [x] Replace ClusterRole with Role in `cleanup-hook/rbac.yaml`
- [x] Replace ClusterRoleBinding with RoleBinding
- [x] Scope to release namespace only

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: {{ include "ngrok-operator.fullname" . }}-cleanup
  namespace: {{ .Release.Namespace }}
  # ...
rules:
  - apiGroups: [ngrok.k8s.ngrok.com]
    resources: [kubernetesoperators]
    verbs: [get, list, watch, patch, delete]
```

#### 7.2 Update cleanup hook to set drain mode
- [x] Patch the KubernetesOperator CR with drain enabled before delete:
```bash
# First enable drain mode
kubectl patch kubernetesoperator "$KO_NAME" -n {{ .Release.Namespace }} \
  --type merge -p '{"spec":{"drain":{"enabled":true}}}'

# Then delete and wait
kubectl delete kubernetesoperator "$KO_NAME" -n {{ .Release.Namespace }} \
  --wait=true --timeout={{ .Values.cleanupHook.timeout }}s
```

#### 7.3 Add agent-manager RBAC for KubernetesOperator read
- [x] Add rules to agent-manager ClusterRole (or Role) to read kubernetesoperators

#### 7.4 Validate controller labels are set
- [x] Ensure `ControllerLabelValues` validation runs at startup
- [x] Fail fast if name or namespace is empty

---

## Milestone 8: Testing & Documentation ✅ COMPLETED

### Tasks

#### 8.1 Unit tests for drain package
- [x] Test StateChecker with various CR states (normal, draining, deleted)
- [x] Test StateChecker caching behavior
- [x] Test Drainer with Delete vs Retain modes
- [x] Test DrainResult tracking

#### 8.2 Integration tests
- [x] Test drain with ingress resources
- [x] Test drain with gateway resources
- [x] Test that controllers don't re-add finalizers during drain

#### 8.3 E2E tests
- [ ] Test helm uninstall with Delete mode (manual testing recommended)
- [ ] Test helm uninstall with Retain mode (default) (manual testing recommended)
- [ ] Verify no stuck resources after uninstall (manual testing recommended)
- [ ] Test multiple operator installations (different namespaces) (manual testing recommended)

#### 8.4 Update documentation
- [ ] Update docs/uninstall.md with new options (future work)
- [ ] Document drainPolicy helm value (future work)
- [ ] Document expected behavior for each mode (future work)

---

## Follow-up Improvements (Post-MVP)

These are not blocking for initial release but should be considered:

1. **Retry with patch for finalizer removal**: Replace `Update()` with `retry.RetryOnConflict()` + `Patch()` for more robust conflict handling
2. **Store purge option**: Implement Option B from Milestone 3 to leverage existing reconciliation for cleanup
3. **Watch-based StateChecker**: Use informer/watch instead of polling for instant drain detection

---

## Execution Order

Recommended order to minimize risk:

1. **M1** (Helm & CRD) - Foundation for everything
2. **M6** (Drainer Improvements) - Fix correctness issues
3. **M2** (Drain State Propagation) - Core functionality
4. **M3** (Driver/Store) - Stop new resources being created
5. **M4** (Agent Manager) - Extend to second pod
6. **M5** (Bindings Forwarder) - Extend to third pod
7. **M7** (Helm Cleanup) - Polish
8. **M8** (Testing) - Validate everything works
