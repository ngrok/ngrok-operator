# BoundEndpoint Status Improvements Plan

## Executive Summary

### What is a BoundEndpoint?

A BoundEndpoint represents one or more ngrok endpoints (tunnels/agents) that all share the same target Kubernetes service. When you run `ngrok http --url http://my-service.default --binding kubernetes 8080`, the operator creates a BoundEndpoint that projects that ngrok endpoint into the cluster as a Kubernetes Service.

Multiple ngrok agents pointing to the same `<service>.<namespace>:<port>` are aggregated into a single BoundEndpoint, since they all result in the same Kubernetes resources being created.

### Current Problems

**Status Design Issues:**
- Per-endpoint status fields exist but are misleading - all endpoints in a BoundEndpoint share the same underlying Kubernetes services, so they all have the same actual status
- Endpoints frequently get stuck showing "provisioning" status even when services are created and working
- kubectl output only shows first endpoint's status, hiding information when multiple endpoints exist
- No standard Kubernetes conditions, making it inconsistent with other CRDs and harder for tools to understand

**Critical Bugs:**
- **Bug #1 - Stuck in Provisioning:** Poller resets status to "provisioning" but controller event filter ignores status-only changes, so endpoints stay stuck in provisioning state even though services are working
- **Bug #2 - Leaked Services:** When BoundEndpoints are deleted, target services in user namespaces aren't cleaned up properly, leaving orphaned services

**Missing Information:**
- No visibility into which Kubernetes services were created
- No timestamps for state transitions
- No clear "ready" indicator
- Error messages don't surface Kubernetes API errors (namespace missing, RBAC issues, etc.)

### Desired End State

**Simplified Status Model:**
- Remove per-endpoint status/error fields since they're misleading (all endpoints share the same fate)
- Add BoundEndpoint-level conditions tracking: ServicesCreated, ConnectivityVerified, Ready
- Keep list of endpoint IDs/URIs for reference, but without individual status fields
- Add pod-like summary showing endpoint count (e.g., "2 endpoints")
- Add references to created Kubernetes services for easier debugging
- Surface actual Kubernetes API errors in condition messages

**Fixed Bugs:**
- Poller preserves existing BoundEndpoint status instead of resetting to provisioning
- Controller uses indexed lookup to properly delete services across all namespaces

**Better kubectl Experience:**
```bash
NAME                     ENDPOINTS    SERVICES    READY   AGE
my-binding              2 endpoints   ✓           True    5m
another-binding         1 endpoint    ✓           False   2m
```

Users can see at a glance how many endpoints are bound, whether services were created, and overall readiness.

---

## Current State (Technical Details)

The BoundEndpoint CRD status currently has:
- `Endpoints []BindingEndpoint` - list of ngrok endpoints with individual status/error fields
- `HashedName string` - hashed identifier
- kubectl printcolumn shows only first endpoint: `.status.endpoints[0].status`

Example status:
```yaml
status:
  endpoints:
  - id: ep_346mgYVJ5l99KxOOPVZ2zBI0Ei0
    status: provisioning
    errorCode: ""
    errorMessage: ""
  - id: ep_346mfwfBJjdPNk58gmMnQHY0yBi
    status: provisioning
    errorCode: ""
    errorMessage: ""
  hashedName: ngrok-fdc71d00-ab87-5f84-b837-b315c947a52c
```

## Issues Identified

### Design Issues

### 1. **Missing Kubernetes Conditions**
BoundEndpoint lacks standard `metav1.Condition` fields that other CRDs use (AgentEndpoint, Domain). This makes it inconsistent with K8s best practices and harder to integrate with tools that expect conditions.

### 2. **Misleading Per-Endpoint Status Fields**
Each endpoint has `status`, `errorCode`, `errorMessage` fields suggesting they have independent states. In reality, all endpoints in a BoundEndpoint share the same Kubernetes services, so they all have the same actual status. The per-endpoint status is purely operator-calculated and doesn't reflect independent states.

