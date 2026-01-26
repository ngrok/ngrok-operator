# Gateway API Multi-Operator Support - Technical Specification

## Problem Statement

The ngrok-operator currently has a **hardcoded GatewayClass controller name** that prevents running multiple operator instances in the same cluster with proper isolation.

### Current State

The controller name is defined as a constant in `internal/controller/gateway/gateway_controller.go`:

```go
const ControllerName gatewayv1.GatewayController = "ngrok.com/gateway-controller"
```

This causes the following issues:

1. **All ngrok-operator instances compete for the same GatewayClasses** - Any GatewayClass with `spec.controllerName: ngrok.com/gateway-controller` will be claimed by all operators
2. **No GatewayClass is created by Helm** - Unlike IngressClass, there is no Helm template to create a GatewayClass
3. **No Helm values exist for Gateway configuration** - Users cannot configure the controller name or class name

### Comparison with Working Ingress Setup

The Ingress API already has proper multi-operator support:

| Aspect | Ingress API | Gateway API (Current) |
|--------|-------------|----------------------|
| Controller name configurable | ✅ `ingress.controllerName` | ❌ Hardcoded constant |
| Class created by Helm | ✅ `templates/ingress-class.yaml` | ❌ No template |
| Class name configurable | ✅ `ingress.ingressClass.name` | ❌ N/A |
| Multi-operator isolation | ✅ Works | ❌ Broken |

---

## Design Solution

Mirror the Ingress pattern for Gateway API by:

1. Making the controller name configurable via Helm values and CLI flag
2. Creating a GatewayClass Helm template
3. Passing the controller name to all gateway reconcilers
4. Updating `ShouldHandleGatewayClass()` to use the configurable name

---

## Implementation Plan

### Step 1: Add Helm Values

**File:** `helm/ngrok-operator/values.yaml`

Add new values under the existing `gateway:` section (around line 342):

```yaml
## @section Kubernetes Gateway feature configuration
##
## @param gateway.enabled When true, Gateway API support will be enabled if the CRDs are detected
## @param gateway.controllerName The name of the controller to use for matching gateway classes
## @param gateway.disableReferenceGrants When true, disables required ReferenceGrants
## @param gateway.gatewayClass.name The name of the GatewayClass resource to create
## @param gateway.gatewayClass.create Whether to create the GatewayClass resource
## @param gateway.gatewayClass.default Whether to mark the GatewayClass as the default
##
gateway:
  enabled: true
  controllerName: "ngrok.com/gateway-controller"  # NEW
  disableReferenceGrants: false
  gatewayClass:                                    # NEW section
    name: ngrok
    create: true
    default: false
```

### Step 2: Create GatewayClass Helm Template

**File:** `helm/ngrok-operator/templates/gateway-class.yaml` (NEW)

Create this file modeled after `templates/ingress-class.yaml`:

```yaml
{{- if .Values.gateway.enabled }}
{{- if .Values.gateway.gatewayClass.create }}
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  labels:
    {{- include "ngrok-operator.labels" . | nindent 4 }}
    app.kubernetes.io/component: controller
  name: {{ .Values.gateway.gatewayClass.name }}
  {{- if .Values.gateway.gatewayClass.default }}
  annotations:
    gateway.networking.k8s.io/is-default-class: "true"
  {{- end }}
spec:
  controllerName: {{ .Values.gateway.controllerName }}
{{- end }}
{{- end }}
```

### Step 3: Add CLI Flag to api-manager

**File:** `cmd/api-manager.go`

#### 3a. Add field to apiManagerOpts struct (around line 93):

```go
type apiManagerOpts struct {
    // ... existing fields ...
    ingressControllerName  string
    gatewayControllerName  string  // NEW
    ingressWatchNamespace  string
    // ... rest of fields ...
}
```

#### 3b. Add flag definition (around line 160, after ingress-controller-name):

```go
c.Flags().StringVar(&opts.ingressControllerName, "ingress-controller-name", "k8s.ngrok.com/ingress-controller", "The name of the controller to use for matching ingresses classes")
c.Flags().StringVar(&opts.gatewayControllerName, "gateway-controller-name", "ngrok.com/gateway-controller", "The name of the controller to use for matching gateway classes")  // NEW
```

#### 3c. Update drainer creation (around line 340):

Change from:
```go
GatewayControllerName: string(gatewaycontroller.ControllerName),
```

To:
```go
GatewayControllerName: opts.gatewayControllerName,
```

#### 3d. Update enableGatewayFeatureSet function (around line 628):

