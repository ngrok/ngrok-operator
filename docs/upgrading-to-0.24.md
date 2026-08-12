# Upgrade from Helm chart 0.23 to 0.24

This guide covers upgrading an existing ngrok Kubernetes Operator installation
from Helm chart `0.23.x` to `0.24.x`.

The project publishes the Helm chart, operator image, and CRDs with independent
version numbers:

| Component | Before | After |
| --- | --- | --- |
| `ngrok/ngrok-operator` Helm chart | `0.23.x` | `0.24.x` |
| Operator image | `0.21.x` | `0.22.x` |
| `ngrok/ngrok-crds` subchart | `0.3.x` | `0.4.x` |

In the rest of this guide, "0.24" means the `ngrok/ngrok-operator` Helm chart
release.

The CRD API versions remain unchanged in this release. Do not change manifests
from the existing `*.k8s.ngrok.com/v1alpha1` API versions to `ngrok.com/v1`;
that API-version migration is a separate future change.

## Before upgrading

### Back up the release configuration

Set these to match your installation:

```bash
RELEASE=ngrok-operator
NAMESPACE=ngrok-operator

helm get values "$RELEASE" --namespace "$NAMESPACE" --all \
  > ngrok-operator-0.23-values.yaml
helm get manifest "$RELEASE" --namespace "$NAMESPACE" \
  > ngrok-operator-0.23-manifest.yaml
```

Keep these backups until the upgrade and its rollback window have closed. Do
not use the output of `helm get values --all` as a new long-term values file;
it contains defaults from the old chart.

### Move cross-namespace AgentEndpoint certificate Secrets

`AgentEndpoint.spec.clientCertificateRefs` may no longer refer to a Secret in
another namespace. References are now always resolved in the AgentEndpoint's
namespace.

List cross-namespace references:

```bash
kubectl get agentendpoints.ngrok.k8s.ngrok.com -A -o json |
  jq -r '
    .items[] as $item
    | $item.spec.clientCertificateRefs[]?
    | select(.namespace != null and .namespace != $item.metadata.namespace)
    | "\($item.metadata.namespace)/\($item.metadata.name): \(.namespace)/\(.name)"
  '
```

For each result:

1. Copy or issue the TLS Secret in the AgentEndpoint's namespace.
2. Remove `namespace` from the certificate reference in your manifest.
3. Apply the updated manifest and confirm the AgentEndpoint remains ready.

After the 0.24 CRD is installed, Kubernetes prunes the removed `namespace`
field and the operator looks for the Secret beside the AgentEndpoint.

### Replace `BoundEndpoint.spec.endpointURI`

The deprecated `spec.endpointURI` field has been removed. Manually created
BoundEndpoints must use `spec.endpointURL`. Operator-created BoundEndpoints
have used `endpointURL` since chart 0.23.

List objects still using the old field:

```bash
kubectl get boundendpoints.bindings.k8s.ngrok.com -A -o json |
  jq -r '
    .items[]
    | select(.spec.endpointURI != null)
    | "\(.metadata.namespace)/\(.metadata.name): \(.spec.endpointURI)"
  '
```

Replace `endpointURI` with `endpointURL`, using the same value, before
upgrading.

### Remove `Domain.spec.region`

Region selection for reserved domains has been removed. List affected Domains:

```bash
kubectl get domains.ingress.k8s.ngrok.com -A -o json |
  jq -r '
    .items[]
    | select(.spec.region != null)
    | "\(.metadata.namespace)/\(.metadata.name): \(.spec.region)"
  '
```

Remove `spec.region` from these manifests before upgrading. Leaving it in place
is not destructive — the 0.24 CRD drops the field and Kubernetes silently prunes
it, and domains already reserved keep the region they were created with — but
removing it keeps your manifests aligned with the schema.

Do not rename `Domain.spec.resolves_to` yet if you may roll back to 0.23. That
field remains supported by 0.24 and is migrated after the rollback window.

### Validate IPPolicy rules

IPPolicy rules now require a CIDR with a prefix length, such as `10.0.0.0/8`
rather than `10.0.0.1`. Rule actions must be `allow` or `deny`, and rule
descriptions are limited to 255 characters.

Review the current rules:

```bash
kubectl get ippolicies.ingress.k8s.ngrok.com -A -o json |
  jq -r '
    .items[] as $item
    | $item.spec.rules[]?
    | "\($item.metadata.namespace)/\($item.metadata.name): cidr=\(.cidr) action=\(.action) descriptionLength=\((.description // "") | length)"
  '
```

