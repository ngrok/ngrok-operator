# Drain Feature Follow-up Plan

This document tracks code review items and next steps for the operator uninstall drain feature.

---

## Status Legend
- ⏳ Pending
- 🔄 In Progress
- ✅ Completed
- ❓ Needs Discussion

---

## 1. API/CRD Changes

### 1.1 Remove `spec.drain.enabled` field ✅
**Priority:** High  
**Rationale:** There's no need for a second way to trigger drain mode. Deleting the CR triggers drain, and `status.drainStatus` persists the state.

**Current State:**
- `DrainConfig.Enabled` exists in [`api/ngrok/v1alpha1/kubernetesoperator_types.go:165`](file:///workspaces/ngrok-operator/api/ngrok/v1alpha1/kubernetesoperator_types.go#L165)
- `StateChecker.IsDraining()` checks three conditions: DeletionTimestamp, spec.drain.enabled, AND status.drainStatus

**Changes Required:**
1. Remove `Enabled bool` field from `DrainConfig` struct
2. Update `StateChecker.IsDraining()` in [`internal/drain/state.go`](file:///workspaces/ngrok-operator/internal/drain/state.go) to only check:
   - `!ko.DeletionTimestamp.IsZero()` (CR is being deleted)
   - `ko.Status.DrainStatus == DrainStatusDraining` (already draining)
3. Update helm cleanup hook job to just delete the CR (no patch needed)
4. Remove the "preserve Enabled if already set" logic from `createKubernetesOperator()` in [`cmd/api-manager.go:796-800`](file:///workspaces/ngrok-operator/cmd/api-manager.go#L796-L800)
5. Update documentation in [`docs/uninstall.md`](file:///workspaces/ngrok-operator/docs/uninstall.md)
6. Regenerate manifests with `make generate manifests`

**Files to Modify:**
- `api/ngrok/v1alpha1/kubernetesoperator_types.go`
- `internal/drain/state.go`
- `internal/drain/state_test.go` (update tests)
- `helm/ngrok-operator/templates/cleanup-hook/job.yaml`
- `cmd/api-manager.go`
- `docs/uninstall.md`

---

## 2. Naming Cleanup

### 2.1 Document and fix naming wiring ✅
**Priority:** Medium  
**Rationale:** The naming is confusing and some wiring appears incorrect. Need a thorough audit.

**Current Helm Template Values:**

| Manager | `--release-name` | `--manager-name` |
|---------|------------------|------------------|
| api-manager | `{{ .Release.Name }}` | `{{ fullname }}-manager` |
| agent-manager | `{{ .Release.Name }}` | `{{ fullname }}-agent-manager` |
| bindings-forwarder | `{{ .Release.Name }}` | `{{ fullname }}-bindings-forwarder` |

Where `fullname` = `fullnameOverride` OR `Release.Name-Chart.Name` (truncated).

**Conceptual Purposes:**

| Name | Purpose | Used For |
|------|---------|----------|
| `releaseName` | KubernetesOperator CR name | StateChecker lookup, CR create/update |
| `managerName` | Resource ownership labels (`k8s.ngrok.com/controller-name`) | Multi-instance resource separation |
| `ingressControllerName` | IngressClass.spec.controller matching | Ingress filtering in store |

**Problems Identified:**

1. **KubernetesOperatorReconciler** receives `ControllerName: opts.managerName` at line 393 - confusing param name
2. **createKubernetesOperator** uses `opts.releaseName` for CR name - is this correct? Should it use fullname?
3. **Drainer** had `ControllerName`/`ControllerNamespace` for label selectors, but user resources don't have these labels - this code should be removed
4. **Potential bug:** If user sets `fullnameOverride`, the CR name (releaseName) differs from fullname-based labels

**Action Required:**
Use a sub-agent to audit all usages of:
- `opts.releaseName` / `--release-name`
- `opts.managerName` / `--manager-name`  
- `ControllerName` / `ControllerNamespace` params
- `controllerLabels` usage

**Questions to Answer:**
- Should the KubernetesOperator CR name be `releaseName` or `fullname`?
- Is managerName even needed if we're not using controller labels for filtering?
- What breaks if user sets `fullnameOverride`?

**Files to Audit:**
- `cmd/api-manager.go`
- `cmd/agent-manager.go`
- `cmd/bindings-forwarder-manager.go`
- `internal/controller/ngrok/kubernetesoperator_controller.go`
- `internal/drain/drain.go`
- `internal/drain/state.go`
- `pkg/managerdriver/driver.go`
- `helm/ngrok-operator/templates/*.yaml`

---

## 3. RBAC Changes

### 3.1 Move kubebuilder RBAC tags to drain package ✅
**Priority:** Medium  
**Rationale:** The RBAC tags are for resources the Drainer touches. They should live in the drain package for clarity.

**Current State:**
Tags at lines 126-137 of [`kubernetesoperator_controller.go`](file:///workspaces/ngrok-operator/internal/controller/ngrok/kubernetesoperator_controller.go#L126-L137) add RBAC for all resource types the drainer touches.

**Problem:**
`CONTROLLER_GEN_PATHS` in [`tools/make/_common.mk`](file:///workspaces/ngrok-operator/tools/make/_common.mk) is:
```
CONTROLLER_GEN_PATHS = {./api/..., ./internal/controller/...}
```
This does NOT include `./internal/drain/...`, so moving RBAC tags there won't work without updating the path.

**Changes Required:**
1. Update `CONTROLLER_GEN_PATHS` to include `./internal/drain/...`
2. Move RBAC tags from `kubernetesoperator_controller.go` to `drain.go`
3. Run `make manifests` to verify RBAC is still generated correctly

**Files to Modify:**
- `tools/make/_common.mk` - Add drain path
- `internal/controller/ngrok/kubernetesoperator_controller.go` - Remove RBAC tags
- `internal/drain/drain.go` - Add RBAC tags

---

## 4. Helm Structure

### 4.1 Consider nesting drainPolicy under cleanupHook ⏳
**Priority:** Low  
**Status:** Leave as TODO, don't change now.

**Note:** May want to move drainPolicy under cleanupHook in a future release as a breaking change.

---

## 5. Code Cleanup

### 5.1 DRY up endpoint poller drain handling ✅
**Priority:** Low  
**Rationale:** The `BoundEndpointPoller` has repetitive `reconcilingCancel` handling when draining.

**Current Code in [`boundendpoint_poller.go`](file:///workspaces/ngrok-operator/internal/controller/bindings/boundendpoint_poller.go):**
- Line 161-167: Checks draining, cancels, returns
- Line 187-193: Checks draining, cancels, returns

**Proposed Fix:**
Extract a helper method that checks draining and cancels context if needed.

### 5.2 Add helper for nil DrainConfig check ✅
**Priority:** Low  
**Rationale:** Throughout the codebase, we check `if ko.Spec.Drain != nil && ko.Spec.Drain.Policy != ""`. Could use helper.

**Proposed Fix:**
Add a method `(ko *KubernetesOperator) GetDrainPolicy() DrainPolicy` that returns the policy or default.

### 5.3 Move status handling from handleDrain to drainer package - SKIPPED
**Priority:** Low  
**Rationale:** `handleDrain()` in the controller does a lot of status updates that could be encapsulated.

**Decision:** Skipped. Moving status updates to the Drainer would:
- Couple the drain package more tightly to Kubernetes client and event recorder
- Add complexity without significant benefit
- Current separation (controller handles status, Drainer handles draining) is reasonable

---

## 6. Util Exports

### 6.1 Remove unnecessary exports from internal/controller/util.go ✅
**Priority:** Low  
**Rationale:** The util functions are re-exported from `internal/util`. New code should import `internal/util` directly.

**Changes Required:**
1. Update `drain_test.go` to import `internal/util` instead of `internal/controller`
2. Audit other packages and update imports
3. Add deprecation comment to `internal/controller/util.go`

---

## 7. Driver/Store Integration

### 7.1 Store deletion during drain - VERIFIED OK ✅
**Status:** No changes needed.

**Analysis:**
- Store is in-memory and gets torn down when operator finishes uninstall
- Driver.Sync() is no-op during drain, so deleting from store doesn't trigger downstream deletes
- On re-install, driver re-hydrates from cluster resources
- Gateway controller also deletes from store during drain (consistent pattern)

---

## 8. Drainer Improvements

### 8.1 DRY up DrainAll function ⏳
**Priority:** Low  
**Status:** Current structure is reasonable. Different resource types genuinely need different handling.

### 8.2 DrainAll namespace/class filtering ✅
**Priority:** HIGH - REQUIRED FOR MERGE  
**Rationale:** In multi-operator setups with Delete policy, we currently delete ALL resources with finalizers, which could delete resources managed by other operators.

**Current Behavior:**
- For Retain mode: Works OK - other operators will re-add finalizers to their resources
- For Delete mode: BROKEN - deletes resources from other operators' IngressClasses/GatewayClasses

**Problem Scenario:**
1. Operator A manages IngressClass "ngrok-a" with `drainPolicy: Delete`
2. Operator B manages IngressClass "ngrok-b"  
3. User runs `helm uninstall operator-a`
4. Drainer deletes ALL Ingresses with finalizers, including those for "ngrok-b"

**Solution: Pass filter configs to Drainer**

Pass namespace and class configurations to the Drainer. Drainer can query IngressClass/GatewayClass resources to get the class names matching our controller. This avoids circular dependency with Store while keeping filtering logic self-contained.

If there's significant duplication with Store filtering logic, we can extract shared helpers later.

**Changes Required:**

1. **Remove controller label usage from Drainer** - User resources don't have `k8s.ngrok.com/controller-*` labels
2. **Add filter fields to Drainer:**
   ```go
   type Drainer struct {
       Client              client.Client
       Log                 logr.Logger
       Policy              ngrokv1alpha1.DrainPolicy
       // Filtering
       WatchNamespace        string   // If set, only drain in this namespace
       IngressControllerName string   // Used to find matching IngressClasses
       GatewayControllerName string   // Used to find matching GatewayClasses (e.g., "ngrok.com/gateway-controller")
   }
   ```
3. **Add helper to get class names:**
   ```go
   func (d *Drainer) getIngressClassNames(ctx context.Context) ([]string, error)
   func (d *Drainer) getGatewayClassNames(ctx context.Context) ([]string, error)
   ```
4. **Update drain functions to filter:**
   - `drainIngresses()` - Filter by namespace + ingressClassName
   - `drainGateways()` - Filter by gatewayClassName  
   - `drainHTTPRoutes/TCPRoutes/TLSRoutes()` - Filter by parent Gateway's class
   - `drainServices()` - Filter by namespace (if set)

**Files to Modify:**
- `internal/drain/drain.go` - Add filters, remove controller label usage, add class name helpers
- `internal/controller/ngrok/kubernetesoperator_controller.go` - Pass filters when creating Drainer
- `cmd/api-manager.go` - Wire up watchNamespace and controllerNames to KubernetesOperatorReconciler
- `internal/drain/drain_test.go` - Add tests for filtering

---

## 9. Documentation

### 9.1 Keep uninstall.md updated ✅
**Priority:** Medium  
**Rationale:** The in-repo `docs/uninstall.md` serves as comprehensive context for AI agents and developers. Keep it updated with implementation details.

**Note:** Public-facing docs (local vs prod recommendations, behavior matrix) will be handled separately in the docs site repo.

---

## Execution Order

Based on priority and dependencies:

### Phase 1: Required for Merge ✅
1. **1.1 Remove spec.drain.enabled** ✅ - API simplification  
2. **8.2 DrainAll namespace/class filtering** ✅ - Correctness fix for multi-operator
3. **Remove controller label filtering from Drainer** ✅ - Part of 8.2, user resources don't have these labels

### Phase 2: Should Do ✅
4. **2.1 Naming audit** ✅ - Created docs/naming-audit.md with findings
5. **3.1 Move RBAC tags to drain package** ✅ - Cleaner code organization
6. **9.1 Keep uninstall.md updated** ✅ - Updated throughout implementation

### Phase 3: Nice to Have ✅
7. **5.x Code cleanup items** ✅ - Added `cancelIfDraining` helper, `GetDrainPolicy()` method
8. **6.1 Util exports cleanup** ✅ - Added deprecation comment, updated imports

---

## Implementation Notes for Phase 1

### Task 1.1: Remove spec.drain.enabled

**Steps:**
1. Edit `api/ngrok/v1alpha1/kubernetesoperator_types.go`:
   - Remove `Enabled bool` from `DrainConfig`
2. Edit `internal/drain/state.go`:
   - Remove `(ko.Spec.Drain != nil && ko.Spec.Drain.Enabled)` condition
3. Edit `internal/drain/state_test.go`:
   - Remove/update tests for `spec.drain.enabled`
4. Edit `helm/ngrok-operator/templates/cleanup-hook/job.yaml`:
   - Remove the patch command, just do delete directly
5. Edit `cmd/api-manager.go`:
   - Simplify `createKubernetesOperator()` - remove special handling for Enabled
6. Update `docs/uninstall.md`:
   - Remove references to manual drain trigger via patch
   - Update "Drain Mode" section to only mention deletion trigger
7. Run `make generate manifests`

### Task 8.2: DrainAll Filtering

**Steps:**
1. **Remove controller label filtering from Drainer:**
   - Remove `ControllerNamespace` and `ControllerName` fields from Drainer struct
   - Remove `selector` parameter from drain functions that don't need it
   - Remove `labels.ControllerLabelSelector()` usage

2. **Add new filter fields to Drainer:**
   ```go
   type Drainer struct {
       Client                client.Client
       Log                   logr.Logger
       Policy                ngrokv1alpha1.DrainPolicy
       WatchNamespace        string  // If set, only drain in this namespace
       IngressControllerName string  // e.g., "k8s.ngrok.com/ingress-controller"
       GatewayControllerName string  // e.g., "ngrok.com/gateway-controller" (future)
   }
   ```

3. **Add class name helpers:**
   ```go
   // getIngressClassNames returns names of IngressClasses managed by this controller
   func (d *Drainer) getIngressClassNames(ctx context.Context) ([]string, error) {
       var classes netv1.IngressClassList
       if err := d.Client.List(ctx, &classes); err != nil {
           return nil, err
       }
       var names []string
       for _, class := range classes.Items {
           if class.Spec.Controller == d.IngressControllerName {
               names = append(names, class.Name)
           }
       }
       return names, nil
   }
   ```

4. **Update drain functions:**
   - `drainIngresses()`: Filter by namespace (if set) + ingressClassName in allowed list
   - `drainGateways()`: Filter by gatewayClassName in allowed list
   - `drainHTTPRoutes/TCPRoutes/TLSRoutes()`: Filter by parent Gateway's class
   - `drainServices()`: Filter by namespace (if set)

5. **Wire up in controller/cmd:**
   - Add `WatchNamespace` and `IngressControllerName` to `KubernetesOperatorReconciler`
   - Pass from `cmd/api-manager.go` using existing `opts.ingressWatchNamespace` and `opts.ingressControllerName`

6. **Add tests** for filtering scenarios

---

## Resolved Questions

1. **drainPolicy location:** Leave as-is (top-level), add as future TODO
2. **Multi-instance filtering:** YES, required - pass configs to Drainer, query class names dynamically
3. **Naming consolidation:** Needs audit first via sub-agent (2.1)
4. **Store deletion during drain:** OK - verified store is in-memory and Sync is no-op
5. **Controller labels in Drainer:** Remove - user resources don't have these labels