### 3. **Poor kubectl Display for Multiple Endpoints**
Current printcolumn only shows `endpoints[0].status`. When multiple endpoints exist, users can't see the count or overall state.

### 4. **No Service Creation Status**
Status doesn't indicate whether Target/Upstream Services were successfully created or if issues occurred (namespace missing, RBAC errors, etc.).

### 5. **No Connectivity Verification Status**
Controller runs `testBoundEndpointConnectivity()` but this isn't surfaced distinctly. Users can't differentiate between:
- Services created but not yet connectivity-tested
- Services created and connectivity verified
- Services failed to create

### 6. **Missing Resource References**
No references to the created K8s Services, making debugging harder.

### 7. **No Temporal Information**
No timestamps for when state transitions occurred or when last verified.

### Critical Bugs

### 8. **Bug #1: Status Stuck in "Provisioning"**
**Root Cause:** Race condition between poller and controller
- Poller calls `createBinding()` or `updateBinding()` and sets all endpoints to `StatusProvisioning` (lines 413, 482)
- Controller watches BoundEndpoint with event filter: `GenerationChangedPredicate{}, AnnotationChangedPredicate{}`
- Status-only changes don't trigger the event filter, so controller never runs to set status to `StatusBound`
- Most noticeable when launching multiple agents - poller overwrites existing "bound" status to "provisioning" but controller never fires to fix it

### 9. **Bug #2: Leaked Services in User Namespaces**
**Root Cause:** Delete function doesn't properly clean up target services
- `deleteBoundEndpointServices()` (lines 328-363) reconstructs service objects from spec and deletes by name
- If target namespace is deleted first, or spec is stale, target service lookup fails
- Code has an index for cross-namespace service lookup but doesn't use it in delete
- Upstream services in operator namespace are cleaned up, but target services in user namespaces leak

## Proposed Changes

### A. Simplify `BindingEndpoint` Type (Remove Per-Endpoint Status)

```go
// BindingEndpoint is a reference to an Endpoint object in the ngrok API
type BindingEndpoint struct {
    // Ref is the ngrok API reference to the Endpoint object (id, uri)
    v6.Ref `json:",inline"`

    // REMOVED: Status, ErrorCode, ErrorMessage fields
    // These were misleading since all endpoints share the same services
}
```

**Rationale:**
- All endpoints in a BoundEndpoint share the same Kubernetes services (one Target Service, one Upstream Service)
- Per-endpoint status/error fields were misleading - they all have the same actual state
- BoundEndpoint-level conditions will track the actual state of the shared resources

### B. Update `BoundEndpointStatus` Struct

```go
type BoundEndpointStatus struct {
    // Endpoints is the list of ngrok API endpoint references bound to this BoundEndpoint
    // All endpoints share the same underlying Kubernetes services
    // +kubebuilder:validation:Required
    Endpoints []BindingEndpoint `json:"endpoints"`

    // HashName is the hashed output of the TargetService and TargetNamespace
    // +kubebuilder:validation:Required
    HashedName string `json:"hashedName"`

    // NEW: EndpointsSummary provides a human-readable count of bound endpoints
    // Format: "N endpoint" or "N endpoints"
    // Examples: "1 endpoint", "2 endpoints"
    // +kubebuilder:validation:Optional
    EndpointsSummary string `json:"endpointsSummary,omitempty"`

    // NEW: Conditions represent the latest available observations of the BoundEndpoint's state
    // +kubebuilder:validation:Optional
    // +listType=map
    // +listMapKey=type
    // +kubebuilder:validation:MaxItems=8
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // NEW: TargetServiceRef references the created ExternalName Service in the target namespace
    // Follows the same pattern as AgentEndpoint/CloudEndpoint referencing Domain resources
    // +kubebuilder:validation:Optional
    TargetServiceRef *K8sObjectRefOptionalNamespace `json:"targetServiceRef,omitempty"`

    // NEW: UpstreamServiceRef references the created ClusterIP Service pointing to pod forwarders
    // +kubebuilder:validation:Optional
    UpstreamServiceRef *K8sObjectRef `json:"upstreamServiceRef,omitempty"`
}
```