Correct bare IP addresses, invalid actions, and over-length descriptions.
Installing the new CRD does not rewrite existing objects, but a future update
to an object that violates the new schema will be rejected.

### Check tooling that reads CRD status

This release changes status fields and conditions on `KubernetesOperator`,
`Domain`, `IPPolicy`, and `NgrokTrafficPolicy`. Manifests do not need changes
because status is written by the operator.

If dashboards, scripts, admission policies, or GitOps health checks read exact
status paths or condition names, review them before upgrading. The most
significant changes are:

- `KubernetesOperator` registration and drain phase fields are replaced by
  `Ready`, `Registered`, and `Draining` conditions plus structured
  `status.drain` counters.
- `Domain.status.region`, the Domain `Progressing` condition, and
  `IPPolicy.status.rules` are removed. On clusters upgraded in place, a stale
  `Progressing` entry may linger in `status.conditions` because 0.24 never
  updates it again; treat it as absent. Freshly created 0.24 objects never have
  it.
- The IPPolicy `RulesConfigured` condition is renamed to
  `IPPolicyRulesConfigured`.
- `NgrokTrafficPolicy.status.policy` is removed; use `spec.policy` for the
  policy and the new `Ready` and `Valid` conditions for observed state.

### Review Helm behavior changes

The cleanup hook no longer receives default CPU and memory requests or limits.
If scheduling or policy in your cluster depends on explicit resources, add
them to your values:

```yaml
cleanupHook:
  resources:
    requests:
      cpu: 50m
      memory: 64Mi
    limits:
      cpu: 100m
      memory: 128Mi
```

The chart also tightens the operator's RBAC. No change is required when the
chart manages RBAC. If you supply roles separately, compare them with the 0.24
chart before upgrading.

The default metadata attached to generated ngrok API resources changes from
`owned-by=kubernetes-ingress-controller` to `owned-by=ngrok-operator`. Update
ngrok API filters or automation that match the old default. Explicit metadata
values are not changed.

## Upgrade

If you install the CRD chart separately (operator chart with `installCRDs:
false`), upgrade `ngrok/ngrok-crds` to `0.4.x` **first**, then upgrade the
operator chart. The window where the new CRDs run against the still-old operator
is benign, with one visible symptom: `KubernetesOperator` shows a blank `Ready`
column, because the old status fields are pruned before the old operator writes
the new conditions. Anything alerting on that column goes blind until the
operator upgrade completes. If the chart manages CRDs (the default), skip this
paragraph and run the upgrade below.

Use the values file you normally manage for this release:

```bash
TARGET_VERSION=0.24.0
# When testing a release candidate, use the latest published one. Do not use
# 0.24.0-rc.1: its operator image (0.22.0-rc.1) ships a connection leak in
# forwarding and crashes on rollback while decoding KubernetesOperator status.
# Both are fixed in a later candidate.

helm repo update ngrok

helm upgrade "$RELEASE" ngrok/ngrok-operator \
  --namespace "$NAMESPACE" \
  --version "$TARGET_VERSION" \
  --values path/to/your-values.yaml \
  --wait
```

## Verify the upgrade

Confirm that the workloads rolled out:

```bash
kubectl get deployments,pods --namespace "$NAMESPACE"
kubectl get kubernetesoperators.ngrok.k8s.ngrok.com -A
```

The KubernetesOperator's old `Status` column is now `Ready`. Inspect its
conditions and recent events if it is not ready:

```bash
kubectl get kubernetesoperators.ngrok.k8s.ngrok.com -A -o yaml
kubectl get events -A --sort-by=.lastTimestamp
```

Verify the resource types you use:

```bash
kubectl get agentendpoints.ngrok.k8s.ngrok.com -A
kubectl get cloudendpoints.ngrok.k8s.ngrok.com -A
kubectl get domains.ingress.k8s.ngrok.com -A
kubectl get ippolicies.ingress.k8s.ngrok.com -A
```

At this point, you may keep your manifests in their 0.23-compatible forms
while you evaluate the release. The 0.24 operator accepts them through
compatibility shims.

The migrations in this guide are in-place. The annotation rename, the
`resolves_to` → `resolvesTo` rename, the CloudEndpoint traffic-policy field
move, and the metadata string → map change are all read-side or normalization
changes: they do not delete and recreate the backing ngrok endpoints, do not
change their IDs, and do not interrupt the data path. Reconciled objects keep
their existing ngrok resources.

## Rolling back

If you need to return to chart `0.23.x` within the rollback window, use the
backups captured above:

```bash
helm upgrade "$RELEASE" ngrok/ngrok-operator \
  --namespace "$NAMESPACE" \
  --version 0.23.0 \
  --values ngrok-operator-0.23-values.yaml \
  --wait
```

