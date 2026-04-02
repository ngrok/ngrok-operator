# ngrok Argo Rollouts Traffic Router Plugin

A [traffic router plugin](https://argoproj.github.io/argo-rollouts/features/traffic-management/plugins/) for [Argo Rollouts](https://argoproj.github.io/argo-rollouts/) that integrates with the [ngrok Kubernetes Operator](https://github.com/ngrok/ngrok-operator) to enable progressive delivery (canary and blue/green deployments) through ngrok endpoints.

---

## How it works

The ngrok Kubernetes Operator translates `Ingress` resources into ngrok `AgentEndpoint` CRDs — in-memory tunnel sessions that proxy traffic from the ngrok edge to in-cluster Services. The plugin controls traffic weight by manipulating these CRDs.

Two operating modes are available:

### Phase 1 — AgentEndpoint pool scaling (default)

Multiple `AgentEndpoint` CRDs sharing the same public URL are automatically load-balanced by ngrok across all of their upstreams. The plugin creates a pool of stable and canary endpoints at the right ratio to approximate the desired weight.

```
totalPoolSize = 10, setWeight(30):
  3 canary AgentEndpoints  → upstream: my-app-canary  → ~30% of requests
  7 stable AgentEndpoints  → upstream: my-app-stable  → ~70% of requests
```

- **No operator changes required** — the `CloudEndpoint` is never touched
- **Weight precision** is limited by pool size (`1/totalPoolSize` granularity; default 10 = 10% steps)
- **Billing**: N active tunnel sessions per rollout instead of 1

### Phase 2 — Traffic Policy routing (exact weights)

The plugin injects a `rand.double()` expression into the ngrok Traffic Policy on the operator-created endpoint and creates a single canary `AgentEndpoint` with an internal URL.

Phase 2 operates in two sub-modes, detected automatically:

#### Phase 2a — Collapsed mode (single-backend Ingress)

The operator creates one `AgentEndpoint` with the public URL. The plugin injects the canary rule into that endpoint's inline traffic policy. Stable traffic falls through to `spec.upstream.url` implicitly.

```yaml
# Plugin-owned Traffic Policy rule on the stable AgentEndpoint
on_http_request:
  - name: ngrok-rollout-canary
    expressions:
      - "rand.double() <= 0.3000"   # 30% to canary
    actions:
      - type: forward-internal
        config:
          url: https://my-app-ngrok-p2-canary-default.internal
# Remaining 70% → spec.upstream.url (stable service)
```

#### Phase 2b — CloudEndpoint (verbose) mode — recommended for production

When the Ingress is annotated with `k8s.ngrok.com/mapping-strategy: "endpoints-verbose"`, the operator creates a `CloudEndpoint` (public URL + traffic policy) that forwards to internal `AgentEndpoints`. The plugin operates on the `CloudEndpoint`'s traffic policy:

```yaml
# Plugin-owned Traffic Policy on the CloudEndpoint
on_http_request:
  # [user prefix rules: auth, rate-limit, etc. — preserved from original policy]
  - name: ngrok-rollout-canary
    expressions:
      - "rand.double() <= 0.3000"   # 30% to canary internal AEP
    actions:
      - type: forward-internal
        config:
          url: https://my-app-ngrok-p2-canary-default.internal
  - name: ngrok-rollout-stable
    actions:                        # 70% to stable internal AEP
      - type: forward-internal
        config:
          url: https://my-app-stable.internal
```

- **User prefix rules** (auth, IP restrictions, rate limits) are captured from the original policy and preserved on every SetWeight call
- **Explicit stable routing** — the stable `forward-internal` rule is part of the policy, making the routing path transparent and easy to inspect
- **Requires `endpoints-verbose` annotation** on the Ingress:
  ```yaml
  annotations:
    k8s.ngrok.com/mapping-strategy: "endpoints-verbose"
  ```

Both sub-modes:
- **Exact weights** at any percentage
- **Only 2 tunnel sessions** per rollout (one stable, one canary)
- **Require operator support**: the plugin sets a `k8s.ngrok.com/rollout-managed: "true"` annotation on the managed endpoint, which tells the operator's reconciler to preserve the traffic policy rather than overwriting it on each sync

Enable Phase 2 in the Rollout config:
```yaml
ngrok/ngrok:
  useTrafficPolicy: true
```

---

## Feature support

| Feature | Phase 1 | Phase 2 | Notes |
|---------|---------|---------|-------|
| Canary rollouts | ✅ | ✅ | |
| Blue/green rollouts | ✅ | ✅ | Uses same SetWeight interface |
| `setWeight` steps | ✅ (±1 slot) | ✅ (exact) | |
| `setHeaderRoute` | ❌ | 🚧 planned | Requires Traffic Policy manipulation |
| `setMirrorRoute` | ❌ | ❌ | ngrok does not support traffic mirroring |
| `verifyWeight` | ✅ | ✅ | |
| Rollback (`abort`) | ✅ instant | ✅ instant | Pool teardown / policy revert |

---

## Local development setup

> **Note:** This guide uses a local build of the ngrok operator because Phase 2 requires
> operator changes (the `rollout-managed` annotation support) that are not yet released.
> Once those changes are published, the operator can be installed via Helm instead.

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/)
- [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [Argo Rollouts kubectl plugin](https://argoproj.github.io/argo-rollouts/installation/#kubectl-plugin-installation) (optional but useful for `kubectl argo rollouts` commands)
- A [ngrok account](https://dashboard.ngrok.com/) with an API key and authtoken
- A reserved ngrok domain (e.g., `bezek-rollout-demo.ngrok.app`) for the demo endpoint

### 1. Clone and set up the operator repo

```bash
git clone https://github.com/ngrok/ngrok-operator
cd ngrok-operator
```

### 2. Create a kind cluster and deploy the operator

```bash
# Create the cluster
make kind-create

# Build and deploy the operator (uses local build with rollout-managed annotation support)
export NGROK_API_KEY=<your-api-key>
export NGROK_AUTHTOKEN=<your-authtoken>
make deploy
```

### 3. Install Argo Rollouts

```bash
make argo-rollouts-install
```

This creates the `argo-rollouts` namespace, installs the controller, and applies the ConfigMap that points the controller at the ngrok plugin binary.

### 4. Build and deploy the plugin

```bash
make plugin-deploy
```

This builds the plugin binary for `linux/amd64`, packages it into a Docker image, loads it into the kind cluster, patches the Argo Rollouts controller Deployment to copy the binary via an init container, and restarts the controller.

### 5. Deploy the demo app

```bash
make argo-rollouts-example-apply ROLLOUT_DEMO_HOST=bezek-rollout-demo.ngrok.app
```

Wait ~15 seconds for the operator to create the endpoints and the tunnel to come online:

```bash
# Wait for READY=True on the CloudEndpoint
kubectl get cloudendpoints -n rollout-demo -w
```

Then verify `stable-v1` is reachable:
```bash
curl https://bezek-rollout-demo.ngrok.app
# → stable-v1
```

### 6. Trigger a canary rollout

Open **three terminals** to watch everything at once.

**Terminal 1 — continuous traffic stream:**
```bash
watch -n 0.5 "curl -s https://bezek-rollout-demo.ngrok.app"
# or without watch:
while true; do curl -s https://bezek-rollout-demo.ngrok.app; echo; sleep 0.5; done
```

**Terminal 2 — live endpoint state:**
```bash
# Watch AgentEndpoints and CloudEndpoints appear/disappear
kubectl get agentendpoints,cloudendpoints -n rollout-demo -w
```

**Terminal 3 — live traffic policy on the CloudEndpoint:**
```bash
# Re-prints the CloudEndpoint's traffic policy every 2s
watch -n 2 "kubectl get cloudendpoint -n rollout-demo -o jsonpath='{.items[0].spec.trafficPolicy}' | python3 -m json.tool"
```

Now trigger the canary in a fourth terminal (or your main shell):

```bash
make argo-rollouts-example-update
```

**What you'll see:**

1. **Terminal 2**: a second `AgentEndpoint` (`rollout-demo-ngrok-p2-canary`) appears with an `.internal` URL
2. **Terminal 3**: the CloudEndpoint policy gains a `rand.double() <= 0.2500` canary rule
3. **Terminal 1**: responses flip between `stable-v1` (~75%) and `canary-v2` (~25%)

The rollout progresses automatically every 30s: `25% → 50% → 75% → done`. **Terminal 3** shows the threshold climbing as each step fires.

Once all steps complete, **Terminal 2** shows the canary endpoint disappear, **Terminal 1** shows 100% `canary-v2` (the promoted version is now stable), and **Terminal 3** shows the routing rules cleaned up.

**To abort mid-rollout:**
```bash
make argo-rollouts-example-abort
```
The canary endpoint disappears immediately and **Terminal 1** drops back to 100% `stable-v1`.

### 7. Tear down

```bash
make argo-rollouts-example-delete   # removes the demo app and RBAC
make argo-rollouts-uninstall        # removes Argo Rollouts
```

---

## Rollout configuration reference

### Ingress (CloudEndpoint mode — recommended)

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-app
  namespace: default
  annotations:
    # endpoints-verbose forces the operator to create a CloudEndpoint + internal AgentEndpoints.
    # This is the recommended setup for production as it allows the plugin to preserve
    # user-authored Traffic Policy rules (auth, rate limits, etc.) across rollouts.
    k8s.ngrok.com/mapping-strategy: "endpoints-verbose"
spec:
  ingressClassName: ngrok
  rules:
  - host: my-app.ngrok.app
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: my-app-stable  # points at the stable service
            port:
              number: 80
```

### Rollout

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Rollout
metadata:
  name: my-app
  namespace: default
spec:
  replicas: 4
  selector:
    matchLabels:
      app: my-app
  template: { ... }
  strategy:
    canary:
      stableService: my-app-stable   # Argo pins this selector to the stable ReplicaSet
      canaryService: my-app-canary   # Argo pins this selector to the canary ReplicaSet
      trafficRouting:
        plugins:
          ngrok/ngrok:
            # useTrafficPolicy: true  → Phase 2: exact rand.double() routing (recommended)
            # useTrafficPolicy: false → Phase 1: AgentEndpoint pool scaling (default)
            useTrafficPolicy: true

            # totalPoolSize: only used in Phase 1.
            # Controls weight granularity: 10 = 10% steps, 20 = 5% steps.
            # The operator-created endpoint counts as one slot.
            totalPoolSize: 10

            # cloudEndpoint / cloudEndpointNamespace: optional explicit CloudEndpoint name.
            # Only needed if auto-discovery fails (e.g. multiple CloudEndpoints in the namespace).
            # cloudEndpoint: my-app.ngrok.app
            # cloudEndpointNamespace: default
      steps:
        - setWeight: 25
        - pause: {duration: 30s}
        - setWeight: 50
        - pause: {duration: 30s}
        - setWeight: 75
        - pause: {duration: 30s}
```

The plugin name `ngrok/ngrok` must match the `name` field in the Argo Rollouts ConfigMap.

---

## Architecture

### Phase 2a — Collapsed mode

```
                    ┌───────────────────────────────────────────┐
                    │           Argo Rollouts controller          │
                    │  ┌─────────────────────────────────────┐  │
                    │  │   ngrok traffic router plugin        │  │
                    │  └──────────────┬──────────────────────┘  │
                    └─────────────────┼─────────────────────────┘
                                      │ Kubernetes API
                    ┌─────────────────▼─────────────────────────┐
                    │             Kubernetes cluster              │
                    │                                             │
                    │  AgentEndpoint (operator-created)           │
                    │    spec.url: https://my-app.ngrok.app       │
                    │    spec.upstream: my-app-stable:80          │
                    │    spec.trafficPolicy:                      │
                    │      rand.double() <= 0.25 → canary.internal│
                    │      (else → spec.upstream, stable)         │
                    │                                             │
                    │  AgentEndpoint (plugin-created)             │
                    │    spec.url: https://...-canary.internal    │
                    │    spec.upstream: my-app-canary:80          │
                    └─────────────────────────────────────────────┘
```

### Phase 2b — CloudEndpoint (verbose) mode

```
                    ┌───────────────────────────────────────────┐
                    │           Argo Rollouts controller          │
                    │  ┌─────────────────────────────────────┐  │
                    │  │   ngrok traffic router plugin        │  │
                    │  └──────────────┬──────────────────────┘  │
                    └─────────────────┼─────────────────────────┘
                                      │ Kubernetes API
                    ┌─────────────────▼─────────────────────────┐
                    │             Kubernetes cluster              │
                    │                                             │
                    │  CloudEndpoint (operator-created)           │
                    │    spec.url: https://my-app.ngrok.app       │
                    │    spec.trafficPolicy:                      │
                    │      [user auth/rate-limit rules]           │
                    │      rand.double() <= 0.25 → canary.internal│
                    │      (unconditional) → stable.internal      │
                    │                                             │
                    │  AgentEndpoint (operator-created, stable)   │
                    │    spec.url: https://...-stable.internal    │
                    │    spec.upstream: my-app-stable:80          │
                    │                                             │
                    │  AgentEndpoint (plugin-created, canary)     │
                    │    spec.url: https://...-canary.internal    │
                    │    spec.upstream: my-app-canary:80          │
                    └─────────────────────────────────────────────┘
```

The plugin uses the Kubernetes API directly (no direct ngrok API calls). It creates and manages `AgentEndpoint` CRDs, which the ngrok operator's agent deployment syncs to active tunnel sessions.

The `k8s.ngrok.com/rollout-managed: "true"` annotation on the operator-created endpoint (AgentEndpoint or CloudEndpoint) signals the operator reconciler to preserve the plugin-owned traffic policy rather than overwriting it on each sync. The plugin removes this annotation when the rollout completes or is aborted, at which point the operator restores its own routing rules.

### RBAC

The plugin runs as a child process of the Argo Rollouts controller and inherits its service account. The `ClusterRole` in `example/rbac.yaml` grants the additional permissions needed:

- `agentendpoints`: get, list, watch, create, update, patch, delete
- `cloudendpoints`: get, list, watch, update, patch
- `ingresses`: get, list, watch, patch (Phase 2 cleanup)

---

## Makefile reference

| Target | Description |
|--------|-------------|
| `plugin-build` | Build the plugin binary and Docker image |
| `plugin-load` | Build and load the image into the kind cluster |
| `plugin-deploy` | Build, load, patch Argo Rollouts Deployment, and restart it |
| `argo-rollouts-install` | Install Argo Rollouts and apply the plugin ConfigMap |
| `argo-rollouts-uninstall` | Remove Argo Rollouts from the cluster |
| `argo-rollouts-example-apply` | Deploy the demo app (requires `ROLLOUT_DEMO_HOST=`) |
| `argo-rollouts-example-update` | Trigger a canary (patch the demo app to v2) |
| `argo-rollouts-example-promote` | Promote past a manual pause gate |
| `argo-rollouts-example-abort` | Abort the rollout and roll back to stable |
| `argo-rollouts-example-status` | Show rollout status, pods, and endpoints |
| `argo-rollouts-example-reset` | Tear down and redeploy fresh (requires `ROLLOUT_DEMO_HOST=`) — run between demos |
| `argo-rollouts-example-delete` | Tear down the demo app and RBAC |

---

## Known limitations

- **Traffic mirroring** (`setMirrorRoute`) is not supported. ngrok does not have a request mirroring primitive.
- **Header-based routing** (`setHeaderRoute`) is not yet implemented. It is planned for Phase 2 — the Traffic Policy CEL environment supports header matching and would be prepended before the `rand.double()` weight rule.
- **Weight granularity in Phase 1** is `100 / totalPoolSize` percent. A pool of 10 gives 10% steps; use a larger pool for finer control at the cost of more active tunnel sessions.
- **The operator changes** that enable Phase 2 (the `rollout-managed` annotation support in `applyAgentEndpoints` and `applyCloudEndpoints`) are not yet part of a published operator release. Local development requires building the operator from source.
- **Phase 2a — path-based routing**: in collapsed mode, the plugin replaces the operator's full inline traffic policy with its own canary + stable rules. Ingresses with multiple path-specific backends pointing to different services are not supported; use CloudEndpoint mode (`endpoints-verbose`) instead.
- **Phase 2b — policy cleanup**: after rollout completion or abort, the plugin restores an all-stable routing policy. The operator will overwrite this with its own generated policy on the next reconcile cycle, so there is a brief window where the policy is in plugin-written format rather than operator-generated format. Traffic is not disrupted during this window.
