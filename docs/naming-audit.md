# Naming Concepts Audit

## Summary Table

| Concept | Flag/Parameter | Default Value | Purpose | Where Used | Issues |
|---------|----------------|---------------|---------|------------|--------|
| `releaseName` | `--release-name` | `"ngrok-operator"` | Helm release name, used to find the KubernetesOperator CR | All 3 managers, Helm templates | ⚠️ Passed as `{{ .Release.Name }}`, **not** `fullnameOverride` |
| `managerName` | `--manager-name` | Varies per manager | Unique identifier for this manager instance | api-manager, agent-manager, bindings-forwarder-manager | ⚠️ Unused after Drainer refactor removed controller label filtering |
| `ControllerName` / `ControllerNamespace` | Labels: `k8s.ngrok.com/controller-*` | namespace + name | Labels added to operator-managed resources (CloudEndpoint, AgentEndpoint) | `internal/controller/labels/`, `pkg/managerdriver/driver.go` | Currently **not used for filtering** in Drainer |
| `ingressControllerName` | `--ingress-controller-name` | `"k8s.ngrok.com/ingress-controller"` | Matches `IngressClass.spec.controller` | api-manager, store, Drainer | ✅ Working correctly |
| `controllerName` (Driver param) | N/A (code param) | Same as `ingressControllerName` | Used by store to match IngressClasses | `pkg/managerdriver/driver.go`, `internal/store/store.go` | Confusing name overlap |

---

## Detailed Analysis

### 1. `releaseName` / `--release-name`

**Purpose:** Identifies the Helm release name to locate the `KubernetesOperator` CR for drain state checking.

