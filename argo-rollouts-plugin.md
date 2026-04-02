# Design Doc: Argo Rollouts Traffic Router Plugin for ngrok Operator

Status: Draft
Scope: Focused design for an Argo Rollouts traffic management plugin that integrates with the ngrok Kubernetes Operator.

---

## Background

The ngrok Kubernetes Operator translates Ingress resources into:
- **CloudEndpoint** CRDs — persistent, globally distributed edge configurations managed via the ngrok API. The CloudEndpoint owns the Traffic Policy that routes requests to internal endpoints.
- **AgentEndpoint** CRDs — in-memory tunnel sessions managed by the operator's agent deployment. Each CR creates exactly one tunnel session regardless of pod count. An AgentEndpoint has two key URLs:
  - `spec.url` — the internal URL that the CloudEndpoint's Traffic Policy forwards requests to (e.g., `https://my-svc.default.svc.cluster.local:80`)
  - `spec.upstream.url` — the Kubernetes Service URL that the tunnel forwards traffic to (e.g., `http://my-app-stable:80`)

When a user creates an Ingress with `ingressClassName: ngrok`, the operator creates:
1. A Domain CRD for the hostname
2. A CloudEndpoint CRD with a Traffic Policy that ends in a `forward-internal` action pointing at the AgentEndpoint's internal URL
3. One or more AgentEndpoint CRDs — one per route/backend service

The CloudEndpoint and AgentEndpoints are fully **derived** objects — they are recalculated and re-applied on every reconciliation cycle from the current Ingress state. The operator does not use Kubernetes OwnerReferences on these objects; instead, it tracks them by controller labels (`k8s.ngrok.com/controller-name`, `k8s.ngrok.com/controller-namespace`).

---

## Argo Rollouts Plugin Architecture

### How Traffic Router Plugins Work

Argo Rollouts plugins are standalone Go binaries that implement the `TrafficRoutingReconciler` interface. The Argo Rollouts controller starts the plugin as a long-lived child process and communicates over a Unix socket using HashiCorp's go-plugin library with Go's native `net/rpc`.

**Important constraints:**
- Plugins must be **stateless** between calls — the plugin struct is not persisted between RPC invocations. State must be reconstructed from Kubernetes resources on each call.
- Only Go plugins are supported (net/rpc protocol requires Go on both sides).
- Plugin inherits the **RBAC permissions** of the Argo Rollouts controller's service account.

### Plugin Interface

```go
type TrafficRoutingReconciler interface {
    // InitPlugin is called once when the plugin is loaded.
    // Use this to set up Kubernetes API clients/informers.
    InitPlugin() types.RpcError

    // UpdateHash informs the plugin about the new pod template hashes for the
    // canary and stable ReplicaSets. Called at the start of each reconciliation.
    UpdateHash(canaryHash, stableHash string, additionalDestinations []WeightDestination) types.RpcError

    // SetWeight is called at each canary step to shift traffic.
    // weight is 0–100, representing the percentage to route to canary.
    SetWeight(rollout *v1alpha1.Rollout, desiredWeight int32, additionalDestinations []WeightDestination) types.RpcError

    // VerifyWeight returns true if the canary is receiving the desired weight.
    // Return nil if weight verification is not supported.
    VerifyWeight(rollout *v1alpha1.Rollout, desiredWeight int32, additionalDestinations []WeightDestination) (*bool, types.RpcError)

    // SetHeaderRoute adds header-based routing rules to send matching requests to canary.
    SetHeaderRoute(rollout *v1alpha1.Rollout, setHeaderRoute *v1alpha1.SetHeaderRoute) types.RpcError

    // SetMirrorRoute configures traffic mirroring to the canary.
    SetMirrorRoute(rollout *v1alpha1.Rollout, setMirrorRoute *v1alpha1.SetMirrorRoute) types.RpcError

    // RemoveManagedRoutes cleans up all resources created by this plugin.
    // Called on rollout completion, abort, or deletion.
    RemoveManagedRoutes(rollout *v1alpha1.Rollout) types.RpcError
}
```

