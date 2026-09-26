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

## Before upgrading

### Back up the release configuration

Set these to match your installation:

```bash
RELEASE=ngrok-operator
NAMESPACE=ngrok-operator

helm get values "$RELEASE" --namespace "$NAMESPACE" --all \
  > ngrok-operator-0.24-values.yaml
helm get manifest "$RELEASE" --namespace "$NAMESPACE" \
  > ngrok-operator-0.24-manifest.yaml
```

Keep these backups until the upgrade and its rollback window have closed. Do
not use the output of `helm get values --all` as a new long-term values file;
it contains defaults from the old chart.

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
