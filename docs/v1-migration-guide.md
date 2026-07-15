# ngrok-operator v1 migration guide

This guide tracks the backwards-incompatible changes the ngrok-operator is
making on the path to 1.0. Each migration is staged across multiple releases
so existing manifests and running deployments keep working during the
transition window. Migrate during the release noted under "Action required";
read-side compatibility code is removed in the listed cleanup release.

If you are an operator maintainer auditing the read-side / write-side shims
that implement these transitions, see
[`docs/developer-guide/passivity-shims.md`](./developer-guide/passivity-shims.md).

## Migrations

### Controller labels, computed-url, and bindings labels: `k8s.ngrok.com/` → `ngrok.com/`

Status: in progress. The first release (0.24) reads and writes both
prefixes. Later cleanup releases are planned but not yet pinned to firm
versions — see the role/release table in
[`docs/developer-guide/passivity-shims.md`](./developer-guide/passivity-shims.md).

This migration covers three sets of operator-written keys: the
controller-identity labels the operator stamps on `AgentEndpoint` /
`CloudEndpoint` objects, the `computed-url` annotation the Service
controller writes back onto LoadBalancer Services, and the bindings labels
the BoundEndpoint controller writes onto upstream Services (plus the
`endpoint-url` annotation that accompanies them).

All three follow the same staged pattern across three phases: the
**migration release** (0.24) reads and writes both prefixes; the
**write-side cleanup** release writes the new prefix only and removes
legacy keys on next reconcile while still reading both; the **read-side
cleanup** release drops the legacy-read code entirely.

#### What changes for you

Most of these keys are operator-internal and require no action; one set is
worth checking:

- **Controller labels** (`controller-name` / `controller-namespace`): these
  are how the operator identifies and garbage-collects the resources it
  owns. They are not a user contract — you should not write them yourself
  (an object you stamp with them may get adopted and deleted by the
  operator) and you generally should not select on them. The rename matters
  for **rollback safety**, not for your tooling, and is handled entirely by
  the operator.
- **`computed-url` annotation**: operator-written onto LoadBalancer
  Services. Internal; no action needed. Legacy keys may already exist on
  Services upgraded from earlier releases and are migrated automatically.
- **Bindings labels** (`endpoint-binding-name` / `endpoint-binding-namespace`):
  these are user-discoverable — if dashboards, monitoring, or GitOps tools
  select on them, plan to update those selectors before the write-side
  cleanup release. The operator writes both prefixes during 0.24, so existing
  selectors keep matching; they stop matching once the write-side cleanup
  release removes the legacy keys on reconcile. The accompanying
  `endpoint-url` **annotation** is operator-written and not selectable; no
  action is needed for it.

| Legacy                                                | New                                    | Notes                                                                       |
| ----------------------------------------------------- | -------------------------------------- | --------------------------------------------------------------------------- |
| `k8s.ngrok.com/controller-name`                       | `ngrok.com/controller-name`            | Labels on `AgentEndpoint` / `CloudEndpoint`.                                |
| `k8s.ngrok.com/controller-namespace`                  | `ngrok.com/controller-namespace`       | Labels on `AgentEndpoint` / `CloudEndpoint`.                                |
| `k8s.ngrok.com/computed-url`                          | `ngrok.com/computed-url`               | Annotation written back on LoadBalancer Services. Operator-written, so legacy keys may already exist on Services upgraded from earlier releases. |
| `bindings.k8s.ngrok.com/endpoint-binding-name`        | `ngrok.com/endpoint-binding-name`      | Labels on Services owned by `BoundEndpoint`.                                |
| `bindings.k8s.ngrok.com/endpoint-binding-namespace`   | `ngrok.com/endpoint-binding-namespace` | Labels on Services owned by `BoundEndpoint`.                                |
| `bindings.k8s.ngrok.com/endpoint-url`                 | `ngrok.com/endpoint-url`               | Annotation on Services owned by `BoundEndpoint`.                            |

#### Action required, by phase