### Lifecycle of a Canary Rollout

```
1. User creates/updates Rollout
2. Argo creates canary ReplicaSet + two Services (stable, canary)
3. InitPlugin() — plugin sets up Kubernetes clients
4. UpdateHash(canaryHash, stableHash) — plugin learns pod hashes
5. SetWeight(5) — plugin routes 5% to canary
6. [pause / analysis steps]
7. SetWeight(20) — plugin routes 20% to canary
8. ...
9. SetWeight(100) — rollout complete, all traffic to new stable
10. RemoveManagedRoutes() — plugin cleans up its resources
```

### How Existing Plugins Manipulate Resources

Existing plugins (Istio, Gateway API, Contour) follow a consistent pattern:
- They **modify existing resources in-place** — they do not clone or create new routing resources
- The user pre-creates the routing resource (VirtualService, HTTPRoute, HTTPProxy) and the Rollout references it
- On each `SetWeight()` call, the plugin fetches the resource, updates the weight values in the backend refs, and patches it back

Example: Gateway API plugin fetches the user's HTTPRoute and updates the `weight` fields in `backendRefs` entries that point to the stable and canary Services.

**ngrok is different**: we don't have a user-managed routing resource to modify. The CloudEndpoint is derived from the Ingress by the operator's reconciler, and the relevant routing rules (the `forward-internal` actions) are written by the operator. This creates the core tension of this integration.

### Plugin Installation and Configuration

Plugins are configured in the `argo-rollouts-config` ConfigMap in the `argo-rollouts` namespace:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: argo-rollouts-config
  namespace: argo-rollouts
data:
  trafficRouterPlugins: |
    - name: "ngrok/ngrok"
      location: "https://github.com/ngrok/rollouts-plugin-trafficrouter-ngrok/releases/download/v0.1.0/plugin-linux-amd64"
      sha256: "<checksum>"
```

For local development, use `file://` location:
```yaml
      location: "file:///tmp/rollouts-plugins/ngrok-traffic-router"
```

---

## Integration Design

### Rollout CR Shape

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Rollout
metadata:
  name: my-app
  namespace: default
spec:
  replicas: 5
  selector:
    matchLabels:
      app: my-app
  template: { ... }
  strategy:
    canary:
      stableService: my-app-stable
      canaryService: my-app-canary
      trafficRouting:
        plugins:
          ngrok/ngrok:
            # Name of the CloudEndpoint the operator created from the Ingress.
            # The plugin will find the associated AgentEndpoints from this.
            cloudEndpoint: my-app
            cloudEndpointNamespace: default
      steps:
        - setWeight: 5
        - pause: {duration: 2m}
        - setWeight: 20
        - pause: {}       # manual gate
        - setWeight: 50
        - pause: {duration: 5m}
        - setWeight: 100
```

### Plugin Config Struct (parsed from Rollout spec)

```go
type NgrokTrafficRouterConfig struct {
    CloudEndpoint          string `json:"cloudEndpoint"`
    CloudEndpointNamespace string `json:"cloudEndpointNamespace"`

    // Phase 1 only: total number of AgentEndpoints in the pool.
    // Weight granularity = 1/TotalPoolSize. Default: 10.
    TotalPoolSize int `json:"totalPoolSize,omitempty"`
}
```

---

## Phase 1 — AgentEndpoint Pool Scaling (No Operator Changes Required)

### Concept

Multiple AgentEndpoints sharing the same `spec.url` (internal URL) are automatically load-balanced by ngrok across their upstreams. By controlling the ratio of canary to stable AgentEndpoints sharing a URL, we approximate traffic weight without touching the CloudEndpoint's Traffic Policy at all.

```
10 AgentEndpoints at spec.url = "https://my-svc.default.svc.cluster.local:80"
  - 2 with spec.upstream.url = "http://my-app-canary:80"  → ~20% canary
  - 8 with spec.upstream.url = "http://my-app-stable:80"  → ~80% stable