If you installed the CRD chart separately, the `0.4.x` CRDs are a superset of
`0.3.x` for the fields this guide covers and can be left in place during a
temporary rollback; the removed-field validation only bites on a future write to
an offending object. Reinstalling the `0.3.x` CRDs is only needed if you are
abandoning 0.24 entirely.

Sharp edges:

- Do not perform the "After the rollback window" migrations below until rollback
  is ruled out. Switching `spec.metadata` to the map form, renaming
  `resolves_to`, or moving CloudEndpoint traffic-policy fields produces objects
  the `0.3.x` CRD schema rejects. Leaving the `0.4.x` CRDs in place (above) keeps
  those objects valid, but if you also roll the CRDs back to `0.3.x` those
  manifests fail to apply.
- The `appProtocol` value cannot be dual-keyed. If you have already switched a
  Service port from `k8s.ngrok.com/http2` to `ngrok.com/http2`, rolling back
  below 0.24 silently drops HTTP/2 for that upstream until you restore the
  legacy value.
- On the `0.24.0-rc.1` operator image (`0.22.0-rc.1`) only, rolling back crashes
  while decoding `KubernetesOperator.status.enabledFeatures`. This is fixed in a
  later candidate; test with the latest published RC and this does not apply.

## After the rollback window

Complete the migrations in this section after the 0.24 installation is healthy
and rollback to chart 0.23 is no longer required. Establishing that once as a
prerequisite lets you migrate directly to the new forms instead of temporarily
maintaining both old and new values.

### Rename user-set annotations

Replace these annotation keys in your manifests:

| Legacy | New | Applies to |
| --- | --- | --- |
| `k8s.ngrok.com/url` | `ngrok.com/url` | LoadBalancer Service |
| `k8s.ngrok.com/mapping-strategy` | `ngrok.com/mapping-strategy` | Service, Ingress, Gateway |
| `k8s.ngrok.com/traffic-policy` | `ngrok.com/traffic-policy` | Service, Ingress, Gateway |
| `k8s.ngrok.com/pooling-enabled` | `ngrok.com/pooling-enabled` | Service, Ingress, Gateway |
| `k8s.ngrok.com/bindings` | `ngrok.com/bindings` | Service, Ingress, Gateway |
| `k8s.ngrok.com/metadata` | `ngrok.com/metadata` | Ingress, Gateway |
| `k8s.ngrok.com/description` | `ngrok.com/description` | Ingress, Gateway |
| `k8s.ngrok.com/app-protocols` | `ngrok.com/app-protocols` | Service backing an Ingress or Gateway route |
| `k8s.ngrok.com/terminate-tls.<option>` | `ngrok.com/terminate-tls.<option>` | Gateway listener TLS options |

The last two rows are not object annotations: `terminate-tls.<option>` keys live
under a Gateway listener's `tls.options`, and `app-protocols` is read from the
backing Service. They follow the same prefix rename and are listed together here
for convenience; each has its own audit below.

Also replace the Service port `appProtocol` value `k8s.ngrok.com/http2` with
`ngrok.com/http2`.

Bindings pod-identity annotations are forwarded verbatim. If an ngrok traffic
policy expression matches `k8s.ngrok.com/*` pod-annotation keys, update the pod
annotations and policy expression together.

#### How the two prefixes interact

The 0.24 operator reads both prefixes. When both are present on the same object
for the same key, the `ngrok.com/` key wins. Precedence is decided by key
**presence**, not value: a canonical `ngrok.com/` key outranks the legacy one
even when its value is empty or invalid. Most keys then reject the empty value as
invalid content, but the practical footgun is a template that renders, say,
`ngrok.com/traffic-policy: ""` when a variable is unset — it silently outranks a
working `k8s.ngrok.com/traffic-policy` and detaches the policy, with no error. On
a gated endpoint that removes the gate. Render canonical keys only when they have
a real value.

This presence-wins rule is what makes a staggered migration safe: you can add the
`ngrok.com/` key alongside the legacy one, confirm the endpoint is healthy, and
drop the legacy key once rollback below 0.24 is ruled out. `appProtocol` is the
exception — it is a single scalar and cannot carry both values at once, so switch
it only after rollback below 0.24 is ruled out (see the rollback section).

Helm's three-way merge removes the old key cleanly on upgrade, so chart-managed
objects rarely end up dual-keyed. Kustomize, Argo CD without prune, and
`kubectl patch` users routinely land in the both-keys state — harmless given the
precedence rule, but worth knowing when auditing.