**Note on Reference Types:**
- Using `K8sObjectRefOptionalNamespace` for `TargetServiceRef` since the target service is in a user-specified namespace
- Using `K8sObjectRef` for `UpstreamServiceRef` since it's always in the same namespace as the BoundEndpoint
- This follows the existing pattern: AgentEndpoint/CloudEndpoint use these same types for `DomainRef`
- These are lightweight (just name + optional namespace) vs full `corev1.ObjectReference` (which includes Kind, APIVersion, UID, etc.)

### C. Define Condition Types

Create constants for standard conditions:

```go
const (
    // ConditionTypeReady indicates the BoundEndpoint is fully operational
    ConditionTypeReady = "Ready"

    // ConditionTypeServicesCreated indicates both Target and Upstream Services were created
    ConditionTypeServicesCreated = "ServicesCreated"

    // ConditionTypeConnectivityVerified indicates connectivity test passed
    ConditionTypeConnectivityVerified = "ConnectivityVerified"
)

const (
    // Reasons for Ready condition
    ReasonBoundEndpointReady = "BoundEndpointReady"
    ReasonServicesNotCreated = "ServicesNotCreated"
    ReasonConnectivityNotVerified = "ConnectivityNotVerified"
    
    // Reasons for ServicesCreated condition
    ReasonServicesCreated = "ServicesCreated"
    ReasonServiceCreationFailed = "ServiceCreationFailed"
    
    // Reasons for ConnectivityVerified condition
    ReasonConnectivityVerified = "ConnectivityVerified"
    ReasonConnectivityFailed = "ConnectivityFailed"
)
```

### D. Update kubectl Printcolumns

Change from showing single endpoint status to showing aggregate count:

```go
// Before:
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.endpoints[0].status"

// After - show endpoints summary, services created, and ready status:
// +kubebuilder:printcolumn:name="Endpoints",type="string",JSONPath=".status.endpointsSummary"
// +kubebuilder:printcolumn:name="Services",type="string",JSONPath=".status.conditions[?(@.type==\"ServicesCreated\")].status"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status"
```

This gives output like:
```
NAME                     ENDPOINTS      SERVICES   READY   AGE
my-binding              2 endpoints     True       True    5m
another-binding         1 endpoint      False      False   2m
```

### E. Controller Changes

**Important Pattern:** Following the established pattern in Domain/AgentEndpoint/CloudEndpoint controllers:
- Sub-functions (like `createTargetService`, `createUpstreamService`) set their specific conditions on the passed BoundEndpoint object
- Top-level functions (`create()`, `update()`) call `updateStatus()` **once** at the end
- `updateStatus()` computes derived fields, calculates Ready condition, and writes to k8s API once per reconcile

#### 1. `create()` function flow
```go
func (r *BoundEndpointReconciler) create(ctx context.Context, cr *BoundEndpoint) error {
    targetService, upstreamService := r.convertBoundEndpointToServices(cr)

    // Create upstream service - sets condition on error
    if err := r.createUpstreamService(ctx, cr, upstreamService); err != nil {
        return r.updateStatus(ctx, cr, err)  // Write status once and return
    }

    // Create target service - sets condition on error
    if err := r.createTargetService(ctx, cr, targetService); err != nil {
        return r.updateStatus(ctx, cr, err)  // Write status once and return
    }

    // Both services created successfully
    setServicesCreatedCondition(cr, true, ReasonServicesCreated, "Target and Upstream services created")
    updateServiceRefs(cr, targetService, upstreamService)

    // Test connectivity
    timeoutCtx, cancel := context.WithTimeout(ctx, time.Second*10)
    defer cancel()

    err := r.testBoundEndpointConnectivity(timeoutCtx, cr)
    setConnectivityVerifiedCondition(cr, err == nil, err)

    // Write status once at the end
    return r.updateStatus(ctx, cr, err)
}
```