The only action that may apply to you concerns the bindings labels above;
the controller labels and `computed-url` are operator-internal.

| Phase                       | Operator reads  | Operator writes                                         | What you do                                                                                          |
| --------------------------- | --------------- | ------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| Migration release (0.24)    | Both prefixes   | **Both** prefixes for all keys in the table above.      | Nothing required. Optionally start updating any bindings-label selectors to the new prefix; both work. |
| Write-side cleanup (planned 1.0) | Both prefixes   | New prefix only; legacy keys removed on next reconcile. | Finish updating any external selectors on the bindings labels to the new prefix.                |
| Read-side cleanup (planned 1.1)  | New prefix only | New prefix only.                                        | Confirm nothing external still selects on the legacy bindings keys. The operator no longer writes or reads them. |

### Finalizer prefix: `k8s.ngrok.com/finalizer` → `ngrok.com/finalizer`

Status: in progress across 0.24 → 0.25 → 0.26.

The operator finalizer is being renamed to align with the broader
`k8s.ngrok.com/` → `ngrok.com/` prefix unification. Because finalizers gate
object deletion, this migration uses a three-release pattern: the operator
single-writes one key at any given time, and only the *identity* of that
key changes between releases.

#### What changes for you

| Legacy                      | New                    |
| --------------------------- | ---------------------- |
| `k8s.ngrok.com/finalizer`   | `ngrok.com/finalizer`  |

The finalizer key is internal to the operator and not something most users
select on. If your external tooling (dashboards, GitOps validators, custom
admission policies) looks for the finalizer literal, plan to update those
selectors before the cleanup release.

#### Action required, by release

| Release | Reads | Operator writes | What you do |
| ------- | ----- | --------------- | ----------- |
| 0.24 (this) | Both prefixes | Legacy finalizer only | Nothing. |
| 0.25 | Both prefixes | New finalizer; legacy stripped on next reconcile | Nothing required; if external tooling matches the literal, update it now. |
| 0.26 | New prefix only | New finalizer only | Confirm no external tooling still references the legacy finalizer. |

#### Supported upgrade path

`previous-stable → 0.24 → 0.25 → 0.26`. The intermediate 0.24 step is
required for finalizers specifically: a direct jump to 0.25 leaves
in-flight deletions on objects an older operator stamped with the legacy
finalizer at the mercy of a rollback. 0.24 is the rollback-safe checkpoint
where every object carries the legacy finalizer and either release can
drive a deletion to completion. See the developer guide for the full
rationale.

### IngressClass `spec.controller`: `k8s.ngrok.com/ingress-controller` → `ngrok.com/ingress-controller`

Status: in progress across 0.24 → 0.25.

The operator binary's default `--ingress-controller-name` flips to
`ngrok.com/ingress-controller` in 0.24. To keep existing IngressClasses
matching during the transition, the operator dual-matches both
`k8s.ngrok.com/ingress-controller` and `ngrok.com/ingress-controller`
whenever its configured `controllerName` equals either of those two stock
defaults — which is the case for both the new binary default and the
legacy value still rendered by the helm chart in 0.24. Custom controller
names retain exact-match behavior so multi-instance isolation is
preserved.

The helm chart deliberately **does not** flip the rendered IngressClass
`spec.controller` in 0.24. A `helm upgrade` applies the new manifest while
the previous operator pod is still running — flipping the rendered value
at the same time as the operator image would briefly leave the
pre-migration operator unable to match its own IngressClass. The helm
chart's rendered controller value stays on `k8s.ngrok.com/ingress-controller`
through 0.24 and flips to `ngrok.com/ingress-controller` in 0.25, by which
point no pre-migration operator pod can be running.

#### What changes for you

| Legacy                                  | New                                |
| --------------------------------------- | ---------------------------------- |
| `k8s.ngrok.com/ingress-controller`      | `ngrok.com/ingress-controller`     |

If you author your own IngressClass manifests, you can adopt the new value
any time in 0.24 — both work. If you rely on the helm-rendered
IngressClass, no action is required; the chart manages the transition for
you.

#### Action required, by release