**Where used:**
- [cmd/api-manager.go#L95](file:///workspaces/ngrok-operator/cmd/api-manager.go#L95): Flag definition, default `"ngrok-operator"`
- [cmd/agent-manager.go#L70](file:///workspaces/ngrok-operator/cmd/agent-manager.go#L70): Flag definition
- [cmd/bindings-forwarder-manager.go#L64](file:///workspaces/ngrok-operator/cmd/bindings-forwarder-manager.go#L64): Flag definition
- `drain.NewStateChecker()` - uses releaseName to look up `KubernetesOperator` CR
- `createKubernetesOperator()` - uses releaseName as the CR's `metadata.name`
- Helm: `--release-name={{ .Release.Name }}`

**Helm template usage:**
```yaml
# controller-deployment.yaml:77
- --release-name={{ .Release.Name }}
```

**⚠️ Issue: `fullnameOverride` breaks this**

When user sets `fullnameOverride`:
- Deployment name becomes: `{{ fullnameOverride }}-manager`
- But `--release-name` is still `{{ .Release.Name }}` (the actual Helm release)
- The `KubernetesOperator` CR is created with `name: releaseName` (from the flag)
- **This works correctly** because the CR name comes from `--release-name`, not the deployment name

**Verdict:** ✅ Safe. The `releaseName` correctly uses `{{ .Release.Name }}` which is independent of `fullnameOverride`.

---

### 2. `managerName` / `--manager-name`

**Purpose:** Unique identifier for the manager instance, used for controller labels.

**Where used:**
- [cmd/api-manager.go#L105](file:///workspaces/ngrok-operator/cmd/api-manager.go#L105): Flag definition, default `"ngrok-ingress-controller-manager"`
- [cmd/agent-manager.go#L75](file:///workspaces/ngrok-operator/cmd/agent-manager.go#L75): Flag definition, default `"agent-manager"`  
- [cmd/bindings-forwarder-manager.go#L68](file:///workspaces/ngrok-operator/cmd/bindings-forwarder-manager.go#L68): Flag definition, default `"bindings-forwarder-manager"`

**Helm template:**
```yaml
# controller-deployment.yaml:136
- --manager-name={{ include "ngrok-operator.fullname" . }}-manager
```

**Code path:**
1. `opts.managerName` → `controllerLabels` via `labels.NewControllerLabelValues(namespace, managerName)`
2. `controllerLabels` passed to controllers like `CloudEndpointReconciler`, `AgentEndpointReconciler`
3. Labels added to created resources: `k8s.ngrok.com/controller-name`, `k8s.ngrok.com/controller-namespace`

**⚠️ Issue: Unused for filtering in Drainer**

The Drainer was refactored to use:
- `IngressControllerName` → filters Ingresses via IngressClass
- `GatewayControllerName` → filters Gateways via GatewayClass
- No longer uses `k8s.ngrok.com/controller-*` labels to filter operator-managed CRs

**Current use:** Only adds labels to CloudEndpoints and AgentEndpoints for informational purposes.

**Recommendation:** 
- **Can be removed** if only used for labeling, OR
- **Document as informational-only labels** for debugging/observability

---

### 3. `ControllerName` / `ControllerNamespace` Labels

**Purpose:** Labels added to operator-created resources to identify which operator instance manages them.

**Label keys:**
- `k8s.ngrok.com/controller-name`
- `k8s.ngrok.com/controller-namespace`

**Defined in:** [internal/controller/labels/controller.go](file:///workspaces/ngrok-operator/internal/controller/labels/controller.go)

**Where labels are applied:**
- `CloudEndpointReconciler` - via `ControllerLabels` field
- `AgentEndpointReconciler` - via `ControllerLabels` field
- `pkg/managerdriver/driver.go` - adds to created endpoints

**Where labels are NOT used:**
- **Drainer** - no longer filters by these labels
- Controllers don't filter watched resources by these labels

**⚠️ Issue: Inconsistent defaults**

| Manager | Default `--manager-name` | Label Value |
|---------|--------------------------|-------------|
| api-manager | `ngrok-ingress-controller-manager` | Used |
| agent-manager | `agent-manager` | Used |
| bindings-forwarder | `bindings-forwarder-manager` | Not used |

But Helm overrides all to `{{ fullname }}-manager`.

**Recommendation:** Align defaults or remove if unused.

---

### 4. `ingressControllerName` / `--ingress-controller-name`

**Purpose:** Value to match against `IngressClass.spec.controller` to determine which Ingresses this operator manages.

**Where used:**
- [cmd/api-manager.go#L160](file:///workspaces/ngrok-operator/cmd/api-manager.go#L160): Flag, default `"k8s.ngrok.com/ingress-controller"`
- [internal/store/store.go#L218](file:///workspaces/ngrok-operator/internal/store/store.go#L218): Filters IngressClasses
- [internal/drain/drain.go#L178](file:///workspaces/ngrok-operator/internal/drain/drain.go#L178): Filters Ingresses during drain
- Helm: `--ingress-controller-name={{ .Values.controllerName | default .Values.ingress.controllerName }}`

**Flow:**
1. User creates `IngressClass` with `spec.controller: k8s.ngrok.com/ingress-controller`
2. Store filters Ingresses to only process those using matching IngressClasses
3. Drainer only removes finalizers from matching Ingresses

**Verdict:** ✅ Working correctly. Used for Kubernetes Ingress API integration.

---

### 5. `controllerName` (Driver parameter)

**Purpose:** Same as `ingressControllerName` - passed to Driver for store filtering.

**Where used:**
- [pkg/managerdriver/driver.go#L133](file:///workspaces/ngrok-operator/pkg/managerdriver/driver.go#L133): `NewDriver(logger, scheme, controllerName, managerName, opts...)`
- Creates store with this value for IngressClass matching

**⚠️ Naming confusion:** 
- Parameter named `controllerName` but represents Ingress controller name
- Different from `managerName` which represents operator instance identity
- Different from `ControllerName` label constant

---

## What Breaks with `fullnameOverride`?

| Setting | Resource Name | Flag Value | Status |
|---------|---------------|------------|--------|
| Default | `myrelease-ngrok-operator-manager` | `--release-name=myrelease` | ✅ Works |
| `fullnameOverride=custom` | `custom-manager` | `--release-name=myrelease` | ✅ Works |
| `fullnameOverride=custom` | `custom-manager` | `--manager-name=custom-manager` | ✅ Works |

**`fullnameOverride` does NOT break anything** because:
1. `releaseName` uses `{{ .Release.Name }}` (Helm release, not fullname)
2. `managerName` uses `{{ fullname }}-manager` (follows the override)
3. KubernetesOperator CR uses `releaseName` for its name

---

## Is `managerName` Still Needed?

**Current uses:**
1. **Controller labels** on CloudEndpoints/AgentEndpoints - informational only
2. **Not used** in Drainer (removed during refactor)
3. **Not used** for watch filtering

**Recommendation:**

| Option | Pros | Cons |
|--------|------|------|
| **Keep as-is** | Informational labels help debugging | Confusing, seems important |
| **Remove entirely** | Simpler, less confusion | Lose debugging labels |
| **Document as informational** | Clear purpose | Still some code complexity |

**Suggested action:** Keep for observability, but document clearly that these labels are **informational only** and do not affect filtering or multi-operator scenarios.

---

## Recommendations

1. **Rename `controllerName` param in `NewDriver`** to `ingressControllerName` for clarity
2. **Add comments** clarifying `managerName` is for informational labels only
3. **Update defaults** in CLI to be consistent (all use `"ngrok-operator-*"` pattern)
4. **Consider removing `--manager-name` flag** if purely informational - could derive from `--release-name`
5. **Document** that `fullnameOverride` is safe to use

---

## Code Location Summary

| File | Relevant Concepts |
|------|-------------------|
| `cmd/api-manager.go` | `releaseName`, `managerName`, `ingressControllerName` |
| `cmd/agent-manager.go` | `releaseName`, `managerName` |
| `cmd/bindings-forwarder-manager.go` | `releaseName`, `managerName` |
| `internal/controller/labels/controller.go` | `ControllerName`, `ControllerNamespace` constants |
| `internal/drain/drain.go` | `IngressControllerName`, `GatewayControllerName` |
| `pkg/managerdriver/driver.go` | `controllerName`, `managerName`, `controllerLabels` |
| `helm/ngrok-operator/templates/controller-deployment.yaml` | All Helm flag mappings |