#### 2. `update()` function flow
Similar to create - calls sub-functions, gathers conditions, calls `updateStatus()` once at end

#### 3. `createTargetService()` / `createUpstreamService()`
- Create the service via k8s API
- On error: Set `ServicesCreatedCondition` to False with K8s error message
- On success: Return without setting condition (let `create()` set the success condition once both are done)
- **Do NOT** write status - let caller handle that

#### 3. `delete()` function - **BUG FIX #2**
- Use indexed lookup to find ALL services across namespaces
- Delete using: `List(ctx, &svcs, client.MatchingFields{BoundEndpointOwnerKey: ownerKey})`
- Do NOT scope list to a single namespace - search cluster-wide
- Iterate and delete all found services (both target and upstream)
- Make idempotent - tolerate NotFound errors

#### 4. New conditions file: `boundendpoint_conditions.go`

Following the pattern from `domain_conditions.go`, `agent_endpoint_conditions.go`, etc.

```go
package bindings

import (
    bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
    "github.com/ngrok/ngrok-operator/internal/ngrokapi"
    "k8s.io/apimachinery/pkg/api/meta"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Condition types
const (
    ConditionReady                 = "Ready"
    ConditionServicesCreated       = "ServicesCreated"
    ConditionConnectivityVerified  = "ConnectivityVerified"
)

// Condition reasons
const (
    ReasonBoundEndpointReady              = "BoundEndpointReady"
    ReasonServicesCreated                 = "ServicesCreated"
    ReasonUpstreamServiceCreationFailed   = "UpstreamServiceCreationFailed"
    ReasonTargetServiceCreationFailed     = "TargetServiceCreationFailed"
    ReasonConnectivityVerified            = "ConnectivityVerified"
    ReasonConnectivityFailed              = "ConnectivityFailed"
    ReasonServicesNotCreated              = "ServicesNotCreated"
    ReasonConnectivityNotVerified         = "ConnectivityNotVerified"
)

// setServicesCreatedCondition sets the ServicesCreated condition
func setServicesCreatedCondition(be *bindingsv1alpha1.BoundEndpoint, created bool, reason, message string) {
    status := metav1.ConditionTrue
    if !created {
        status = metav1.ConditionFalse
    }

    condition := metav1.Condition{
        Type:               ConditionServicesCreated,
        Status:             status,
        Reason:             reason,
        Message:            message,
        ObservedGeneration: be.Generation,
    }

    meta.SetStatusCondition(&be.Status.Conditions, condition)
}

// setConnectivityVerifiedCondition sets the ConnectivityVerified condition
func setConnectivityVerifiedCondition(be *bindingsv1alpha1.BoundEndpoint, verified bool, err error) {
    status := metav1.ConditionTrue
    reason := ReasonConnectivityVerified
    message := "Successfully connected to upstream service"

    if !verified {
        status = metav1.ConditionFalse
        reason = ReasonConnectivityFailed
        message = ngrokapi.SanitizeErrorMessage(err.Error())
    }

    condition := metav1.Condition{
        Type:               ConditionTypeConnectivityVerified,
        Status:             status,
        Reason:             reason,
        Message:            message,
        ObservedGeneration: be.Generation,
    }

    meta.SetStatusCondition(&be.Status.Conditions, condition)
}

// calculateReadyCondition calculates the overall Ready condition based on other conditions
func calculateReadyCondition(be *bindingsv1alpha1.BoundEndpoint) {
    // Check if services were created
    servicesCreatedCondition := meta.FindStatusCondition(be.Status.Conditions, ConditionServicesCreated)
    servicesCreated := servicesCreatedCondition != nil && servicesCreatedCondition.Status == metav1.ConditionTrue

    // Check if connectivity was verified
    connectivityCondition := meta.FindStatusCondition(be.Status.Conditions, ConditionConnectivityVerified)
    connectivityVerified := connectivityCondition != nil && connectivityCondition.Status == metav1.ConditionTrue

    // Overall ready status
    ready := servicesCreated && connectivityVerified

    // Determine reason and message based on state
    var reason, message string
    switch {
    case ready:
        reason = ReasonBoundEndpointReady
        message = "BoundEndpoint is ready"
    case !servicesCreated:
        if servicesCreatedCondition != nil {
            reason = servicesCreatedCondition.Reason
            message = servicesCreatedCondition.Message
        } else {
            reason = ReasonServicesNotCreated
            message = "Services not yet created"
        }
    case !connectivityVerified:
        if connectivityCondition != nil {
            reason = connectivityCondition.Reason
            message = connectivityCondition.Message
        } else {
            reason = ReasonConnectivityNotVerified
            message = "Connectivity not yet verified"
        }
    default:
        reason = "Unknown"
        message = "BoundEndpoint is not ready"
    }

    setReadyCondition(be, ready, reason, message)
}

// setReadyCondition sets the Ready condition
func setReadyCondition(be *bindingsv1alpha1.BoundEndpoint, ready bool, reason, message string) {
    status := metav1.ConditionTrue
    if !ready {
        status = metav1.ConditionFalse
    }

    condition := metav1.Condition{
        Type:               ConditionReady,
        Status:             status,
        Reason:             reason,
        Message:            message,
        ObservedGeneration: be.Generation,
    }

    meta.SetStatusCondition(&be.Status.Conditions, condition)
}
```