| Release | Operator binary default | Helm-rendered IngressClass | What you do |
| ------- | ----------------------- | -------------------------- | ----------- |
| 0.24 (this) | `ngrok.com/ingress-controller`, dual-matches legacy | `k8s.ngrok.com/ingress-controller` (unchanged) | Nothing required. |
| 0.25 | `ngrok.com/ingress-controller`, dual-matches legacy | `ngrok.com/ingress-controller` | Nothing required if you use the helm chart. |
| 0.26 | `ngrok.com/ingress-controller`, exact-match | `ngrok.com/ingress-controller` | Confirm no `k8s.ngrok.com/ingress-controller` IngressClasses remain in self-authored manifests. |

#### Supported upgrade path

`previous-stable → 0.24 → 0.25 → 0.26`. Skipping 0.24 is unsupported
because the rendered IngressClass and the operator's controller name flip
together without an intermediate dual-match release.

### User-set annotations: `k8s.ngrok.com/` → `ngrok.com/`

Status: in progress. 0.24 reads both prefixes; **1.0 reads `ngrok.com/`
only**. Unlike the operator-written keys above, these annotations live in
*your* manifests — the operator cannot migrate them for you, so the legacy
prefix's removal lands exactly at the 1.0 major version.

#### What changes for you

These are the affected keys. The 0.24 operator reads both prefixes; if both
are present on the same object, the `ngrok.com/` value wins.

| Legacy                                  | New                                | Applies to                |
| --------------------------------------- | ----------------------------------- | -------------------------- |
| `k8s.ngrok.com/url`                     | `ngrok.com/url`                    | Service (LoadBalancer)    |
| `k8s.ngrok.com/mapping-strategy`        | `ngrok.com/mapping-strategy`       | Service, Ingress, Gateway |
| `k8s.ngrok.com/traffic-policy`          | `ngrok.com/traffic-policy`         | Service, Ingress, Gateway |
| `k8s.ngrok.com/pooling-enabled`         | `ngrok.com/pooling-enabled`        | Service, Ingress, Gateway |
| `k8s.ngrok.com/bindings`                | `ngrok.com/bindings`               | Service, Ingress, Gateway |
| `k8s.ngrok.com/metadata`                | `ngrok.com/metadata`               | Ingress, Gateway          |
| `k8s.ngrok.com/description`             | `ngrok.com/description`            | Ingress, Gateway          |
| `k8s.ngrok.com/app-protocols`           | `ngrok.com/app-protocols`          | Service backing an Ingress / Gateway route |
| `k8s.ngrok.com/terminate-tls.<option>`  | `ngrok.com/terminate-tls.<option>` | Gateway listener TLS options |

The Service port `appProtocol` field value `k8s.ngrok.com/http2` also has a
new spelling, `ngrok.com/http2`; both are recognized through 0.24.

Pod annotations forwarded as bindings pod identity are also affected: the
forwarder passes along pod annotations under either prefix during the
migration window, but keys are forwarded **verbatim** — if your ngrok
traffic-policy expressions match on `k8s.ngrok.com/*` pod-annotation keys,
update the pod annotations and the policy expressions together; from 1.0 the
forwarder only passes `ngrok.com/*` keys.

**Labels:** there are no user-written ngrok-prefixed labels. This was
audited during this migration — the only prefixed label families are the
operator-written controller and bindings labels, covered by the
operator-written keys migration above.

#### How to migrate

The default, rollback-safe procedure:

1. On 0.24, **add** each `ngrok.com/` key alongside its `k8s.ngrok.com/`
   twin with the same value. With both present the operator uses the
   `ngrok.com/` key, and a rollback to a pre-0.24 operator still reads the
   legacy one — behavior is identical on both sides of a rollback.
2. Once rolling back below 0.24 is no longer possible, delete the legacy
   keys.
3. Finish both steps before upgrading to 1.0 — from 1.0 the operator reads
   `ngrok.com/` only.