#### Find legacy keys

Find Kubernetes objects that still carry legacy-prefixed annotations:

```bash
for kind in ingress gateway service; do
  # Skip kinds whose CRD isn't installed (e.g. gateway without Gateway API).
  kubectl get "$kind" -A -o json 2>/dev/null |
    jq -r --arg kind "$kind" '
      .items[]
      | select((.metadata.annotations // {}) | keys | any(startswith("k8s.ngrok.com/")))
      | "\($kind) \(.metadata.namespace)/\(.metadata.name)"
    '
done
```

This match is prefix-based, so it also surfaces any `k8s.ngrok.com/*` keys that
are yours and unrelated to the operator (image tags, node labels, and the like)
and the `k8s.ngrok.com/v1alpha1` API group you should leave alone. Cross-check
hits against the rename table above; only the keys in that table need renaming.

Find Gateway listener TLS options using the old prefix (skip if Gateway API is
not installed):

```bash
kubectl get gateway -A -o json 2>/dev/null |
  jq -r '
    .items[]
    | select(
        [.spec.listeners[]?.tls.options // {} | keys[]]
        | any(startswith("k8s.ngrok.com/"))
      )
    | "gateway \(.metadata.namespace)/\(.metadata.name)"
  '
```

Find the legacy Service `appProtocol` value:

```bash
kubectl get service -A -o json |
  jq -r '
    .items[]
    | select([.spec.ports[]?.appProtocol] | any(. == "k8s.ngrok.com/http2"))
    | "service \(.metadata.namespace)/\(.metadata.name)"
  '
```

Find legacy-prefixed pod annotations used by bindings:

```bash
kubectl get pods -A -o json |
  jq -r '
    .items[]
    | select(
        (.metadata.annotations // {})
        | keys
        | any(startswith("k8s.ngrok.com/"))
      )
    | "\(.metadata.namespace)/\(.metadata.name)"
  '
```

Pods commonly carry `k8s.ngrok.com/*` annotations unrelated to bindings, so this
audit is noisy. Only annotations your ngrok traffic-policy expressions actually
match need renaming; the rest are yours to leave or clean up as you see fit.

The operator also emits `LegacyAnnotation` warning events for managed
Ingresses, Gateways, and LoadBalancer Services:

```bash
kubectl get events -A --field-selector reason=LegacyAnnotation
```

These events are reconcile-driven and expire, so their absence proves nothing in
either direction: a migrated object stops appearing, and so does an unmigrated
idle one that has not reconciled recently. Treat them as a hint, not a complete
audit. Backend Service `app-protocols`, Service `appProtocol`, and bindings pod
annotations never produce these events at all, so rely on the direct audits
above for a point-in-time inventory.

### Rename `Domain.spec.resolves_to`

Replace the legacy field:

```yaml
spec:
  resolves_to:
    - value: example
```

with:

```yaml
spec:
  resolvesTo:
    - value: example
```

Find remaining objects:

```bash
kubectl get domains.ingress.k8s.ngrok.com -A -o json |
  jq -r '
    .items[]
    | select(.spec.resolves_to != null)
    | "\(.metadata.namespace)/\(.metadata.name)"
  '
```

### Migrate CloudEndpoint traffic policies

Replace a top-level policy reference:

```yaml
spec:
  trafficPolicyName: my-policy
```

with:

```yaml
spec:
  trafficPolicy:
    targetRef:
      name: my-policy
```

Replace an inline policy:

```yaml
spec:
  trafficPolicy:
    policy:
      on_http_request:
        - actions:
            - type: deny
```

with:

```yaml
spec:
  trafficPolicy:
    inline:
      on_http_request:
        - actions:
            - type: deny
```

A `targetRef` resolves an `NgrokTrafficPolicy` in the same namespace as the
CloudEndpoint. Cross-namespace policy references are not supported.

Find legacy CloudEndpoint fields:

```bash
kubectl get cloudendpoints.ngrok.k8s.ngrok.com -A -o json |
  jq -r '
    .items[]
    | select(
        .spec.trafficPolicyName != null
        or .spec.trafficPolicy.policy != null
      )
    | "\(.metadata.namespace)/\(.metadata.name)"
  '
```

Operator-generated CloudEndpoints are migrated by the operator; update only
CloudEndpoint manifests you manage directly.

### Convert CRD metadata to a map

The canonical form of `spec.metadata` is now a map:

```yaml
spec:
  metadata:
    owned-by: ngrok-operator
    team: platform
```

Replace the legacy JSON-object string:

```yaml
spec:
  metadata: '{"owned-by":"ngrok-operator","team":"platform"}'
```