#### 5. Additional helper functions in controller

```go
// updateStatus is the single point where status is written to k8s API
// Note: Controller does NOT set EndpointsSummary - the poller owns that field
func (r *BoundEndpointReconciler) updateStatus(ctx context.Context, be *bindingsv1alpha1.BoundEndpoint, statusErr error) error {
    // Calculate overall Ready condition based on other conditions
    calculateReadyCondition(be)

    // Write status to k8s API once
    // Uses scoped merge patch to only update Conditions and service refs (fields owned by controller)
    return r.controller.ReconcileStatus(ctx, be, statusErr)
}

// updateServiceRefs sets the TargetServiceRef and UpstreamServiceRef from created services
func updateServiceRefs(be *bindingsv1alpha1.BoundEndpoint, targetSvc, upstreamSvc *v1.Service) {
    be.Status.TargetServiceRef = &ngrokv1alpha1.K8sObjectRefOptionalNamespace{
        Name:      targetSvc.Name,
        Namespace: &targetSvc.Namespace,
    }
    be.Status.UpstreamServiceRef = &ngrokv1alpha1.K8sObjectRef{
        Name: upstreamSvc.Name,
    }
}
```

### F. Poller Changes - **BUG FIX #1**

#### Status Ownership Model
**Clear division of responsibilities:**
- **Poller owns:** `status.Endpoints` list and `status.EndpointsSummary`
- **Controller owns:** `status.Conditions`, `status.TargetServiceRef`, `status.UpstreamServiceRef`
- **Why this works:** Controller event filter (`GenerationChangedPredicate`, `AnnotationChangedPredicate`) ignores status-only updates, so:
  - Poller updates Endpoints/Summary → doesn't trigger controller reconcile → no infinite loop
  - Controller updates Conditions/Refs → doesn't trigger controller reconcile → no infinite loop
  - Controller only reconciles on spec changes, which is correct behavior

**Changes to `createBinding()` (line 413):**
```go
// REMOVE these lines:
endpoint.Status = bindingsv1alpha1.StatusProvisioning
endpoint.ErrorCode = ""
endpoint.ErrorMessage = ""

// Keep only the Ref and compute summary:
toCreateStatus.Endpoints = append(toCreateStatus.Endpoints, bindingsv1alpha1.BindingEndpoint{
    Ref: desiredEndpoint.Ref,
})
// After building Endpoints list, set summary:
toCreateStatus.EndpointsSummary = computeEndpointsSummary(len(toCreateStatus.Endpoints))
```