Add at the start of the function:
```go
gatewayControllerName := gatewayv1.GatewayController(opts.gatewayControllerName)
```

Then update each reconciler to include `ControllerName: gatewayControllerName`:

```go
// GatewayClassReconciler
if err := (&gatewaycontroller.GatewayClassReconciler{
    Client:         mgr.GetClient(),
    Log:            ctrl.Log.WithName("controllers").WithName("GatewayClass"),
    Scheme:         mgr.GetScheme(),
    Recorder:       mgr.GetEventRecorderFor("gateway-class"),
    ControllerName: gatewayControllerName,  // NEW
}).SetupWithManager(mgr); err != nil {

// GatewayReconciler
if err := (&gatewaycontroller.GatewayReconciler{
    Client:         mgr.GetClient(),
    Log:            ctrl.Log.WithName("controllers").WithName("Gateway"),
    Scheme:         mgr.GetScheme(),
    Recorder:       mgr.GetEventRecorderFor("gateway-controller"),
    Driver:         driver,
    ControllerName: gatewayControllerName,  // NEW
    DrainState:     drainState,
}).SetupWithManager(mgr); err != nil {

// HTTPRouteReconciler
if err := (&gatewaycontroller.HTTPRouteReconciler{
    Client:         mgr.GetClient(),
    Log:            ctrl.Log.WithName("controllers").WithName("Gateway"),
    Scheme:         mgr.GetScheme(),
    Recorder:       mgr.GetEventRecorderFor("gateway-controller"),
    Driver:         driver,
    ControllerName: gatewayControllerName,  // NEW
    DrainState:     drainState,
}).SetupWithManager(mgr); err != nil {
```

### Step 4: Update Controller Deployment Template

**File:** `helm/ngrok-operator/templates/controller-deployment.yaml`

Add the new flag after the ingress-controller-name line (around line 126):

```yaml
        - --ingress-controller-name={{ .Values.controllerName | default .Values.ingress.controllerName }}
        - --gateway-controller-name={{ .Values.gateway.controllerName }}  # NEW
```

### Step 5: Add ControllerName Field to Reconcilers

#### 5a. GatewayClassReconciler

**File:** `internal/controller/gateway/gatewayclass_controller.go`

Update the struct (around line 44):
```go
type GatewayClassReconciler struct {
    client.Client

    Log            logr.Logger
    Scheme         *runtime.Scheme
    Recorder       record.EventRecorder
    ControllerName gatewayv1.GatewayController  // NEW
}
```

Change `ShouldHandleGatewayClass` from package function to method (around line 211):

FROM:
```go
func ShouldHandleGatewayClass(gatewayClass *gatewayv1.GatewayClass) bool {
    return gatewayClass.Spec.ControllerName == ControllerName
}
```

TO:
```go
func (r *GatewayClassReconciler) ShouldHandleGatewayClass(gatewayClass *gatewayv1.GatewayClass) bool {
    return gatewayClass.Spec.ControllerName == r.ControllerName
}
```

Update all calls to `ShouldHandleGatewayClass` in this file to use `r.ShouldHandleGatewayClass()`:
- Line ~66 in SetupWithManager predicate
- Line ~97 in Reconcile

#### 5b. GatewayReconciler

**File:** `internal/controller/gateway/gateway_controller.go`

Update the struct (around line 55):
```go
type GatewayReconciler struct {
    client.Client

    Log            logr.Logger
    Scheme         *runtime.Scheme
    Recorder       record.EventRecorder
    Driver         *managerdriver.Driver
    ControllerName gatewayv1.GatewayController  // NEW
    DrainState     controller.DrainState
}
```

Add a method at the end of the file:
```go
func (r *GatewayReconciler) shouldHandleGatewayClass(gatewayClass *gatewayv1.GatewayClass) bool {
    return gatewayClass.Spec.ControllerName == r.ControllerName
}
```

Update the call in Reconcile (around line 122) from:
```go
if !ShouldHandleGatewayClass(gwClass) {
```
to:
```go
if !r.shouldHandleGatewayClass(gwClass) {
```

#### 5c. HTTPRouteReconciler

**File:** `internal/controller/gateway/httproute_controller.go`

Update the struct (around line 54):
```go
type HTTPRouteReconciler struct {
    client.Client

    Log            logr.Logger
    Scheme         *runtime.Scheme
    Recorder       record.EventRecorder
    Driver         *managerdriver.Driver
    ControllerName gatewayv1.GatewayController  // NEW
    DrainState     controller.DrainState
}
```