This applies to `Domain`, `IPPolicy` and its rules, `KubernetesOperator`,
`CloudEndpoint`, and `AgentEndpoint`. Use only string keys and string values.

Update only the objects whose `spec.metadata` **you** set. In 0.24 the operator
still writes the string form for the objects it owns (the default is a JSON
string, and operator-generated resources normalize to a string on the wire), so
the audits below will list every operator-owned Domain, CloudEndpoint,
AgentEndpoint, and the KubernetesOperator — none of which are user-actionable in
this release. Match the results against the manifests you author directly and
ignore the rest.

Find top-level metadata that is still a JSON string:

```bash
for resource in \
  domains.ingress.k8s.ngrok.com \
  ippolicies.ingress.k8s.ngrok.com \
  kubernetesoperators.ngrok.k8s.ngrok.com \
  cloudendpoints.ngrok.k8s.ngrok.com \
  agentendpoints.ngrok.k8s.ngrok.com; do
  kubectl get "$resource" -A -o json |
    jq -r --arg resource "$resource" '
      .items[]
      | select((.spec.metadata | type) == "string")
      | "\($resource) \(.metadata.namespace)/\(.metadata.name)"
    '
done
```

Find string-form metadata on individual IPPolicy rules:

```bash
kubectl get ippolicies.ingress.k8s.ngrok.com -A -o json |
  jq -r '
    .items[] as $item
    | $item.spec.rules[]?
    | select((.metadata | type) == "string")
    | "\($item.metadata.namespace)/\($item.metadata.name): \(.cidr)"
  '
```

The 0.24 CRDs accept both forms. A later cleanup release will accept only the
map form.

### Update selectors for operator-written bindings labels

Most operator-written labels and annotations are internal and migrate
automatically. The bindings labels are sometimes used by dashboards,
monitoring, or GitOps tooling. Update any external selectors to the new keys:

| Legacy | New |
| --- | --- |
| `bindings.k8s.ngrok.com/endpoint-binding-name` | `ngrok.com/endpoint-binding-name` |
| `bindings.k8s.ngrok.com/endpoint-binding-namespace` | `ngrok.com/endpoint-binding-namespace` |

The operator writes both forms in 0.24, so selectors can be changed without
interrupting matching. The accompanying
`bindings.k8s.ngrok.com/endpoint-url` annotation is operator-written and does
not require action.

Do not write or select on the operator's `controller-name`,
`controller-namespace`, or `computed-url` keys. They are internal ownership
metadata and are migrated automatically.

### Update self-authored IngressClasses

If you manage your own IngressClass manifests, replace:

```yaml
spec:
  controller: k8s.ngrok.com/ingress-controller
```

with:

```yaml
spec:
  controller: ngrok.com/ingress-controller
```

The 0.24 operator recognizes both values. If you use the chart-rendered
IngressClass, no action is required; the chart performs the transition in a
later release.

## Prepare for later cleanup releases

The 0.24 release intentionally retains compatibility code. Complete the
user-managed migrations above before their cleanup releases:

| Migration | 0.24 behavior | Future requirement |
| --- | --- | --- |
| User annotations and `appProtocol` | Reads old and new forms | Remove all `k8s.ngrok.com/*` user configuration before 1.0 |
| CloudEndpoint traffic-policy fields | Reads old and new fields | Use only `targetRef` and `inline` before the announced cleanup release |
| `Domain.spec.resolves_to` | Reads `resolves_to` and `resolvesTo` | Use only `resolvesTo` before the announced cleanup release |
| CRD `spec.metadata` | Reads JSON strings and maps | Use maps before the announced cleanup release |
| Bindings labels | Writes both prefixes | Update external selectors before the planned 1.0 write-side cleanup |
| IngressClass controller | Recognizes both values | Self-authored manifests must use `ngrok.com/ingress-controller` before legacy matching is removed in 0.26 |

The finalizer rename is handled entirely by the operator across the planned
0.24, 0.25, and 0.26 sequence. In 0.25 the operator begins writing the new
finalizer; in 0.26 it removes legacy-read support. Do not edit operator
finalizers manually. If external tooling matches the literal
`k8s.ngrok.com/finalizer`, update that tooling before upgrading beyond 0.24.

The chart-rendered IngressClass moves to
`ngrok.com/ingress-controller` in 0.25, and the operator removes legacy
matching in 0.26. The chart handles both steps unless you manage the
IngressClass yourself.

Do not skip 0.24 when later upgrading through the finalizer and IngressClass
cleanup sequence. It is the compatibility checkpoint that allows the later
changes to roll out safely.