```

The CloudEndpoint Traffic Policy is **never modified** — it continues pointing at the single internal URL, and ngrok distributes across all pooled AgentEndpoints.

### SetWeight Implementation

```go
func (p *NgrokPlugin) SetWeight(rollout *v1alpha1.Rollout, desiredWeight int32, ...) types.RpcError {
    cfg := parseConfig(rollout)
    totalSessions := cfg.TotalPoolSize // default 10
    canaryCount := int(math.Round(float64(totalSessions) * float64(desiredWeight) / 100.0))
    stableCount := totalSessions - canaryCount

    internalURL := p.getInternalURLFromCloudEndpoint(cfg.CloudEndpoint, cfg.CloudEndpointNamespace)

    if err := p.scaleAgentEndpointPool("stable", rollout, internalURL, stableService, stableCount); err != nil {
        return toRpcError(err)
    }
    return toRpcError(p.scaleAgentEndpointPool("canary", rollout, internalURL, canaryService, canaryCount))
}
```

`scaleAgentEndpointPool` creates or deletes AgentEndpoint CRs named with a deterministic pattern:
`<rollout-name>-ngrok-<stable|canary>-<index>` (e.g., `my-app-ngrok-canary-0`, `my-app-ngrok-canary-1`)

Each plugin-created AgentEndpoint:
- `spec.url`: Same internal URL as the original operator-created AgentEndpoint (for pooling)
- `spec.upstream.url`: Points at either `http://my-app-stable:80` or `http://my-app-canary:80`
- Labels: Annotated with `k8s.ngrok.com/rollout-managed: "true"` and `k8s.ngrok.com/rollout-name: my-app` so `RemoveManagedRoutes` can find them

### VerifyWeight Implementation

Counts live plugin-created AgentEndpoints for each group and checks the ratio:

```go
func (p *NgrokPlugin) VerifyWeight(rollout *v1alpha1.Rollout, desiredWeight int32, ...) (*bool, types.RpcError) {
    canaryCount := countPluginEndpoints(rollout, "canary")
    stableCount := countPluginEndpoints(rollout, "stable")
    total := canaryCount + stableCount
    if total == 0 {
        return boolPtr(false), nil
    }
    actualWeight := int32(math.Round(float64(canaryCount) / float64(total) * 100))
    ok := abs(actualWeight - desiredWeight) <= toleranceForPoolSize(total)
    return &ok, nil
}
```

### RemoveManagedRoutes Implementation

```go
func (p *NgrokPlugin) RemoveManagedRoutes(rollout *v1alpha1.Rollout) types.RpcError {
    // List all AgentEndpoints with rollout-managed label for this rollout
    endpoints := listByLabel("k8s.ngrok.com/rollout-name", rollout.Name)
    for _, ep := range endpoints {
        if err := p.k8sClient.Delete(ep); err != nil {
            return toRpcError(err)
        }
    }
    return nil
}
```

After deletion, the operator's original AgentEndpoint (the one it created from the Ingress) is the only one remaining and resumes handling 100% of traffic.

### Tradeoffs

| Aspect | Detail |
|--------|--------|
| ✅ No operator changes | CloudEndpoint is never touched |
| ✅ No reconciler conflict | Plugin only creates/deletes its own AgentEndpoints |
| ✅ Rollback is instant | Delete canary CRs → tunnel sessions tear down immediately |
| ⚠️ Weight precision | Limited to `1/TotalPoolSize` granularity (default 10 = 10% steps) |
| ⚠️ Billing | N active endpoints per rollout instead of 1 |
| ⚠️ Distribution is probabilistic per session | Not exactly N% per request; ngrok distributes per-connection |
| ❌ SetHeaderRoute | Not expressible without Traffic Policy manipulation |
| ❌ SetMirrorRoute | Not expressible without Traffic Policy manipulation |

### Finding the Operator-Created AgentEndpoint (and its Internal URL)

