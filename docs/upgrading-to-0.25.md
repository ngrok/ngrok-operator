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
and set `credentials.accessToken` instead:

```yaml
credentials:
  accessToken: "<your-access-token>"
```

If you pass credentials on the command line, replace both `--set` flags with
one:

```bash
helm upgrade "$RELEASE" ngrok/ngrok-operator \
  --namespace "$NAMESPACE" \
  --set credentials.accessToken="$NGROK_ACCESS_TOKEN"
```

Passing either removed value now fails the render:

```
Error: UPGRADE FAILED: execution error at (ngrok-operator/templates/api-manager/deployment.yaml:30:28):
credentials.apiKey and credentials.authtoken have been replaced by a single
credentials.accessToken (an ngrok access token).
```

The error names the api-manager deployment because it is the first template to
pull in the credentials Secret, for its checksum annotation. The removed values
are what it is complaining about.

This includes values carried forward implicitly. If you upgrade with
`--reuse-values`, the old keys come along from the previous release and the
upgrade fails until you remove them.

### Create a new Secret if you manage your own

If you manage the credentials Secret yourself and point at it with
`credentials.secret.name`, create a new Secret under a new name rather than
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

The 0.25 chart reorganizes its values. Operator settings move under `ngrok`,
`features` and per-component `config` sections, and pod settings move under
`defaults` or a component section. There are no aliases for the old keys: a
values file that still sets one fails the render and lists each old key with
its new location.

| 0.24 value | 0.25 value |
|------------|------------|
| `description`, `region`, `rootCAs`, `serverAddr`, `apiURL`, `clusterDomain` | `ngrok.<same>` |
| `ngrokMetadata`, `metaData` | `ngrok.metadata` |
| `ingress.*` | `features.ingress.*` |
| `watchNamespace`, `controllerName`, `ingressClass.*` | `features.ingress.watchNamespace`, `features.ingress.controllerName`, `features.ingress.ingressClass.*` |
| `gateway.*` | `features.gateway.*` |
| `bindings.enabled`, `bindings.endpointSelectors`, `bindings.serviceAnnotations`, `bindings.serviceLabels`, `bindings.ingressEndpoint` | `features.bindings.<same>` |
| `bindings.forwarder.*` | `bindingsForwarder.*` |
| `drainPolicy`, `defaultDomainReclaimPolicy` | `features.<same>` |
| `oneClickDemoMode` | `apiManager.config.oneClickDemoMode` |
| `podAnnotations`, `podLabels`, `nodeSelector`, `tolerations`, `affinity`, `podAffinityPreset`, `podAntiAffinityPreset`, `nodeAffinityPreset`, `topologySpreadConstraints`, `priorityClassName`, `extraEnv` | `defaults.<same>` (applies to every component) |
| `replicaCount`, `resources`, `lifecycle`, `terminationGracePeriodSeconds`, `extraVolumes`, `extraVolumeMounts`, `podDisruptionBudget.*`, `serviceAccount.*` | `apiManager.<same>` |
| `log.*`, `agent.*`, `credentials.*`, `image.*` | unchanged |

Three behaviors change along with the keys:

- Top-level pod settings such as `podAnnotations` and `tolerations` now apply to
  the agent and bindings-forwarder as well as the api-manager. To keep a
  setting on one component only, set it on that component instead, for example
  `apiManager.tolerations`.
- Log settings can be overridden for one component under
  `<component>.config.log`, for example `agent.config.log.level: debug`.
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

0.25 adds a new CRD, `trafficpolicies.ngrok.com` (see
[Migrate `NgrokTrafficPolicy` to `TrafficPolicy`](#migrate-ngroktrafficpolicy-to-trafficpolicy)),
and the operator watches it on startup. If you install the CRD chart
separately, upgrade `ngrok/ngrok-crds` to `0.5.x` before upgrading the operator
chart, or the operator will not start.

### Custom RBAC

The chart adds rules for `trafficpolicies` and `trafficpolicies/status` in the
`ngrok.com` API group to the api-manager and agent roles, plus
`trafficpolicy-editor` and `trafficpolicy-viewer` aggregated ClusterRoles. No
change is required when the chart manages RBAC. If you supply roles
separately, add the equivalent rules before upgrading.

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
kubectl get kubernetesoperators.ngrok.k8s.ngrok.com -A
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

To set them, use these instead of `credentials.accessToken`:

```yaml
credentials:
  agent:
    accessToken: "<token that can start tunnels>"
  apiManager:
    accessToken: "<token with ngrok API permissions>"
```

Either may be set on its own alongside `credentials.accessToken`, which the
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
  --set credentials.accessToken="$NEW_NGROK_ACCESS_TOKEN"
```

Revoke the old token in the dashboard once the rollout completes.

This works because the checksum is taken over the Secret the chart renders. If
you manage the Secret yourself with `credentials.secret.name`, the chart renders
nothing, the checksum never changes, and neither pod restarts — the token is
read from the environment once at startup, so both keep using the old value.
Restart them yourself after rotating, before revoking the old token:

```bash
kubectl rollout restart --namespace "$NAMESPACE" \
  deploy/"$FULLNAME"-manager deploy/"$FULLNAME"-agent
```

## Rolling back

To return to `0.24.x` within the rollback window, reinstall with the backups
captured above:

```bash
helm upgrade "$RELEASE" ngrok/ngrok-operator \
  --namespace "$NAMESPACE" --version 0.24.0 \
  --values ngrok-operator-0.24-values.yaml --wait
```

Rolling back to 0.24 is safe for objects the 0.25 operator wrote: 0.24 reads
both label, annotation, and finalizer prefixes and re-adds the legacy forms on
its next reconcile.

Do not start the "After the rollback window" migration below until rollback is
ruled out. 0.24 does not know about `ngrok.com/v1` `TrafficPolicy`, and rolling
back the CRD chart removes the `trafficpolicies.ngrok.com` CRD, which deletes
every `TrafficPolicy` object stored under it.

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

## Prepare for later cleanup releases

0.25 still retains some compatibility code. Complete the user-managed
migrations before their cleanup releases:

| Migration | 0.25 behavior | Future requirement |
| --- | --- | --- |
| `NgrokTrafficPolicy` kind | Serves both CRDs; prefers `TrafficPolicy` | Migrate to `ngrok.com/v1` `TrafficPolicy` before the cleanup release removes the `ngroktrafficpolicies.ngrok.k8s.ngrok.com` CRD. Objects still stored under it are deleted with the CRD |
| User annotations and `appProtocol` | Reads old and new forms | Remove all `k8s.ngrok.com/*` user configuration before 1.0 |
| CloudEndpoint traffic-policy fields | Reads old and new fields | Use only `targetRef` and `inline` before the announced cleanup release |
| CRD `spec.metadata` | Reads JSON strings and maps | Use maps before the announced cleanup release |
| Operator-written labels, annotations, and finalizer | Writes new prefix only; still reads legacy | None. The next release stops reading the legacy prefix, so upgrade through 0.25 rather than skipping it |