**Changes to `updateBinding()` (line 482):**
```go
// REMOVE these lines:
endpoint.Status = bindingsv1alpha1.StatusProvisioning
endpoint.ErrorCode = ""
endpoint.ErrorMessage = ""

// Keep only the Ref and compute summary:
toUpdateStatus.Endpoints = append(toUpdateStatus.Endpoints, bindingsv1alpha1.BindingEndpoint{
    Ref: desiredEndpoint.Ref,
})
// After building Endpoints list, set summary:
toUpdateStatus.EndpointsSummary = computeEndpointsSummary(len(toUpdateStatus.Endpoints))
```

**Add helper function in poller:**
```go
// computeEndpointsSummary returns "N endpoint" or "N endpoints" string
func computeEndpointsSummary(count int) string {
    if count == 1 {
        return "1 endpoint"
    }
    return fmt.Sprintf("%d endpoints", count)
}
```

**Rationale:**
- Poller's job is to sync endpoint list from ngrok API, so it naturally owns the Endpoints list
- Poller can efficiently compute EndpointsSummary when building the list
- Controller focuses on Kubernetes resources (services, connectivity) → owns Conditions
- This eliminates the race condition where poller overwrites controller's Conditions
- No risk of infinite reconcile loops due to event filter

## Implementation Steps

1. **Update API types** (`boundendpoint_types.go`)
   - **BREAKING CHANGE:** Remove `Status`, `ErrorCode`, `ErrorMessage` from `BindingEndpoint` type
   - Add `Conditions`, `TargetServiceRef`, `UpstreamServiceRef`, `EndpointsSummary` to `BoundEndpointStatus`
   - Update kubebuilder printcolumn annotations
   - Run `make generate` and `make manifests`

2. **Update `boundendpoint_poller.go`** - **BUG FIX #1**
   - Remove lines that set `endpoint.Status = StatusProvisioning` in `createBinding()` (line 413)
   - Remove lines that set `endpoint.Status = StatusProvisioning` in `updateBinding()` (line 482)
   - Only set `Ref` field when appending endpoints to status
   - Poller should NOT set conditions or computed fields - controller owns all status

3. **Create `boundendpoint_conditions.go`** - New file following established pattern
   - Define condition type constants (Ready, ServicesCreated, ConnectivityVerified)
   - Define reason constants (BoundEndpointReady, ServicesCreated, ConnectivityVerified, etc.)
   - Implement `setServicesCreatedCondition()`
   - Implement `setConnectivityVerifiedCondition()`
   - Implement `setReadyCondition()`
   - Implement `calculateReadyCondition()` - computes Ready based on other conditions

4. **Update `boundendpoint_controller.go`** - **BUG FIX #2 + Status Improvements**
   - Update `create()` to follow pattern: create resources, set conditions, call `updateStatus()` once
   - Update `update()` similarly
   - Update `createTargetService()` to set condition on error, but not write status
   - Update `createUpstreamService()` to set condition on error, but not write status
   - **FIX delete()**: Use indexed cluster-wide service lookup by labels instead of reconstructing from spec
     - Find services by label (e.g., `ngrok.com/boundendpoint=<namespace>/<name>` or `ngrok.com/hashedName=<value>`)
     - Works even if target namespace deleted or spec changed
     - Finalizer (auto-managed by base controller) ensures cleanup happens before CR deletion
   - Add `updateStatus()` function that calls `calculateReadyCondition()` and writes once (does NOT set EndpointsSummary - poller owns that)
   - Add `updateServiceRefs()` helper
   - Remove `determineAndSetBindingEndpointStatus` and `setEndpointsStatus` functions (replaced by conditions)
   - Add log message fix: "Created Upstream Service" -> "Created Target Service" (line 217)

5. **Update tests**
   - Fix all tests that reference removed `Status`, `ErrorCode`, `ErrorMessage` fields
   - Add condition assertions to existing tests
   - Test summary computation logic
   - Test service cleanup via index
   - Test race condition scenario (multiple agents)

6. **Update documentation**
   - Document breaking change in CHANGELOG
   - Document new status fields
   - Update examples with conditions
   - Note migration path for users (existing endpoints will auto-migrate on next reconcile)