If a rollback below 0.24 is already ruled out (or you can roll your
manifests back together with the operator), a straight rename is
equivalent. The two-step dance exists only because a pre-0.24 operator
silently ignores `ngrok.com/*` keys — after a rollback an endpoint would
keep serving, but without its traffic policy, bindings, or URL settings.

**`appProtocol` cannot dual-key:** `Service.spec.ports[].appProtocol` is a
single scalar value, so the recipe above does not apply to it. Switching a
port from `k8s.ngrok.com/http2` to `ngrok.com/http2` and then rolling back
below 0.24 silently drops HTTP/2 for that upstream. Keep the legacy value
until a rollback below 0.24 is ruled out, then switch it — before 1.0.

#### Finding legacy keys

When the operator reconciles an Ingress, Gateway, or LoadBalancer Service
it manages that carries legacy-prefixed annotations, it emits a Warning
event with reason `LegacyAnnotation`:

    kubectl get events -A --field-selector reason=LegacyAnnotation

Treat events as a best-effort immediate signal, **not** as proof your
cluster is ready for 1.0: they expire (typically after an hour), objects
that fail earlier reconcile checks are not scanned, and several surfaces
are log-only (see the exceptions below). The scan is also key-based, not
kind-aware — a stray legacy key that does nothing on that resource kind is
still flagged; deleting it is as valid as renaming it. For a complete
point-in-time inventory, audit directly:

```sh
# Ingresses, Gateways, and Services with legacy-prefixed annotations
for kind in ingress gateway service; do
  kubectl get "$kind" -A -o json | jq -r --arg k "$kind" \
    '.items[] | select((.metadata.annotations // {}) | keys | any(startswith("k8s.ngrok.com/"))) | "\($k) \(.metadata.namespace)/\(.metadata.name)"'
done

# Gateway listeners with legacy TLS option keys
kubectl get gateway -A -o json | jq -r \
  '.items[] | select([.spec.listeners[]?.tls.options // {} | keys[]] | any(startswith("k8s.ngrok.com/"))) | "gateway \(.metadata.namespace)/\(.metadata.name)"'

# Service ports with the legacy appProtocol value
kubectl get service -A -o json | jq -r \
  '.items[] | select([.spec.ports[]?.appProtocol] | any(. == "k8s.ngrok.com/http2")) | "service \(.metadata.namespace)/\(.metadata.name)"'
```

> **Exceptions (no events):**
>
> - `k8s.ngrok.com/app-protocols` and the `k8s.ngrok.com/http2` appProtocol
>   value are read from the backend Service of an Ingress or Gateway route —
>   those Services are not reconciled directly, so legacy use surfaces in the
>   operator logs only. Grep the logs for `legacy annotation key in use` and
>   `legacy appProtocol value in use`.
> - Legacy-prefixed **pod annotations** forwarded as bindings pod identity
>   produce no events or logs (they are read per connection on a hot path).
>   Audit for them directly:
>
>   ```sh
>   kubectl get pods -A -o json | jq -r \
>     '.items[] | select((.metadata.annotations // {}) | keys | any(startswith("k8s.ngrok.com/"))) | "\(.metadata.namespace)/\(.metadata.name)"'
>   ```

#### Action required, by release

| Release | Reads | What you do |
| ------- | ----- | ----------- |
| 0.24 (this) | Both prefixes | Add `ngrok.com/` keys alongside the legacy ones (see *How to migrate*); drop the legacy keys once rollback below 0.24 is ruled out. Use the `LegacyAnnotation` events and the audit commands above to find stragglers. |
| 1.0 | `ngrok.com/` only | Confirm no `k8s.ngrok.com/` annotation keys remain in your manifests. The operator no longer reads them. |

## What did *not* change in this set of migrations

The CRD API groups (`ingress.k8s.ngrok.com/v1alpha1`,
`ngrok.k8s.ngrok.com/v1alpha1`, `bindings.k8s.ngrok.com/v1alpha1`) are
**unchanged**. A separate 1.0 workstream will consolidate these into
`ngrok.com/v1` with a conversion webhook; that migration will appear here
when it begins.