The plugin needs to find the AgentEndpoint the operator created for the stable service to read its `spec.url` (the internal URL that the CloudEndpoint's Traffic Policy forwards to). The plugin will use this same internal URL for its pool members.

**Discovery approach**: The plugin lists AgentEndpoints in the namespace and matches by `spec.upstream.url` — the AgentEndpoint whose upstream URL contains the stable Service name is the one to target.

```go
// Pseudocode: find the operator-created AgentEndpoint for the stable service
func (p *NgrokPlugin) findStableAgentEndpoint(namespace, stableServiceName string) (*AgentEndpoint, error) {
    endpoints, _ := p.k8sClient.ListAgentEndpoints(namespace)
    for _, ep := range endpoints {
        if strings.Contains(ep.Spec.Upstream.URL, stableServiceName) {
            return &ep, nil
        }
    }
    return nil, fmt.Errorf("no AgentEndpoint found with upstream matching service %s", stableServiceName)
}
```

**Known edge case**: If multiple Ingresses or AgentEndpoints exist for the same Service (e.g., different paths on the same backend), this match is ambiguous. The long-term fix is to have the operator stamp AgentEndpoints with a label linking them to their source Ingress — something like `k8s.ngrok.com/ingress-resource: <ingress-name>` — so the plugin can filter precisely. This is a small operator change with broad traceability value beyond just this plugin. For now, service-name matching is acceptable as a v1 approach with a warning in the docs.

**The original endpoint in the pool**: Because ngrok pooling is automatic — any AgentEndpoints sharing the same `spec.url` round-robin together — the operator's original AgentEndpoint will be included in the pool alongside the plugin-created ones. This skews the weight. For example, if the plugin creates 8 stable + 2 canary endpoints (10 total), the original operator-created stable endpoint makes 11 total, resulting in ~18% canary weight instead of 20%.

**Solution**: The plugin takes ownership of the original operator-created AgentEndpoint by patching it to join the pool as one of its managed stable members. On `RemoveManagedRoutes`, it restores the original. Alternatively, the plugin simply includes the original endpoint in its total count when calculating how many additional pool members to create:

```
desired canary weight: 20% of totalPoolSize=10 → 2 canary
operator already has 1 stable endpoint
plugin creates: 2 canary + 7 stable = 9 plugin-created
total pool: 10 (9 plugin + 1 original operator stable) → 2/10 = 20% ✓
```

This is cleaner because it doesn't require the plugin to modify the operator-created endpoint.

### Open Questions for Phase 1

**OQ-1: What is the exact format of the AgentEndpoint internal URL?**

The translator generates internal URLs via `buildInternalEndpointURL()` in `translator.go:833`. The plugin-created pool members must use the exact same `spec.url` value as the operator-created endpoint to participate in the pool. What does this URL look like — is it something like `https://my-svc.default.svc.cluster.local:80` or does it use a different format? Is it stable across operator versions?

**OQ-2 (future): Operator label for Ingress-to-AgentEndpoint traceability**

Adding `k8s.ngrok.com/ingress-resource: <ingress-name>` (and possibly `k8s.ngrok.com/ingress-namespace: <namespace>`) to operator-created AgentEndpoints would eliminate the service-name matching ambiguity. This is worth tracking as a small operator enhancement regardless of this plugin, since it improves debuggability generally.

---

## Phase 2 — Traffic Policy Manipulation (Exact Weights, Header Routing)

### Concept

Instead of approximating weight via endpoint count, the plugin directly patches the forwarding section of the CloudEndpoint's Traffic Policy using `rand.double()`. This gives exact percentages and header-based routing with only 2 AgentEndpoints total (one stable, one canary).

```yaml
on_http_request:
  # [ngrok-rollout-managed] — plugin-owned block, do not edit manually
  - expressions:
      - "rand.double() <= 0.20"   # 20% to canary
    actions:
      - type: forward-internal
        config:
          url: https://my-svc-canary.default.svc.cluster.local:80
  - actions:
      - type: forward-internal
        config:
          url: https://my-svc-stable.default.svc.cluster.local:80
```

### The Reconciler Conflict Problem

The operator's reconciler **continuously** recalculates and overwrites the CloudEndpoint's Traffic Policy from the Ingress on every sync. If the plugin patches the forwarding section to add `rand.double()` routing, the next reconcile cycle overwrites it, undoing the change within seconds.

Two approaches were considered:

**Approach A (rejected): CloudEndpoint pooling**

Multiple CloudEndpoints sharing the same public URL also round-robin in ngrok. The plugin could create a second CloudEndpoint for the canary URL alongside the operator's existing one. However, this has problems:
- Getting precise weights (e.g., 20% canary) would require N CloudEndpoints in the pool, recreating the same coarse-granularity problem as Phase 1 but at higher cost (CloudEndpoints are more expensive than AgentEndpoints)
- The original CloudEndpoint still gets reconciled and could diverge

**Approach B (chosen): Annotation-based forwarding ownership transfer**

An opt-in annotation on the Ingress:

```
k8s.ngrok.com/rollout-managed: "true"
```

When this annotation is present on the Ingress:
- The operator builds the Traffic Policy prefix (user auth, rate limits, custom rules from NgrokTrafficPolicy) as normal
- It **omits** the forwarding suffix — the `forward-internal` action(s) — from what it writes
- The plugin is **always responsible** for writing the forwarding suffix

The critical requirement is that the plugin **always owns the forwarding suffix** once the annotation is set — it cannot be a partial handoff where the operator writes it at startup and the plugin takes over later. That creates a race. The contract must be:

> While `rollout-managed` annotation is present: operator never writes forwarding suffix. Plugin always writes it.

This means the plugin must handle all forwarding states including SetWeight(0) (100% stable, no canary rules).

### Annotation Lifecycle and Safe Transition

**Transition in (starting a rollout):**

```
InitPlugin() {
    1. Read current CloudEndpoint Traffic Policy (get existing forwarding suffix)
    2. Write plugin-owned forwarding suffix to CloudEndpoint (100% → stable)
       → CloudEndpoint is still valid; forwarding is unchanged
    3. Patch Ingress to add rollout-managed annotation
       → Next operator reconcile will omit forwarding suffix
       → Plugin's suffix from step 2 persists
}
```

Step 2 must happen before step 3. The window where the CloudEndpoint temporarily loses its forwarding suffix (if the operator reconciles between steps 2 and 3) is eliminated by this ordering.

**During rollout:** Every `SetWeight()` call re-writes the forwarding suffix with updated weights. Even if the operator reconciles and writes a Traffic Policy without the suffix, the Argo controller will call `SetWeight` again on its next loop, re-applying the plugin's rules.

**Transition out (rollout complete or aborted):**

```
RemoveManagedRoutes() {
    1. Write final forwarding suffix (100% → stable)
    2. Remove rollout-managed annotation from Ingress
       → Next operator reconcile re-takes ownership of forwarding suffix
       → Operator will write the same thing (forward to stable), so no traffic change
}
```

This requires an operator code change: the Traffic Policy build step in `translator.go` must check for the annotation and skip appending the forwarding suffix when it is present.

### Open Questions for Phase 2

**OQ-3: Where in the operator does the forwarding suffix get written, and is there a clean place to check the annotation?**

The operator builds the CloudEndpoint Traffic Policy in two conceptual parts:

1. **User-configured rules** — from NgrokTrafficPolicy, Ingress annotations, etc.
2. **Operator-generated forwarding** — the `forward-internal` action(s) at the end pointing at AgentEndpoint internal URLs

The Phase 2 operator change is: when `k8s.ngrok.com/rollout-managed: "true"` is present on the Ingress, skip step 2.

Questions:
- In `translator.go`, is there a single point where the forwarding suffix is appended? Or does it happen in multiple places depending on how many backends the Ingress has?
- Is the Ingress annotation accessible at the point in the code where forwarding is appended? Or would we need to thread it through as a flag?
- If a user's NgrokTrafficPolicy itself contains `forward-internal` actions, does the operator append additional forwarding or does it trust the user's policy as complete? This affects whether skipping the operator's forwarding suffix is safe when a user has their own forwarding rules.

**OQ-4: Traffic Policy JSON manipulation**

The Traffic Policy is stored as raw JSON in `CloudEndpoint.Spec.TrafficPolicy.Policy`. The plugin needs to parse it, strip the forwarding rules it previously wrote, and append new ones. Two sub-questions:
- Does the operator export Go structs for the Traffic Policy that the plugin could import and use for type-safe manipulation? Or will the plugin need to work with raw JSON / `map[string]interface{}`?
- What's a reliable way for the plugin to identify "its" rules in the policy so it can replace them without touching user rules? Options: an annotation on the CloudEndpoint marking the index boundary, or a convention that plugin-managed rules always appear last in `on_http_request`.

---

## Local Testing Setup

### Prerequisites

1. **Local Kubernetes cluster** (kind, minikube, or k3d recommended)
2. **ngrok operator installed** with valid credentials
3. **Argo Rollouts installed**:
   ```bash
   kubectl create namespace argo-rollouts
   kubectl apply -n argo-rollouts -f https://github.com/argoproj/argo-rollouts/releases/latest/download/install.yaml
   ```
4. **Argo Rollouts kubectl plugin** (optional but useful):
   ```bash
   brew install argoproj/tap/kubectl-argo-rollouts
   ```

### Step 1: Install Argo Rollouts

```bash
# Install Argo Rollouts controller
kubectl create namespace argo-rollouts
kubectl apply -n argo-rollouts -f https://github.com/argoproj/argo-rollouts/releases/latest/download/install.yaml

# Verify controller is running
kubectl -n argo-rollouts get pods
```

### Step 2: Build the Plugin Binary

```bash
# Clone the plugin repo (once created)
git clone https://github.com/ngrok/rollouts-plugin-trafficrouter-ngrok
cd rollouts-plugin-trafficrouter-ngrok

# Build for your local platform
go build -o rollouts-plugin-trafficrouter-ngrok ./cmd/main.go
chmod +x rollouts-plugin-trafficrouter-ngrok

# Place it where the Argo controller can find it
# Option A: If running Argo in-cluster, build into the controller image or use a hostPath volume
# Option B: For kind, copy into the node:
#   kind load docker-image or use extraMounts
```

### Step 3: Configure the Plugin

```bash
# Create or patch the argo-rollouts-config ConfigMap
kubectl -n argo-rollouts apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: argo-rollouts-config
  namespace: argo-rollouts
data:
  trafficRouterPlugins: |
    - name: "ngrok/ngrok"
      location: "file:///tmp/rollouts-plugins/ngrok-traffic-router"
EOF
```

For kind, mount the plugin binary into the controller node via the kind config:
```yaml
# kind-config.yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraMounts:
  - hostPath: /path/to/rollouts-plugin-trafficrouter-ngrok
    containerPath: /tmp/rollouts-plugins/ngrok-traffic-router
    readOnly: true
```

### Step 4: Grant RBAC

The plugin inherits the Argo Rollouts controller service account's permissions. Add AgentEndpoint and CloudEndpoint access:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: argo-rollouts-ngrok-plugin
rules:
- apiGroups: ["ngrok.k8s.ngrok.com"]
  resources: ["agentendpoints"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["ngrok.k8s.ngrok.com"]
  resources: ["cloudendpoints"]
  verbs: ["get", "list", "watch", "update", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: argo-rollouts-ngrok-plugin
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: argo-rollouts-ngrok-plugin
subjects:
- kind: ServiceAccount
  name: argo-rollouts
  namespace: argo-rollouts
```

### Step 5: Deploy a Test Application

```bash
# Deploy a basic app with stable and canary services
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  replicas: 4
  selector:
    matchLabels:
      app: my-app
  template:
    metadata:
      labels:
        app: my-app
    spec:
      containers:
      - name: app
        image: nginx:1.25
---
apiVersion: v1
kind: Service
metadata:
  name: my-app-stable
spec:
  selector:
    app: my-app
  ports:
  - port: 80
---
apiVersion: v1
kind: Service
metadata:
  name: my-app-canary
spec:
  selector:
    app: my-app
  ports:
  - port: 80
EOF

# Create the Ingress (operator will create CloudEndpoint + AgentEndpoint)
kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-app
spec:
  ingressClassName: ngrok
  rules:
  - host: my-app.example.ngrok.app
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: my-app-stable
            port:
              number: 80
EOF

# Wait for operator to create CloudEndpoint + AgentEndpoint
kubectl get cloudendpoints,agentendpoints
```

### Step 6: Create the Rollout

```bash
# Convert the Deployment to a Rollout
kubectl apply -f - <<EOF
apiVersion: argoproj.io/v1alpha1
kind: Rollout
metadata:
  name: my-app
spec:
  replicas: 5
  selector:
    matchLabels:
      app: my-app
  template:
    metadata:
      labels:
        app: my-app
    spec:
      containers:
      - name: app
        image: nginx:1.25
  strategy:
    canary:
      stableService: my-app-stable
      canaryService: my-app-canary
      trafficRouting:
        plugins:
          ngrok/ngrok:
            cloudEndpoint: my-app
            cloudEndpointNamespace: default
            totalPoolSize: 10
      steps:
      - setWeight: 20
      - pause: {duration: 30s}
      - setWeight: 50
      - pause: {}
      - setWeight: 100
EOF
```

### Step 7: Trigger and Observe a Canary

```bash
# Trigger a rollout by updating the image
kubectl argo rollouts set image my-app app=nginx:1.26

# Watch progress
kubectl argo rollouts get rollout my-app --watch

# Observe AgentEndpoints being created/scaled
kubectl get agentendpoints -w

# Manually promote past a pause step
kubectl argo rollouts promote my-app

# Observe plugin logs in the Argo controller
kubectl -n argo-rollouts logs deploy/argo-rollouts | grep -i ngrok
```

### Debugging Tips

- Plugin logs appear in the Argo Rollouts controller logs (not a separate process)
- If plugin binary fails to load: check file permissions (`chmod +x`) and architecture (darwin-arm64 vs linux-amd64)
- If `SetWeight` is not called: check that the Rollout's `trafficRouting.plugins` key exactly matches the plugin name in the ConfigMap
- If AgentEndpoints are not being created: check RBAC — the plugin runs as `argo-rollouts` service account
- Use `kubectl describe rollout my-app` to see rollout events and plugin errors

---

## Open Questions Summary

| # | Question | Blocking? | Phase | Status |
|---|----------|-----------|-------|--------|
| OQ-1 | What is the exact format of AgentEndpoint internal URLs and is it stable across operator versions? | Yes | P1 | Open |
| OQ-2 | Should the operator add `k8s.ngrok.com/ingress-resource` labels to AgentEndpoints for precise traceability? | No (workaround exists) | P1 | Future |
| OQ-3 | In `translator.go`, where is the `forward-internal` suffix written and is there a clean boundary from user rules? | Yes | P2 | Open |
| OQ-4 | Can the plugin safely manipulate the Traffic Policy JSON, and are there exported Go types to use? | Yes | P2 | Open |

**Resolved:**
- ✅ **AgentEndpoint discovery**: Match `spec.upstream.url` against the stable Service name. Multiple-match edge case handled by future operator label addition.
- ✅ **Original endpoint in pool**: Accounted for by subtracting 1 from the plugin-created stable count (see pool calculation above).
- ✅ **Pooling behavior**: Confirmed automatic and guaranteed. ngrok round-robins across all AgentEndpoints sharing the same `spec.url`.
- ✅ **Header routing (Phase 1)**: Not supported — requires Phase 2 Traffic Policy manipulation.

---

## What We Need From You

**For Phase 1 (unblocking work now):**
- What does an operator-created AgentEndpoint's `spec.url` look like for a typical Ingress? Something like `https://my-svc.default.svc.cluster.local:80`? This determines the exact pooling URL format the plugin uses.

**For Phase 2 (operator changes needed):**
- In `translator.go`, where is the `forward-internal` action appended to the CloudEndpoint's Traffic Policy? Is it one place or spread across branches? Is the source Ingress annotation accessible there?
- If a user attaches an NgrokTrafficPolicy that itself contains `forward-internal` actions, does the operator append more forwarding on top of that today? We need to understand this to know whether "skip operator forwarding when annotation is present" is always safe.