Add a method at the end of the file:
```go
func (r *HTTPRouteReconciler) shouldHandleGatewayClass(gatewayClass *gatewayv1.GatewayClass) bool {
    return gatewayClass.Spec.ControllerName == r.ControllerName
}
```

Update the call in `findHTTPRouteForGateway` (around line 361) from:
```go
if !ShouldHandleGatewayClass(gwc) {
```
to:
```go
if !r.shouldHandleGatewayClass(gwc) {
```

### Step 6: Update Test Setup

**File:** `internal/controller/gateway/suite_test.go`

Update the reconciler setups (around lines 160-185) to include `ControllerName: ControllerName`:

```go
err = (&GatewayClassReconciler{
    Client:         k8sManager.GetClient(),
    Log:            logf.Log.WithName("controllers").WithName("GatewayClass"),
    Recorder:       k8sManager.GetEventRecorderFor("gatewayclass-controller"),
    Scheme:         k8sManager.GetScheme(),
    ControllerName: ControllerName,  // NEW
}).SetupWithManager(k8sManager)

err = (&GatewayReconciler{
    Client:         k8sManager.GetClient(),
    Log:            logf.Log.WithName("controllers").WithName("Gateway"),
    Scheme:         k8sManager.GetScheme(),
    Recorder:       k8sManager.GetEventRecorderFor("gateway-controller"),
    Driver:         driver,
    ControllerName: ControllerName,  // NEW
}).SetupWithManager(k8sManager)

err = (&HTTPRouteReconciler{
    Client:         k8sManager.GetClient(),
    Log:            logf.Log.WithName("controllers").WithName("HTTPRoute"),
    Scheme:         k8sManager.GetScheme(),
    Recorder:       k8sManager.GetEventRecorderFor("httproute-controller"),
    Driver:         driver,
    ControllerName: ControllerName,  // NEW
}).SetupWithManager(k8sManager)
```

---

## Verification Steps

After implementing, verify with:

```bash
# 1. Build
make generate manifests build

# 2. Run tests
go test ./internal/controller/gateway/... -v

# 3. Verify Helm template renders GatewayClass
helm template test ./helm/ngrok-operator \
  --set credentials.apiKey=test \
  --set credentials.authtoken=test \
  | grep -A15 "kind: GatewayClass"

# 4. Verify custom values work
helm template test ./helm/ngrok-operator \
  --set credentials.apiKey=test \
  --set credentials.authtoken=test \
  --set gateway.controllerName="ngrok.com/gateway-controller-team-a" \
  --set gateway.gatewayClass.name=ngrok-team-a \
  | grep -A15 "kind: GatewayClass"

# 5. Verify flag is passed to deployment
helm template test ./helm/ngrok-operator \
  --set credentials.apiKey=test \
  --set credentials.authtoken=test \
  | grep "gateway-controller-name"
```

---

## Expected Outcome

After implementation, users can run multiple ngrok-operators with Gateway API isolation:

```yaml
# Operator 1 (values.yaml)
gateway:
  controllerName: "ngrok.com/gateway-controller-team-a"
  gatewayClass:
    name: ngrok-team-a

# Operator 2 (values.yaml)  
gateway:
  controllerName: "ngrok.com/gateway-controller-team-b"
  gatewayClass:
    name: ngrok-team-b
```

Each operator will only watch GatewayClasses that match its configured `controllerName`.

---

## Files to Modify

| File | Action |
|------|--------|
| `helm/ngrok-operator/values.yaml` | Add gateway.controllerName and gateway.gatewayClass.* |
| `helm/ngrok-operator/templates/gateway-class.yaml` | **CREATE** - GatewayClass template |
| `helm/ngrok-operator/templates/controller-deployment.yaml` | Add --gateway-controller-name flag |
| `cmd/api-manager.go` | Add gatewayControllerName field, flag, and wiring |
| `internal/controller/gateway/gatewayclass_controller.go` | Add ControllerName field, update ShouldHandleGatewayClass |
| `internal/controller/gateway/gateway_controller.go` | Add ControllerName field, add shouldHandleGatewayClass method |
| `internal/controller/gateway/httproute_controller.go` | Add ControllerName field, add shouldHandleGatewayClass method |
| `internal/controller/gateway/suite_test.go` | Update test setup with ControllerName |

---

## Notes

- The `ControllerName` constant in `gateway_controller.go` can remain for backwards compatibility and as a default value
- TCPRoute and TLSRoute controllers don't directly call `ShouldHandleGatewayClass`, so they don't need updates
- The drain.go already uses `GatewayControllerName` from configuration, no changes needed there
