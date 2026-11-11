# Upgrade from Helm chart 0.24 to 0.25

<!--
Maintainers: every PR that changes user-visible behavior for 0.25 adds to this
guide in the same PR. Put each change in the section that matches when the user
must act:
  - "Required before upgrading": the upgrade breaks something unless the user
    acts first (usually a cleanup-release removal of a 0.24 compatibility shim).
  - "Review before upgrading": no manifest change, but tooling, RBAC, or
    workflows may notice.
  - "After the rollback window": new canonical forms that a 0.24 rollback does
    not understand.
Then add or update the matching row in "Prepare for later cleanup releases".
The maintainer-side staging for each shim lives in
docs/developer-guide/passivity-shims.md.
-->

This guide covers upgrading an existing ngrok Kubernetes Operator installation
from Helm chart `0.24.x` to `0.25.x`.

The project publishes the Helm chart, operator image, and CRDs with independent
version numbers:

| Component | Before | After |
| --- | --- | --- |
| `ngrok/ngrok-operator` Helm chart | `0.24.x` | `0.25.x` |
| Operator image | `0.22.x` | `0.23.x` |
| `ngrok/ngrok-crds` subchart | `0.4.x` | `0.5.x` |

In the rest of this guide, "0.25" means the `ngrok/ngrok-operator` Helm chart
release.