## Backward Compatibility

**Breaking Changes:**
- Removing `Status`, `ErrorCode`, `ErrorMessage` from `BindingEndpoint` type is a **breaking API change**
- Requires CRD update and may require users to recreate BoundEndpoints
- Should be noted in release notes and CHANGELOG

**Compatible Changes:**
- New fields (`Conditions`, service refs, summary) are optional
- Controller will populate them during next reconciliation
- Existing `Endpoints` array structure preserved (just without per-endpoint status fields)
- `HashedName` field unchanged

## Testing Checklist

- [ ] **Bug #1 Fix:** Launch 2 agents with same URL, verify both stay bound (not stuck in provisioning)
- [ ] **Bug #2 Fix:** Delete BoundEndpoint, verify target service in user namespace is cleaned up
- [ ] Single endpoint - verify summary shows "1 endpoint"
- [ ] Multiple endpoints - verify summary shows "N endpoints"
- [ ] Service creation failures set appropriate conditions with K8s error messages
- [ ] Connectivity test failures set appropriate conditions
- [ ] kubectl output shows summary, services status, and ready correctly
- [ ] Condition transitions have proper timestamps
- [ ] Service refs are set correctly to created services
- [ ] Missing namespace error surfaced in ServicesCreated condition
- [ ] RBAC permission error surfaced in ServicesCreated condition

## Questions Resolved

1. **Should `EndpointsSummary` be computed on-demand or stored?**
   - ✅ **Decision**: Store it as a status field - simpler and more common pattern than using condition messages

2. **Should we show the summary in a column or use conditions?**
   - ✅ **Decision**: Use dedicated `EndpointsSummary` field + column for clarity (not a condition)

3. **What should we show when namespace doesn't exist or RBAC issues occur?**
   - ✅ **Decision**: `ServicesCreated` condition False with the actual Kubernetes API error message in the condition's message field. This naturally handles all edge cases (missing namespace, RBAC, quota issues, etc.) without having to enumerate them.

4. **Service references pattern:**
   - ✅ **Confirmed**: Using `K8sObjectRefOptionalNamespace` and `K8sObjectRef` follows the existing pattern in the codebase
   - AgentEndpoint and CloudEndpoint both use `DomainRef *K8sObjectRefOptionalNamespace` to reference downstream Domain resources
   - These lightweight types are preferred over `corev1.ObjectReference` (which includes Kind, APIVersion, UID, ResourceVersion, etc.)
   - `K8sObjectRefOptionalNamespace` for TargetServiceRef (cross-namespace)
   - `K8sObjectRef` for UpstreamServiceRef (same namespace as BoundEndpoint)

5. **Status ownership and reconcile triggers:**
   - ✅ **Decision**: Poller owns `Endpoints` list + `EndpointsSummary`; Controller owns `Conditions` + service refs
   - Event filter (`GenerationChangedPredicate`, `AnnotationChangedPredicate`) ignores status-only updates → no infinite loops
   - Controller only reconciles on spec changes, which is correct

6. **Finalizers and delete cleanup:**
   - ✅ **Confirmed**: Base controller automatically adds finalizers when `statusID` function is defined
   - Delete function will use indexed lookup with labels to find services across namespaces
   - Auto-delete BoundEndpoint when Endpoints list becomes empty (existing behavior)

## Future Improvements (Post-Implementation)

These items were identified during review but are out of scope for this change to keep it minimal:

1. **Service watches for auto-healing**: Add watches on Target and Upstream Services that enqueue parent BoundEndpoint for reconciliation if services are deleted/modified out-of-band
2. **Connectivity test retry with backoff**: Add RequeueAfter in controller for transient connectivity failures so Ready condition can automatically recover
3. **Periodic health checks**: Add controller-managed periodic connectivity verification with exponential backoff
4. **Cross-namespace RBAC validation**: Pre-check RBAC permissions before attempting service creation to provide better error messages