**Upgrade from 0.24 only.** Do not upgrade directly from 0.23 or earlier. 0.25
removes compatibility code that 0.24 used to migrate existing objects, so a
0.23 installation must first upgrade to 0.24, run healthily, and complete the
"After the rollback window" steps in the
[0.24 upgrade guide](./upgrading-to-0.24.md#after-the-rollback-window) that
this guide lists as required below.

The 0.25 chart replaces the operator's two ngrok credentials with a single
[access token](https://dashboard.ngrok.com/access-tokens).
This is a breaking change: `credentials.apiKey` and `credentials.authtoken` are
removed, and an upgrade that still passes them fails with an explicit error
rather than starting a release with no credentials.

One token can serve both components, or you can set a separate token for each.
See [one token per component](#optional-one-token-per-component).

## Before upgrading

### Back up the release configuration

Set these to match your installation:

```bash
RELEASE=ngrok-operator
NAMESPACE=ngrok-operator
# The chart's resource name prefix: $RELEASE if the release name contains
# "ngrok-operator", otherwise "$RELEASE-ngrok-operator".
FULLNAME=ngrok-operator

helm get values "$RELEASE" --namespace "$NAMESPACE" --all \
  > ngrok-operator-0.24-values.yaml
helm get manifest "$RELEASE" --namespace "$NAMESPACE" \
  > ngrok-operator-0.24-manifest.yaml
```

Both files contain your current credentials, so store them somewhere only you
can read. Keep them until the upgrade and its rollback window have closed. Do
not use the output of `helm get values --all` as a new long-term values file;
it contains defaults from the old chart.

### Create an access token

One token replaces both of today's credentials. It authenticates the agent
connection and the ngrok API, so it needs the permissions the operator uses for
both: starting tunnels, and reading and writing endpoints, domains, reserved TCP
addresses, IP policies, and the Kubernetes operator registration.

Create the token at
[dashboard.ngrok.com/access-tokens](https://dashboard.ngrok.com/access-tokens).
Give it the permissions for the operations above.

The token is shown once. Store it the way you store the credentials it replaces.

### Replace the credential values

Remove `credentials.apiKey` and `credentials.authtoken` from your values file
and set `ngrok.credentials.accessToken` instead:

```yaml
ngrok:
  credentials:
    accessToken: "<your-access-token>"
```

If you pass credentials on the command line, replace both `--set` flags with
one:

```bash
helm upgrade "$RELEASE" ngrok/ngrok-operator \
  --namespace "$NAMESPACE" \
  --set ngrok.credentials.accessToken="$NGROK_ACCESS_TOKEN"
```

Passing either removed value now fails the render:

```
Error: UPGRADE FAILED: execution error at (ngrok-operator/templates/api-manager/deployment.yaml:1:4):

These Helm values moved in 0.25 and are no longer read at their old location:

  credentials -> ngrok.credentials (apiKey and authtoken are replaced by accessToken)
```

The error names the api-manager deployment because it checks for values from
the 0.24 layout, which is where the removed values live.

This includes values carried forward implicitly. If you upgrade with
`--reuse-values`, the old keys come along from the previous release and the
upgrade fails until you remove them.

### Create a new Secret if you manage your own

If you manage the credentials Secret yourself and point at it with
`ngrok.credentials.secret.name`, create a new Secret under a new name rather than
editing the existing one. The chart now reads one key per component:

| Before | After |
| --- | --- |
| `API_KEY` | `API_MANAGER_ACCESS_TOKEN` |
| `AUTHTOKEN` | `AGENT_ACCESS_TOKEN` |

Both keys hold the same token unless you set one per component, below.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-ngrok-credentials-v2
  namespace: ngrok-operator
type: Opaque
data:
  AGENT_ACCESS_TOKEN: <base64-encoded-access-token>
  API_MANAGER_ACCESS_TOKEN: <base64-encoded-access-token>
```

Then point the upgrade at it:

```yaml
ngrok:
  credentials:
    secret:
      name: my-ngrok-credentials-v2
```

Leaving the old Secret untouched keeps the running 0.24 pods working until the
upgrade replaces them, and keeps `helm rollback` working afterwards: the
previous release still names the old Secret, which still holds the keys 0.24
reads.

A Secret that carries only `API_KEY` and `AUTHTOKEN` leaves both 0.25 pods
unable to start, so create the new Secret before the chart upgrade.

### Move your values to the new layout

The 0.25 chart reorganizes its values into three groups: the chart's own
settings stay at the top level, the operator's configuration moves under
`ngrok`, and pod settings move under `components`. There are no aliases for the
old keys: a values file that still sets one fails the render and lists each old
key with its new location.

| 0.24 value | 0.25 value |
|------------|------------|
| `description`, `region`, `rootCAs`, `serverAddr`, `apiURL`, `clusterDomain` | `ngrok.<same>` |
| `ngrokMetadata`, `metaData` | `ngrok.metadata` |
| `credentials.*` | `ngrok.credentials.*` |
| `log.*` | `ngrok.log.*` |
| `ingress.*` | `ngrok.features.ingress.*` |
| `watchNamespace`, `controllerName`, `ingressClass.*` | `ngrok.features.ingress.watchNamespace`, `ngrok.features.ingress.controllerName`, `ngrok.features.ingress.ingressClass.*` |
| `gateway.*` | `ngrok.features.gateway.*` |
| `bindings.enabled`, `bindings.endpointSelectors`, `bindings.serviceAnnotations`, `bindings.serviceLabels`, `bindings.ingressEndpoint` | `ngrok.features.bindings.<same>` |
| `drainPolicy` | `ngrok.features.cleanup.drainPolicy` |
| `defaultDomainReclaimPolicy` | `ngrok.features.domains.defaultReclaimPolicy` |
| `oneClickDemoMode` | `ngrok.features.oneClickDemoMode.enabled` |
| `cleanupHook.enabled`, `cleanupHook.timeout` | `ngrok.features.cleanup.enabled`, `ngrok.features.cleanup.timeout` |
| `cleanupHook.image`, `cleanupHook.resources` | `components.cleanupHook.<same>` |
| `podAnnotations`, `podLabels`, `nodeSelector`, `tolerations`, `affinity`, `podAffinityPreset`, `podAntiAffinityPreset`, `nodeAffinityPreset`, `topologySpreadConstraints`, `priorityClassName`, `extraEnv` | `components.common.<same>` (applies to every component) |
| `lifecycle`, `extraVolumes`, `extraVolumeMounts` | `components.common.<same>` (as in 0.24, these apply to every component) |
| `replicaCount`, `resources`, `terminationGracePeriodSeconds`, `podDisruptionBudget.*`, `serviceAccount.*` | `components.apiManager.<same>` |
| `agent.*` | `components.agent.*` |
| `bindings.forwarder.*` | `components.bindingsForwarder.*` |
| `nameOverride`, `fullnameOverride`, `commonLabels`, `commonAnnotations`, `image.*`, `installCRDs`, `crdAccessRoles.*` | unchanged |

Three behaviors change along with the keys:

- Top-level pod settings such as `podAnnotations` and `tolerations` now apply to
  the agent and bindings-forwarder as well as the api-manager. To keep a
  setting on one component only, set it on that component instead, for example
  `components.apiManager.tolerations`.
- Log settings can be overridden for one component under
  `components.<component>.log`, for example `components.agent.log.level: debug`.
- An empty operator setting means "use the operator's built-in default". The
  defaults are listed in the chart README.

## Required before upgrading

These changes remove compatibility that 0.24 provided. Each one should already
be done if you completed the 0.24 guide's post-rollback-window migrations.

### Rename `Domain.spec.resolves_to` to `resolvesTo`

The legacy `Domain.spec.resolves_to` field has been removed from the CRD. The
0.25 operator reads only `spec.resolvesTo`; a Domain that still sets only
`resolves_to` is treated as having no `resolvesTo` entries, and the field is
pruned the next time the object is written.

Find remaining objects:

```bash
kubectl get domains.ingress.k8s.ngrok.com -A -o json |
  jq -r '
    .items[]
    | select(.spec.resolves_to != null)
    | "\(.metadata.namespace)/\(.metadata.name)"
  '
```

Replace:

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

The 0.24 CRD already accepts `resolvesTo`, so this change is safe to apply
before upgrading and survives a rollback to 0.24.

### Move CloudEndpoint traffic policies to `targetRef` and `inline`

The deprecated `CloudEndpoint.spec.trafficPolicyName` and
`spec.trafficPolicy.policy` fields have been removed from the CRD. The 0.25
operator reads only `spec.trafficPolicy.targetRef` and
`spec.trafficPolicy.inline`. A CloudEndpoint that still sets only a legacy
field no longer has a traffic policy the operator can resolve and cannot
reconcile successfully. An existing endpoint can report `Ready=False` while
its last applied policy continues serving traffic.
Migrate these fields before upgrading; the retained remote policy does not
preserve reconciliation of the legacy configuration. The removed fields are
pruned the next time the object is written.
Applying a manifest that still sets a legacy field fails `kubectl apply`'s
strict field validation with an `unknown field` error.

When `spec.trafficPolicy` is present, it must set exactly one of `inline` or
`targetRef`. An empty `trafficPolicy: {}` is rejected, as it is on AgentEndpoint.

Find remaining objects:

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

Replace:

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

and replace `spec.trafficPolicy.policy` with `spec.trafficPolicy.inline`.

Operator-generated CloudEndpoints need no action. The 0.24 CRD already accepts
`targetRef` and `inline`, so this change is safe to apply before upgrading and
survives a rollback to 0.24.

### Move external selectors off legacy bindings labels

The operator no longer writes the legacy bindings labels on Services it creates
for BoundEndpoints, and removes them from existing Services on their next
reconcile:

| Removed | Use instead |
| --- | --- |
| `bindings.k8s.ngrok.com/endpoint-binding-name` | `ngrok.com/endpoint-binding-name` |
| `bindings.k8s.ngrok.com/endpoint-binding-namespace` | `ngrok.com/endpoint-binding-namespace` |

The operator-written `bindings.k8s.ngrok.com/endpoint-url` annotation is removed
the same way; use `ngrok.com/endpoint-url`.

0.24 writes both forms, so dashboards, monitoring, network policies, or GitOps
tooling that select on these labels can switch to the new keys before
upgrading without losing matches.

### Update tooling that matches other legacy operator-written keys

The 0.24 guide advised against depending on these internal keys. If any tooling
does anyway, update it before upgrading. The operator now writes only the new
form and removes the legacy form on the next reconcile:

| Removed | Written instead | Found on |
| --- | --- | --- |
| `k8s.ngrok.com/controller-name` label | `ngrok.com/controller-name` | AgentEndpoint, CloudEndpoint, Domain |
| `k8s.ngrok.com/controller-namespace` label | `ngrok.com/controller-namespace` | AgentEndpoint, CloudEndpoint, Domain |
| `k8s.ngrok.com/computed-url` annotation | `ngrok.com/computed-url` | LoadBalancer Service |
| `k8s.ngrok.com/finalizer` finalizer | `ngrok.com/finalizer` | Objects the operator manages |

The finalizer change is handled by the operator. Do not edit operator
finalizers manually: 0.25 still recognizes and removes the legacy finalizer on
objects that carry it, and swaps it for the new one the next time it
reconciles each object.

## Review before upgrading

### Upgrade the CRDs before the operator

0.25 adds new CRDs, `trafficpolicies.ngrok.com` (see
[Migrate `NgrokTrafficPolicy` to `TrafficPolicy`](#migrate-ngroktrafficpolicy-to-trafficpolicy)),
`kubernetesoperators.ngrok.com` (see
[`KubernetesOperator` moves to `ngrok.com/v1`](#kubernetesoperator-moves-to-ngrokcomv1)),
`domains.ngrok.com` (see
[`Domain` moves to `ngrok.com/v1`](#domain-moves-to-ngrokcomv1)),
`agentendpoints.ngrok.com` (see
[AgentEndpoints move to `ngrok.com/v1`](#agentendpoints-move-to-ngrokcomv1)),
and `cloudendpoints.ngrok.com` (see
[CloudEndpoints move to `ngrok.com/v1`](#cloudendpoints-move-to-ngrokcomv1)),
and the operator watches them on startup. If you install the CRD chart
separately, upgrade `ngrok/ngrok-crds` to `0.5.x` before upgrading the operator
chart, or the operator will not start. If that happens, the new pods crash-loop
with `no matches for kind ... in version ngrok.com/v1`, the old pods keep
serving, and `helm upgrade --wait` fails. Upgrade the CRD chart: the new pods
then start on their own, and the next `helm upgrade` clears the failed release.

### Custom RBAC

No change is required when the chart manages RBAC. If you supply roles
separately, add the equivalent rules before upgrading.

### `KubernetesOperator` moves to `ngrok.com/v1`

The operator's own `KubernetesOperator` resource moves from
`ngrok.k8s.ngrok.com/v1alpha1` to `ngrok.com/v1`. No action is needed: the
operator creates and reconciles the `ngrok.com/v1` object itself. It attaches to
the same ngrok `KubernetesOperator` the old object used, so its ID, its
bindings certificate, and every endpoint it manages carry over unchanged. No
drain runs.

The old `ngrok.k8s.ngrok.com/v1alpha1` object stays in the cluster, unchanged
apart from its status:

- it keeps its finalizer and its ID, so rolling back to 0.24 picks it up as it
  was;
- its `Ready` condition is `False` with reason `SupersededByV1`, and the
  condition message names the `ngrok.com/v1` object;
- the operator no longer reconciles it or calls ngrok for it.

Deleting it is safe: the operator releases its finalizer without draining or
deleting anything in ngrok. While it exists, the operator treats a deletion of
the `ngrok.com/v1` object as a rollback rather than an uninstall, and does not
drain. `helm uninstall` removes the old object first, so it still drains.
1.0 removes the old object's finalizer, and 1.1 removes the old CRD.

The `kubernetesoperators.ngrok.com` and `trafficpolicies.ngrok.com` CRDs carry
`helm.sh/resource-policy: keep` in this release, so a rollback cannot delete
them or the objects stored under them. The annotation also stops
`helm uninstall` from deleting them, whether they come from the operator chart
or from a separate `ngrok/ngrok-crds` release. After a drain they hold no
operator-managed objects, and installing the chart again under the same
release name and namespace adopts them.

Deleting a CRD deletes every object stored under it. `trafficpolicies.ngrok.com`
holds your own `TrafficPolicy` objects, and `kubernetesoperators.ngrok.com`
holds the `KubernetesOperator` of every operator release in the cluster. Before
removing them, back up any `TrafficPolicy` objects you want to keep and make
sure no other ngrok-operator release still uses them:

```bash
kubectl get trafficpolicies.ngrok.com,kubernetesoperators.ngrok.com -A
```

To remove them:

```bash
kubectl delete crd kubernetesoperators.ngrok.com trafficpolicies.ngrok.com
```

1.0 removes the annotation, and Helm deletes the CRDs again on uninstall.

What changes for tooling:

- Both kinds use the plural `kubernetesoperators`, so name the group when you
  query or delete them: `kubectl get kubernetesoperators.ngrok.com -A`. A bare
  `kubectl get kubernetesoperator` may return either one.
- `spec.metadata` on the `ngrok.com/v1` kind is a map of strings. The JSON
  string form is not accepted.
- `status.enabledFeatures` is always an array.

### AgentEndpoints move to `ngrok.com/v1`

AgentEndpoint moves from `ngrok.k8s.ngrok.com/v1alpha1` to `ngrok.com/v1`.
Both kinds are served in 0.25. Endpoints keep serving through the upgrade:

- The AgentEndpoints the operator creates for Ingresses, Gateway API routes,
  and LoadBalancer Services are now `ngrok.com/v1` objects. Each takes the name
  of the legacy AgentEndpoint it replaces and takes over its agent endpoint.
  The legacy object is left in place, marked `Ready` with reason
  `SupersededByV1`, so a rollback to 0.24 finds it intact. The operator
  deletes it with its replacement once the Ingress, route, or Service no
  longer needs it.
- An `ngrok.com/v1` AgentEndpoint's metadata holds strings only, so the
  `ngrok.com/metadata` annotation on an Ingress or route must be a JSON object
  of strings. If any value isn't a string, such as `{"replicas": 3}`, the
  generated AgentEndpoint gets none of that metadata.
- AgentEndpoints you created keep working. Migrate them after the rollback
  window; see
  [Migrate `AgentEndpoint` to `ngrok.com/v1`](#migrate-agentendpoint-to-ngrokcomv1).

Both CRDs use the plural `agentendpoints`. Once both exist,
`kubectl get agentendpoints` lists the `ngrok.com` objects only. Use the full
names, `agentendpoints.ngrok.com` and `agentendpoints.ngrok.k8s.ngrok.com`, in
scripts and tooling.

Like the other new CRDs, `agentendpoints.ngrok.com` carries
`helm.sh/resource-policy: keep`, so a rollback can't delete it and every object
stored under it. As a result, `helm uninstall` leaves it behind; delete it by
hand after uninstalling.

### CloudEndpoints move to `ngrok.com/v1`

CloudEndpoint moves from `ngrok.k8s.ngrok.com/v1alpha1` to `ngrok.com/v1`.
Both kinds are served in 0.25. Endpoints keep serving through the upgrade:

- The CloudEndpoints the operator creates for Ingresses, Gateway API routes,
  and LoadBalancer Services (in `endpoints-verbose` mode) are now
  `ngrok.com/v1` objects. Each takes the name of the legacy CloudEndpoint it
  replaces and takes over its ngrok endpoint, ID included, so nothing is
  recreated. The legacy object is left in place, marked `Ready` with reason
  `SupersededByV1`, so a rollback to 0.24 finds it intact. The operator
  deletes it with its replacement once the Ingress, route, or Service no
  longer needs it. A Service names its first `ngrok.com/v1` CloudEndpoint
  after one of its legacy ones, preferring one that already has an ID.
- An `ngrok.com/v1` CloudEndpoint's metadata holds strings only, so the
  `ngrok.com/metadata` annotation on an Ingress, route, or Service must be a
  JSON object of strings. If any value isn't a string, such as
  `{"replicas": 3}`, the generated CloudEndpoint gets none of that metadata.
- CloudEndpoints you created keep working. Migrate them after the rollback
  window; see
  [Migrate `CloudEndpoint` to `ngrok.com/v1`](#migrate-cloudendpoint-to-ngrokcomv1).

Both CRDs use the plural `cloudendpoints` and the short name `clep`. Once both
exist, `kubectl get cloudendpoints` lists the `ngrok.com` objects only. Use the
full names, `cloudendpoints.ngrok.com` and
`cloudendpoints.ngrok.k8s.ngrok.com`, in scripts and tooling.

`cloudendpoints.ngrok.com` carries `helm.sh/resource-policy: keep`, so a
rollback can't delete it and every object stored under it. As a result,
`helm uninstall` leaves it behind; delete it by hand after uninstalling (see
[Uninstalling](./uninstall.md)).

### Endpoint events

The operator no longer emits `DeprecatedField` warning events on
CloudEndpoints; the fields they warned about are removed (see
[Move CloudEndpoint traffic policies](#move-cloudendpoint-traffic-policies-to-targetref-and-inline)).

### `Domain.spec.domain` is now immutable

Changing `spec.domain` on an existing Domain is rejected at admission:

```text
spec.domain is immutable. Reserved domains cannot be renamed via the ngrok API; create a new Domain resource instead
```

Earlier releases accepted the change but silently kept the old reservation. To
reserve a different domain, create a new Domain resource. The old Domain can
stay in place, or be deleted separately; its `reclaimPolicy` controls what
happens to its reservation when it is deleted.

A Domain whose `spec.domain` was already changed before upgrading keeps
reporting against its original reservation. Check for drifted Domains:

```bash
kubectl get domains.ingress.k8s.ngrok.com -A -o json |
  jq -r '
    .items[]
    | select(.status.domain != null and .spec.domain != .status.domain)
    | "\(.metadata.namespace)/\(.metadata.name): spec=\(.spec.domain) status=\(.status.domain)"
  '
```

### Operator flags, if you run the binaries yourself

The chart now passes configuration as `NGROK_OPERATOR__*` environment variables instead of flags, so chart users need no change. If you run `ngrok-operator` with your own arguments, every setting's flag is renamed after its values path (see [configuration](../specs/configuration.md#names)):

| 0.24 flag | 0.25 flag |
|-----------|-----------|
| `--enable-feature-ingress`, `--enable-feature-gateway`, `--enable-feature-bindings` | `--features-ingress-enabled`, `--features-gateway-enabled`, `--features-bindings-enabled` |
| `--ingress-controller-name`, `--ingress-watch-namespace` (agent: `--watch-namespace`) | `--features-ingress-controller-name`, `--features-ingress-watch-namespace` |
| `--disable-reference-grants` | `--features-gateway-disable-reference-grants` |
| `--bindings-endpoint-selectors`, `--bindings-service-annotations`, `--bindings-service-labels`, `--bindings-ingress-endpoint` | `--features-bindings-<same>` |
| `--default-domain-reclaim-policy` | `--features-domains-default-reclaim-policy` |
| `--drain-policy` | `--features-cleanup-drain-policy` |
| `--one-click-demo-mode` | `--features-one-click-demo-mode-enabled` |
| `--ngrokMetadata` | `--metadata` |
| `--zap-log-level`, `--zap-encoder`, `--zap-stacktrace-level` | `--log-level`, `--log-format`, `--log-stacktrace-level` |

- Map flags take a YAML or JSON map (`'{env: dev}'`) instead of `key=value,key=value`.
- `--features-bindings-endpoint-selectors` takes a YAML or JSON list (`'["true"]'`) instead of comma-separated values.
- The agent and bindings-forwarder no longer accept `--description`, which they ignored.
- The api-manager's `--election-id` defaults to empty, which turns leader election off. The chart sets it, so chart installs are unchanged; if you run several api-manager replicas yourself, pass `--election-id` to keep one leader.
- The other `--zap-*` flags (`--zap-devel`, `--zap-time-encoding`) are gone.

## Upgrade

If you install the CRD chart separately, upgrade `ngrok/ngrok-crds` to `0.5.x`
first, then upgrade the operator chart, keeping its CRD installation disabled as
in your existing configuration.

Use the values file you normally manage for this release:

```bash
TARGET_VERSION=0.25.0

helm repo update ngrok

helm upgrade "$RELEASE" ngrok/ngrok-operator \
  --namespace "$NAMESPACE" \
  --version "$TARGET_VERSION" \
  --values path/to/your-values.yaml \
  --wait
```

## Verify the upgrade

Confirm that the workloads rolled out and the operator is ready:

```bash
kubectl get deployments,pods --namespace "$NAMESPACE"
kubectl get kubernetesoperators.ngrok.com -A
```

At startup the api-manager verifies the token by listing endpoints. A token it
cannot use stops startup with an error:

```
Unable to verify access token: HTTP 403: ... [ERR_NGROK_203]
```

The agent-manager has no equivalent check, so confirm that it established its
tunnel session:

```bash
kubectl logs --namespace "$NAMESPACE" deploy/"$FULLNAME"-agent | grep "agent connected"
```

```
drivers.agent  ngrok agent connected
```

Confirm both TrafficPolicy CRDs are installed:

```bash
kubectl get crd trafficpolicies.ngrok.com ngroktrafficpolicies.ngrok.k8s.ngrok.com
```

Verify the resource types you use:

```bash
kubectl get agentendpoints.ngrok.k8s.ngrok.com -A
kubectl get cloudendpoints.ngrok.k8s.ngrok.com -A
kubectl get domains.ingress.k8s.ngrok.com -A
kubectl get ippolicies.ingress.k8s.ngrok.com -A
kubectl get ngroktrafficpolicies.ngrok.k8s.ngrok.com -A
```

## Optional: one token per component

The operator runs its credentials in two pods split by capability. The
agent-manager establishes tunnel sessions and makes no API calls; the
api-manager reconciles resources against the ngrok API and never starts
tunnels.

The chart can take a separate token per component, so each token can be scoped
to only the permissions its component needs.

To set them, use these instead of `ngrok.credentials.accessToken`:

```yaml
ngrok:
  credentials:
    agent:
      accessToken: "<token that can start tunnels>"
    apiManager:
      accessToken: "<token with ngrok API permissions>"
```

Either may be set on its own alongside `ngrok.credentials.accessToken`, which the
other falls back to. Setting one without a fallback for the other fails the
render rather than leaving a pod unable to start.

Both deployments annotate a checksum over the whole rendered Secret, so
changing either token restarts both pods.

## The old Secret keys

If the chart manages the Secret, Helm removes `API_KEY` and `AUTHTOKEN` as part
of the upgrade — they are gone from the live Secret once it completes, with no
cleanup step needed. Confirm:

```bash
kubectl get secret "$FULLNAME-credentials" --namespace "$NAMESPACE" \
  -o go-template='{{range $k, $v := .data}}{{$k}}{{"\n"}}{{end}}'
```

```
AGENT_ACCESS_TOKEN
API_MANAGER_ACCESS_TOKEN
```

If you manage the Secret yourself, delete the old Secret once you are committed
to 0.25 — see rolling back, below.

Revoking the old API key and authtoken in the dashboard is a separate step, and
one to take only after you are committed to 0.25 — see rolling back, below.

## Rotating the token

Set the new value and upgrade. Both deployments that hold the token carry a checksum of the credentials
Secret, so each restarts and picks up the new value.

```bash
helm upgrade "$RELEASE" ngrok/ngrok-operator \
  --namespace "$NAMESPACE" \
  --reuse-values \
  --set ngrok.credentials.accessToken="$NEW_NGROK_ACCESS_TOKEN"
```

Revoke the old token in the dashboard once the rollout completes.

This works because the checksum is taken over the Secret the chart renders. If
you manage the Secret yourself with `ngrok.credentials.secret.name`, the chart renders
nothing, the checksum never changes, and neither pod restarts — the token is
read from the environment once at startup, so both keep using the old value.
Restart them yourself after rotating, before revoking the old token:

```bash
kubectl rollout restart --namespace "$NAMESPACE" \
  deploy/"$FULLNAME"-manager deploy/"$FULLNAME"-agent
```

## Rolling back

To return to the `0.24.x` release you upgraded from within the rollback
window, reinstall it with the backups captured above:

```bash
helm upgrade "$RELEASE" ngrok/ngrok-operator \
  --namespace "$NAMESPACE" --version 0.24.2 \
  --values ngrok-operator-0.24-values.yaml --wait
```

Rolling back to 0.24 is safe for objects the 0.25 operator wrote: 0.24 reads
both label, annotation, and finalizer prefixes and re-adds the legacy forms on
its next reconcile.

Rolling back keeps the operator registered, without a drain or any change in
ngrok, and needs no extra steps: the 0.24 operator picks up the
`ngrok.k8s.ngrok.com/v1alpha1` `KubernetesOperator` that 0.25 left in place,
with its finalizer and ID, and attaches to the same ngrok `KubernetesOperator`.
The `ngrok.com` CRDs and their objects stay in the cluster because of the keep
annotation. 0.24 ignores them, and upgrading to 0.25 again picks them up.

`ngrok.com/v1` `TrafficPolicy` objects survive a rollback, but 0.24 does not
read them, so endpoints that reference only a `TrafficPolicy` fail to
reconcile until you upgrade again. Do not start the "After the rollback window" migration below until
rollback is ruled out.

AgentEndpoints roll back too: 0.24 serves the legacy AgentEndpoints the
operator left in place, and creates legacy ones for Ingresses, routes, and
Services that have none. A `ngrok.com/v1` AgentEndpoint you created after the
upgrade stops serving until you upgrade again. The `ngrok.com/v1` objects stay
in the cluster during the rollback. When you upgrade again, the agent serves
them as they were until the new operator takes over the leader lease and
brings the generated ones up to date, which can take about 30 seconds. Until
then an Ingress, route, or Service you changed or deleted during the rollback
serves its old configuration, and a Service created after the first upgrade
can show two endpoints on its URL in the ngrok dashboard.

CloudEndpoints roll back too: 0.24 serves the legacy CloudEndpoints, which keep
their ngrok IDs. A `ngrok.com/v1` CloudEndpoint you created after the upgrade
is ignored by 0.24: its endpoint keeps running but is no longer updated, and
deleting it hangs on its finalizer until you upgrade again. An Ingress, route,
or Service you created after the upgrade has only a `ngrok.com/v1`
CloudEndpoint, whose endpoint keeps serving unmanaged, so 0.24 generates a
legacy one with no ID:

- If the URL doesn't have pooling enabled (the default), ngrok rejects 0.24's
  create with `ERR_NGROK_18017`. The legacy object reports
  `CloudEndpointCreationFailed` and keeps retrying, and no second endpoint is
  created. After you upgrade again, there is nothing to clean up.
- If pooling is enabled, 0.24 creates a second endpoint on the same URL, which
  is a duplicate in the pool and on the bill. After you upgrade again, a
  Service's extra legacy CloudEndpoint is deleted for you. For an Ingress or
  route, the operator reports a `CloudEndpointIDConflict` (see
  [Migrate `CloudEndpoint`](#migrate-cloudendpoint-to-ngrokcomv1)); delete the
  legacy object to remove the extra endpoint.

If you install the CRD chart separately, roll back the operator first, then the
CRD chart.

### Uninstalling after a rollback

Avoid this if you can. The 0.24 uninstall hook deletes
`kubernetesoperator "$RELEASE"` by its bare name, which now resolves to the
`ngrok.com` kind. The 0.24 hook has no permissions there, so nothing drains.
Helm then deletes the old CRDs while their objects still carry finalizers, and
those objects stay in `Terminating`.

To uninstall cleanly, delete the old object yourself first. The 0.24 operator
drains as usual:

```bash
kubectl delete kubernetesoperators.ngrok.k8s.ngrok.com "$RELEASE" -n "$NAMESPACE"
helm uninstall "$RELEASE" -n "$NAMESPACE"
```

Then remove what the keep annotation left behind:

```bash
kubectl patch kubernetesoperators.ngrok.com "$RELEASE" -n "$NAMESPACE" \
  --type=merge -p '{"metadata":{"finalizers":null}}'
kubectl delete crd kubernetesoperators.ngrok.com trafficpolicies.ngrok.com
```

If you already uninstalled without deleting the old object first, set
`metadata.finalizers` to `null` on every object left in `Terminating`, then
run the two commands above. With `drainPolicy: Retain` (the default) your ngrok
resources were kept anyway. With `Delete`, remove them in the ngrok dashboard,
along with the Kubernetes Operator registration.

`Domain` objects survive a rollback. The `domains.ngrok.com` CRD carries
`helm.sh/resource-policy: keep`, so rolling back the CRD chart leaves it, and
the `ngrok.com/v1` Domains in it, in place. 0.24 does not know about them and
keeps serving every hostname from its `ingress.k8s.ngrok.com/v1alpha1` Domains,
which 0.25 never deletes; 0.24 creates one for any hostname first seen under
0.25 and adopts the existing reservation. Upgrading to 0.25 again picks the
`ngrok.com/v1` Domains back up. Nothing touches a reserved domain in ngrok
along the way.

While rolled back, an `ngrok.com/v1` Domain is not reconciled: deleting one
waits on its finalizer until you upgrade again. If you uninstall a
rolled-back release, `domains.ngrok.com` and its objects stay behind. Remove
the finalizers from those objects, then delete the CRD:

```bash
kubectl get domains.ngrok.com -A -o jsonpath='{range .items[*]}{.metadata.namespace}{" "}{.metadata.name}{"\n"}{end}' |
  while read -r ns name; do
    kubectl patch domains.ngrok.com "$name" -n "$ns" --type=merge -p '{"metadata":{"finalizers":null}}'
  done
kubectl delete crd domains.ngrok.com
```

The backed-up values include `credentials.apiKey` and `credentials.authtoken`,
so keep those credentials valid until the rollback window has closed. Do not
revoke the old API key and authtoken until you are committed to 0.25.

If you created a new Secret for 0.25, keep the old one until then as well. The
rolled-back release reads from it.

## After the rollback window

Complete the migrations in this section after the 0.25 installation is healthy
and rollback to chart 0.24 is no longer required.

### Migrate `NgrokTrafficPolicy` to `TrafficPolicy`

`ngrok.k8s.ngrok.com/v1alpha1` `NgrokTrafficPolicy` is deprecated. Its
replacement is `ngrok.com/v1` `TrafficPolicy`, the first resource to move to
the consolidated `ngrok.com/v1` API group. The spec is unchanged, so migrating
is a change of `apiVersion` and `kind`:

```yaml
apiVersion: ngrok.k8s.ngrok.com/v1alpha1
kind: NgrokTrafficPolicy
metadata:
  name: my-policy
spec:
  policy:
    on_http_request:
      - actions:
          - type: deny
```

becomes:

```yaml
apiVersion: ngrok.com/v1
kind: TrafficPolicy
metadata:
  name: my-policy
spec:
  policy:
    on_http_request:
      - actions:
          - type: deny
```

Both kinds are served in 0.25. References from AgentEndpoints, CloudEndpoints,
Ingresses, Services, and Gateway API routes resolve by name within the
namespace, preferring a `TrafficPolicy` and falling back to an
`NgrokTrafficPolicy` of the same name, so references keep working throughout
the migration without edits. `kubectl apply` of an `NgrokTrafficPolicy` prints
a deprecation warning. When an AgentEndpoint or CloudEndpoint reference
resolves to the deprecated kind, the operator emits a `DeprecatedAPIGroup`
warning event on that endpoint; references from Ingresses, Services, and
Gateway API routes are logged by the operator instead.

The operator does not copy objects between the two kinds. For each policy,
create the `TrafficPolicy`, confirm it is ready, then delete the
`NgrokTrafficPolicy`:

```bash
kubectl get ngroktrafficpolicies.ngrok.k8s.ngrok.com -A

kubectl get ngroktrafficpolicies.ngrok.k8s.ngrok.com my-policy -n my-namespace -o json |
  jq '
    .kind = "TrafficPolicy"
    | .apiVersion = "ngrok.com/v1"
    | del(.metadata.resourceVersion, .metadata.uid,
          .metadata.creationTimestamp, .metadata.generation,
          .metadata.managedFields, .status)
  ' |
  kubectl apply -f -

kubectl get trafficpolicies.ngrok.com my-policy -n my-namespace

kubectl delete ngroktrafficpolicies.ngrok.k8s.ngrok.com my-policy -n my-namespace
```

If you manage policies with GitOps, change `apiVersion` and `kind` in the
source manifest instead, and let your tool prune the old object.

Gateway API routes that reference a policy through an `ExtensionRef` filter
with `group: ngrok.k8s.ngrok.com` and `kind: NgrokTrafficPolicy` keep resolving
the migrated policy. Update them to `group: ngrok.com` and
`kind: TrafficPolicy` at the same time or later.

Find endpoints that still resolve the deprecated kind:

```bash
kubectl get events -A --field-selector reason=DeprecatedAPIGroup
```

Events expire and do not cover Ingress, Service, or Gateway API references, so
treat the `kubectl get ngroktrafficpolicies` listing above as the authoritative
check.

### `Domain` moves to `ngrok.com/v1`

`ingress.k8s.ngrok.com/v1alpha1` `Domain` is deprecated. Its replacement is
`ngrok.com/v1` `Domain`. The kind keeps its name, and the spec is unchanged
except that `spec.metadata` must be a map of strings.

**The operator migrates the Domains it created.** It creates an
`ngrok.com/v1` Domain for every hostname it serves: Ingress and Gateway hosts,
and the hostnames of AgentEndpoints and CloudEndpoints.

- With no legacy Domain for the hostname, the new Domain gets the usual
  defaults.
- When the legacy Domain for the hostname is one the operator created (it
  carries the operator's controller labels), the new Domain is a copy of its
  spec. Labels and annotations are not copied; the new object gets the
  operator's own labels.
- When the legacy Domain is one you created, the operator leaves it alone and
  keeps serving the hostname from it. You re-stamp it (below).

The `ngrok.com/v1` Domain takes over the legacy Domain's reservation ID, so the
reserved domain in ngrok, its ID and its CNAME target don't change. From then
on:

- The `ngrok.com/v1` Domain manages the reservation. Its spec is pushed to
  ngrok.
- The legacy Domain stays `Ready`, with reason `SupersededByV1`. Its other
  status fields stop updating. The operator emits a `DeprecatedAPIGroup`
  warning event when it takes over, and again if you edit the legacy Domain;
  edits have no effect.
- Deleting the legacy Domain leaves the reservation in place. Deleting the
  `ngrok.com/v1` Domain instead hands the reservation back to the legacy one.
  The reservation follows `reclaimPolicy` only when neither object remains.

The operator never deletes a legacy Domain itself, and legacy Domains keep
their finalizer in 0.25. 1.0 removes it; see
[Prepare for later cleanup releases](#prepare-for-later-cleanup-releases).

**You migrate the Domains you created yourself.** They keep working as before
until you do. `kubectl apply` of a legacy Domain prints a deprecation warning.
For each Domain, create the `ngrok.com/v1` object, confirm it is `Ready` and
has the legacy Domain's ID, then delete the legacy one:

```bash
kubectl get domains.ingress.k8s.ngrok.com -A

kubectl get domains.ingress.k8s.ngrok.com my-domain -n my-namespace -o json |
  jq '
    .apiVersion = "ngrok.com/v1"
    | del(.metadata.resourceVersion, .metadata.uid,
          .metadata.creationTimestamp, .metadata.generation,
          .metadata.managedFields, .metadata.finalizers, .status)
    | if .spec.metadata == "" then del(.spec.metadata)
      elif (.spec.metadata | type) == "string" then .spec.metadata |= fromjson
      else . end
  ' |
  kubectl apply -f -

kubectl get domains.ngrok.com my-domain -n my-namespace
kubectl get domains.ingress.k8s.ngrok.com my-domain -n my-namespace

kubectl delete domains.ingress.k8s.ngrok.com my-domain -n my-namespace
```

Both `get` commands should show the same ID before you delete the legacy
object.

If you delete a legacy Domain without creating an `ngrok.com/v1` one, and an
Ingress, Gateway or endpoint still uses its hostname, the operator creates a
default `ngrok.com/v1` Domain for it, which takes over the same reservation.
Re-stamp instead if you set a `description`, `resolvesTo` or `reclaimPolicy`
you want to keep.

`ngrok.com/v1` `Domain` accepts `spec.metadata` only as a map of string values;
the deprecated JSON-string form is rejected. The `jq` filter above converts
string-form metadata that holds a JSON object. If a Domain's metadata is free
text, or has non-string values, change it to a map of strings first: `jq`
fails on free text, and `kubectl apply` rejects non-string values. For the
Domains it created, the operator does the same conversion. It doesn't copy a Domain whose metadata is free text,
has non-string values, or is an empty object: it keeps serving it from the
legacy Domain and emits a `DomainMigrationBlocked` warning event on the
endpoint. Change its metadata to a non-empty map of strings to migrate it.

If you manage Domains with GitOps, change `apiVersion` (and any string-form
`metadata`) in the source manifest instead. Let your tool create the new
object and confirm it is `Ready` before it prunes the old one.

Both CRDs use the plural `domains`, and while both exist `kubectl get domains`
shows the legacy `ingress.k8s.ngrok.com` kind. Use the fully qualified names
`domains.ngrok.com` and `domains.ingress.k8s.ngrok.com`.

### Migrate `IPPolicy` to `ngrok.com/v1`

`ingress.k8s.ngrok.com/v1alpha1` `IPPolicy` is deprecated. Its replacement is
`ngrok.com/v1` `IPPolicy`. The kind keeps its name, so migrating is a change of
`apiVersion`:

```yaml
apiVersion: ingress.k8s.ngrok.com/v1alpha1
kind: IPPolicy
metadata:
  name: office
spec:
  rules:
    - cidr: 203.0.113.0/24
      action: allow
```

becomes:

```yaml
apiVersion: ngrok.com/v1
kind: IPPolicy
metadata:
  name: office
spec:
  rules:
    - cidr: 203.0.113.0/24
      action: allow
```

Both kinds are served in 0.25. **The IP policy keeps its ngrok ID.** When you
create an `ngrok.com/v1` `IPPolicy` with the same name and namespace as an
existing legacy one, the operator hands the existing ngrok IP policy to the new
object instead of creating another, so traffic policies that reference the
policy's `ipp_...` ID keep working. From then on:

- The `ngrok.com/v1` object manages the IP policy. Its spec is pushed to ngrok.
- The legacy object stays `Ready`, with reason `SupersededByV1`. Changes to it
  have no effect; the operator emits a `DeprecatedAPIGroup` warning event if
  you edit it.
- Deleting the legacy object leaves the IP policy in place. Deleting the
  `ngrok.com/v1` object instead hands the IP policy back to the legacy object.
  The ngrok IP policy is deleted only when neither object remains.

`kubectl apply` of a legacy `IPPolicy` prints a deprecation warning.

Both CRDs use the plural `ippolicies`. While both exist,
`kubectl get ippolicies` lists the **legacy** `ingress.k8s.ngrok.com` objects,
not the new ones. Use the fully qualified names below.

For each policy, create the `ngrok.com/v1` object, confirm it is ready and has
the same ID, then delete the legacy one:

```bash
kubectl get ippolicies.ingress.k8s.ngrok.com -A

kubectl get ippolicies.ingress.k8s.ngrok.com office -n my-namespace -o json |
  jq '
    .apiVersion = "ngrok.com/v1"
    | del(.metadata.resourceVersion, .metadata.uid,
          .metadata.creationTimestamp, .metadata.generation,
          .metadata.managedFields, .metadata.finalizers, .status)
    | if (.spec.metadata | type) == "string" then .spec.metadata |= fromjson else . end
    | if .spec.rules then .spec.rules |= map(
        if (.metadata | type) == "string" then .metadata |= fromjson else . end
      ) else . end
  ' |
  kubectl apply -f -

kubectl get ippolicies.ngrok.com office -n my-namespace
kubectl get ippolicies.ingress.k8s.ngrok.com office -n my-namespace

kubectl delete ippolicies.ingress.k8s.ngrok.com office -n my-namespace
```

Both `get` commands should show the same ID before you delete the legacy
object.

`ngrok.com/v1` `IPPolicy` accepts `metadata`, on the policy and on each rule,
only as a map of string values. The deprecated JSON-string form is rejected.
The `jq` filter above converts string-form metadata that holds a JSON object.
If a policy's metadata is free text, or has non-string values, change it to a
map of strings before applying, or `kubectl apply` fails validation.

If you manage policies with GitOps, change `apiVersion` (and any string-form
`metadata`) in the source manifest instead. Let your tool create the new object
and confirm it has the legacy object's ID before it prunes the old one.

If an `ngrok.com/v1` `IPPolicy` already had its own ID while the legacy one
still existed, for example because both were applied before the legacy one
got an ID, the two objects hold different ngrok IP policies. The operator
manages only the `ngrok.com/v1` one. The legacy object's IP policy keeps its
last rules but is no longer updated, and neither IP policy is deleted. The
operator emits an `IPPolicyIDConflict` warning event on both objects. Point
any traffic policies at the ID you want to keep, then delete the other object.

If you roll back to 0.24, it serves the legacy objects again with their IDs
unchanged. 0.24 ignores `ngrok.com/v1` `IPPolicy` objects: their IP policies
keep running but are no longer updated, and deleting one hangs on its
finalizer until you upgrade again. The `ippolicies.ngrok.com` CRD is kept on
rollback, so these objects are still there when you do.

Find legacy objects and their state:

```bash
kubectl get ippolicies.ingress.k8s.ngrok.com -A \
  -o custom-columns=NAMESPACE:.metadata.namespace,NAME:.metadata.name,ID:.status.id,REASON:'.status.conditions[?(@.type=="Ready")].reason'
```

A `REASON` of `SupersededByV1` means the legacy object is safe to delete.

### Migrate `AgentEndpoint` to `ngrok.com/v1`

`ngrok.k8s.ngrok.com/v1alpha1` `AgentEndpoint` is deprecated. Its replacement
is `ngrok.com/v1` `AgentEndpoint`. The spec is unchanged except
`spec.metadata`, which must be a map:

```yaml
apiVersion: ngrok.k8s.ngrok.com/v1alpha1
kind: AgentEndpoint
metadata:
  name: my-endpoint
spec:
  url: https://example.ngrok.app
  upstream:
    url: http://my-service.my-namespace:80
  metadata: '{"team":"edge"}'
```

becomes:

```yaml
apiVersion: ngrok.com/v1
kind: AgentEndpoint
metadata:
  name: my-endpoint
spec:
  url: https://example.ngrok.app
  upstream:
    url: http://my-service.my-namespace:80
  metadata:
    team: edge
```

Keep the name. An `ngrok.com/v1` AgentEndpoint takes over the agent endpoint of
the legacy AgentEndpoint with the same namespace and name, and the legacy one
stops driving it: it reports `Ready` with reason `SupersededByV1`, edits to it
have no effect, and the operator emits a `DeprecatedAPIGroup` warning event on
it. Deleting it then leaves the endpoint running. `kubectl apply` of a legacy
AgentEndpoint prints a deprecation warning.

For each AgentEndpoint you created, apply the `ngrok.com/v1` object, wait for
it to be ready, then delete the legacy one:

```bash
# AgentEndpoints you created; the operator's own carry its controller labels.
kubectl get agentendpoints.ngrok.k8s.ngrok.com -A \
  -l '!ngrok.com/controller-name,!k8s.ngrok.com/controller-name'

kubectl get agentendpoints.ngrok.k8s.ngrok.com my-endpoint -n my-namespace -o json |
  jq '
    .apiVersion = "ngrok.com/v1"
    | if (.spec.metadata | type) == "string"
      then .spec.metadata |= (try fromjson catch error("spec.metadata is not a JSON object; set it by hand"))
      else . end
    | del(.metadata.resourceVersion, .metadata.uid,
          .metadata.creationTimestamp, .metadata.generation,
          .metadata.managedFields, .metadata.finalizers, .status)
  ' |
  kubectl apply -f -

kubectl wait agentendpoints.ngrok.com my-endpoint -n my-namespace \
  --for=condition=Ready

kubectl delete agentendpoints.ngrok.k8s.ngrok.com my-endpoint -n my-namespace
```

The `jq` filter turns string-form metadata into a map and stops if the string
isn't a JSON object. Check the result if a value isn't a string:
`spec.metadata` only holds string values.

If you manage AgentEndpoints with GitOps, change `apiVersion` (and the
metadata form) in the source manifest instead, and let your tool prune the old
object after it has created the new one. If the old object is deleted first,
the endpoint closes and reopens, which can drop requests for about a second.
Argo CD prunes before it creates by default, so set the `PruneLast=true` sync
option on the Application (or sync without pruning, then prune). Flux prunes
after it applies.

If you delete an `ngrok.com/v1` AgentEndpoint whose legacy twin still exists,
the legacy one drives the endpoint again.

### Migrate `CloudEndpoint` to `ngrok.com/v1`

`ngrok.k8s.ngrok.com/v1alpha1` `CloudEndpoint` is deprecated. Its replacement is
`ngrok.com/v1` `CloudEndpoint`. The spec is unchanged except `spec.metadata`,
which must be a map:

```yaml
apiVersion: ngrok.k8s.ngrok.com/v1alpha1
kind: CloudEndpoint
metadata:
  name: my-endpoint
spec:
  url: https://example.ngrok.app
  trafficPolicy:
    inline:
      on_http_request:
        - actions:
            - type: custom-response
              config:
                status_code: 200
  metadata: '{"team":"edge"}'
```

becomes:

```yaml
apiVersion: ngrok.com/v1
kind: CloudEndpoint
metadata:
  name: my-endpoint
spec:
  url: https://example.ngrok.app
  trafficPolicy:
    inline:
      on_http_request:
        - actions:
            - type: custom-response
              config:
                status_code: 200
  metadata:
    team: edge
```

Both kinds are served in 0.25. **The endpoint keeps its ngrok ID.** When you
create an `ngrok.com/v1` `CloudEndpoint` with the same name and namespace as an
existing legacy one, the operator hands the existing ngrok endpoint to the new
object instead of creating another. From then on:

- The `ngrok.com/v1` object manages the endpoint. Its spec is pushed to ngrok.
- The legacy object stays `Ready`, with reason `SupersededByV1`. Changes to it
  have no effect; the operator emits a `DeprecatedAPIGroup` warning event if
  you edit it.
- Deleting the legacy object leaves the endpoint in place. Deleting the
  `ngrok.com/v1` object instead hands the endpoint back to the legacy object.
  The ngrok endpoint is deleted only when neither object remains.

`kubectl apply` of a legacy `CloudEndpoint` prints a deprecation warning.

Keep the name, not just the URL: the two objects are matched by namespace and
name. If the two objects have different `spec.url` values, the endpoint moves
to the `ngrok.com/v1` URL as soon as it takes over, and traffic on the old URL
stops. Distinct names on one URL stay separate endpoints, as pooling allows.

Both CRDs use the plural `cloudendpoints` and the short name `clep`. Use the
fully qualified names below in scripts. For each CloudEndpoint you created,
apply the `ngrok.com/v1` object, confirm it has the same ID, then delete the
legacy one:

```bash
# CloudEndpoints you created; the operator's own carry its controller labels
# or a Service owner.
kubectl get cloudendpoints.ngrok.k8s.ngrok.com -A \
  -l '!ngrok.com/controller-name,!k8s.ngrok.com/controller-name'

kubectl get cloudendpoints.ngrok.k8s.ngrok.com my-endpoint -n my-namespace -o json |
  jq '
    .apiVersion = "ngrok.com/v1"
    | if (.spec.metadata | type) == "string"
      then .spec.metadata |= (try fromjson catch error("spec.metadata is not a JSON object; set it by hand"))
      else . end
    | if (.spec.metadata | type) == "object"
      then .spec.metadata |= with_entries(select(.value | type == "string"))
      else . end
    | del(.metadata.resourceVersion, .metadata.uid,
          .metadata.creationTimestamp, .metadata.generation,
          .metadata.managedFields, .metadata.finalizers, .status)
  ' |
  kubectl apply -f -

kubectl get cloudendpoints.ngrok.com my-endpoint -n my-namespace
kubectl get cloudendpoints.ngrok.k8s.ngrok.com my-endpoint -n my-namespace

kubectl delete cloudendpoints.ngrok.k8s.ngrok.com my-endpoint -n my-namespace
```

Both `get` commands should show the same ID before you delete the legacy
object.

`ngrok.com/v1` `CloudEndpoint` accepts `metadata` only as a map of string
values. The `jq` filter converts string-form metadata that holds a JSON object,
and **drops any value that isn't a string**, such as `{"replicas": 3}`, from the
endpoint. Free-text metadata makes the filter stop; set it by hand.

If you manage CloudEndpoints with GitOps, change `apiVersion` (and any
string-form `metadata`) in the source manifest instead. Let your tool create the
new object and confirm it has the legacy object's ID before it prunes the old
one. If the old object is deleted first, its endpoint is deleted and the new
object creates another, with a new ID, which drops traffic for a few seconds.
Argo CD prunes before it creates by default, so set the `PruneLast=true` sync
option on the Application (or sync without pruning, then prune). Flux prunes
after it applies.

CloudEndpoints the operator generates for Ingresses, Gateway API routes, and
Services need no action.

If the two objects hold different ngrok endpoints, for example because both
were applied before the legacy one got an ID, the operator manages only the
`ngrok.com/v1` one, deletes neither endpoint, and emits a
`CloudEndpointIDConflict` warning event on both objects. Delete the object
whose endpoint you don't need; deleting an object deletes its endpoint.

Find legacy objects and their state:

```bash
kubectl get cloudendpoints.ngrok.k8s.ngrok.com -A \
  -o custom-columns=NAMESPACE:.metadata.namespace,NAME:.metadata.name,ID:.status.id,REASON:'.status.conditions[?(@.type=="Ready")].reason'
```

A `REASON` of `SupersededByV1` means the legacy object is safe to delete.

## Prepare for later cleanup releases

0.25 still retains some compatibility code. Complete the user-managed
migrations before their cleanup releases:

| Migration | 0.25 behavior | Future requirement |
| --- | --- | --- |
| `NgrokTrafficPolicy` kind | Serves both CRDs; prefers `TrafficPolicy` | Migrate to `ngrok.com/v1` `TrafficPolicy` before the cleanup release removes the `ngroktrafficpolicies.ngrok.k8s.ngrok.com` CRD. Objects still stored under it are deleted with the CRD |
| `KubernetesOperator` kind | Operator writes `ngrok.com/v1`; the legacy object is left in place, passive, with its finalizer | None. 1.0 removes the legacy finalizer and 1.1 removes the `kubernetesoperators.ngrok.k8s.ngrok.com` CRD and the legacy object with it. 0.25 and 1.0 are both required stops: do not skip either |
| `ingress.k8s.ngrok.com/v1alpha1` `Domain` | Serves both CRDs; the operator creates and prefers `ngrok.com/v1` Domains, and migrates the ones it created | Re-stamp the Domains you created (see [`Domain` moves to `ngrok.com/v1`](#domain-moves-to-ngrokcomv1)) before 1.1. 1.0 stops reconciling legacy Domains and removes their finalizers; a legacy Domain with no `ngrok.com/v1` twin keeps its reservation in ngrok, unmanaged, until you re-stamp it. 1.1 removes the `domains.ingress.k8s.ngrok.com` CRD. Upgrade through both 0.25 and 1.0; see [CRD Group Migration](../specs/crd-group-migration.md#release-plan) |
| `IPPolicy` in `ingress.k8s.ngrok.com` | Serves both CRDs; an `ngrok.com/v1` `IPPolicy` takes over the legacy object's ngrok IP policy | Migrate to `ngrok.com/v1` `IPPolicy` before upgrading to 1.0, which stops reconciling the legacy kind. 1.1 removes the `ippolicies.ingress.k8s.ngrok.com` CRD; a policy re-stamped after that gets a new ID |
| `ngrok.k8s.ngrok.com/v1alpha1` `AgentEndpoint` kind | Serves both CRDs; the `ngrok.com/v1` twin drives the endpoint | Migrate your AgentEndpoints to `ngrok.com/v1` before 1.0. The agent runs the endpoint itself, so a legacy AgentEndpoint goes offline as soon as 1.0 is running |
| `ngrok.k8s.ngrok.com/v1alpha1` `CloudEndpoint` kind | Serves both CRDs; an `ngrok.com/v1` `CloudEndpoint` takes over the legacy object's ngrok endpoint | Migrate your CloudEndpoints to `ngrok.com/v1` before upgrading to 1.0, which stops reconciling the legacy kind; its endpoint keeps running unmanaged. A late re-stamp keeps its ID until 1.1 removes the legacy CRD, after which it gets a new one |
| User annotations and `appProtocol` | Reads old and new forms | Remove all `k8s.ngrok.com/*` user configuration before 1.0 |
| CRD `spec.metadata` | Reads JSON strings and maps | Use maps before the announced cleanup release |
| Operator-written labels, annotations, and finalizer | Writes new prefix only; still reads legacy | None. The next release stops reading the legacy prefix, so upgrade through 0.25 rather than skipping it |
