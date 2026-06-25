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

### CloudEndpoint: `spec.trafficPolicyName` → `spec.trafficPolicy.targetRef.name`, and `spec.trafficPolicy.policy` → `spec.trafficPolicy.inline`

Status: in progress starting 0.24, cleanup planned for a later 1.0 release.

The `CloudEndpoint` traffic policy fields are being consolidated onto the
same shape `AgentEndpoint` already uses (`spec.trafficPolicy.inline` /
`spec.trafficPolicy.targetRef`). Two legacy fields are deprecated in
parallel:

- `spec.trafficPolicyName` — replaced by `spec.trafficPolicy.targetRef.name`.
  `targetRef.namespace` is also now supported for cross-namespace references.
- `spec.trafficPolicy.policy` — replaced by `spec.trafficPolicy.inline`.

The operator dual-reads both shapes during the deprecation window. When a
legacy field is set alongside its canonical replacement, the canonical
field wins and the legacy field is ignored with a `DeprecatedField`
warning event. Manifests carrying only the legacy field continue to work
unchanged.

#### What changes for you

| Legacy                                       | New                                        |
| -------------------------------------------- | ------------------------------------------ |
| `spec.trafficPolicyName: my-policy`          | `spec.trafficPolicy.targetRef.name: my-policy` |
| `spec.trafficPolicy.policy: { ... }`         | `spec.trafficPolicy.inline: { ... }`        |

The new `targetRef` also supports `namespace` so a `CloudEndpoint` can
reference an `NgrokTrafficPolicy` in a different namespace. The operator's
RBAC scope is the trust boundary for those cross-namespace references —
see "Cross-namespace policy references" below.

#### Rollback safety during the migration window

These two field renames have different rollback shapes because of how the
prior 0.23 CRD validated each one. Plan your migration accordingly.

**Top-level rename (`trafficPolicyName` → `trafficPolicy.targetRef.name`)
— rollback-safe with the legacy field _only_:**

The 0.23 controller rejects manifests where both `spec.trafficPolicyName`
and `spec.trafficPolicy` are set (invalid-config error). And because the
0.23 CRD does not know about `trafficPolicy.targetRef`, server-side
pruning strips that field on rollback, leaving `trafficPolicy: {}` — which
the 0.23 controller treats as the rejected "both set" case.

So during 0.24, leave existing manifests on `trafficPolicyName` alone
(canonical-only and dual-set are not rollback-safe to pre-0.24). Migrate
to `trafficPolicy.targetRef.name` only once you no longer need to roll
back below 0.24.

```yaml
# Rollback-safe to 0.23: legacy-only during the migration window.
spec:
  url: https://my-endpoint.internal
  trafficPolicyName: my-policy
```

The 0.24 controller normalizes legacy-only manifests in-memory to the
canonical shape, so you keep all the new behavior (status, conditions,
re-enqueue on policy changes) without changing the manifest.

**Nested rename (`trafficPolicy.policy` → `trafficPolicy.inline`) —
rollback-safe when dual-set:**

The 0.23 CRD preserves unknown fields under `trafficPolicy.policy` but
prunes the unknown `trafficPolicy.inline` on rollback. If you set both
fields with the same content, the 0.24 controller prefers `inline` and
the 0.23 controller reads `policy`. The combination is rollback-safe.

```yaml
# Rollback-safe to 0.23: dual-set the inline content.
spec:
  url: https://my-endpoint.internal
  trafficPolicy:
    inline: { on_http_request: [{ actions: [{ type: deny }] }] }
    policy: { on_http_request: [{ actions: [{ type: deny }] }] }
```

The operator-generated CloudEndpoints (those created by the
Ingress/Gateway/Service controllers from your manifests) automatically
dual-write both `inline` and `policy` so they remain rollback-safe
without any user action. Drop the `policy` value from your own manifests
in the cleanup release.

#### Action required, by release

| Release | Operator behavior | What you do |
| ------- | ----------------- | ----------- |
| 0.24 (this) | Reads both shapes; canonical wins when both are effective; emits a `DeprecatedField` warning event when a legacy field is in use. | Keep `trafficPolicyName` as the only top-level form during 0.24 if you need rollback to 0.23. For nested policies you can dual-set `inline + policy` for rollback safety. |
| Next | Same. | Once rollback to 0.23 or earlier is no longer a concern, migrate `trafficPolicyName` → `trafficPolicy.targetRef.name`. Manifests that dual-set the nested form can drop `policy`. |
| Cleanup release | Legacy fields removed from the `CloudEndpoint` CRD. | Manifests must use the canonical fields. |

#### Supported upgrade path

Any prior 0.2x release → 0.24 → … → cleanup release. A rollback from
0.24 to 0.23 is safe **only if your manifests still match the prior
release's accepted shapes** (legacy-only top-level form, and either
`policy`-only or dual-set nested form).

#### External tooling

If you have GitOps validators, dashboards, custom admission policies, or
linters that match on `spec.trafficPolicyName` or
`spec.trafficPolicy.policy` literally, update those selectors to also
recognize `spec.trafficPolicy.targetRef.name` / `spec.trafficPolicy.inline`
before the cleanup release.

The `Traffic Policy` column on `kubectl get cloudendpoint` was removed
in 0.24. The old column rendered the legacy `spec.trafficPolicyName`
field; once cross-namespace `targetRef` and inline policies became
first-class shapes a single string summary was misleading. Use
`kubectl describe cloudendpoint <name>` or the `TrafficPolicyApplied`
status condition (visible in the `Ready`/`Reason`/`Message` columns
when policy resolution fails) instead.

#### Go API surface change

If you import `github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1`
directly from a typed Go client, note that `CloudEndpointSpec.TrafficPolicy`
changed type from `*NgrokTrafficPolicySpec` to
`*CloudEndpointTrafficPolicyCfg`. The JSON/YAML wire format is unchanged
— Helm values, kustomize overlays, raw manifests, and unstructured
clients are unaffected — but typed consumers will fail to compile until
they switch to the new struct. The new type carries the canonical
`Inline` / `Reference` fields plus the deprecated `Policy` fold-in for
rollback safety during the migration window.

#### Cross-namespace policy references

`spec.trafficPolicy.targetRef.namespace` lets a CloudEndpoint or
AgentEndpoint attach to an `NgrokTrafficPolicy` in another namespace.
There is no per-resource opt-in (no ReferenceGrant equivalent) for these
references in 0.24 — the operator's RBAC/watch scope is the trust
boundary. In practice:

- A cluster-scoped install can resolve refs across all namespaces.
- A namespace-scoped install (the chart's `watchNamespace` configuration)
  can only resolve refs to namespaces it watches. Cross-namespace refs
  outside that scope surface as `TrafficPolicyApplied=False` with a
  `TrafficPolicyNotFound` event.

If you do not want one namespace's policies attachable from another,
either run the operator namespace-scoped or restrict creation of
`AgentEndpoint`/`CloudEndpoint` via your own RBAC.
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

## What did *not* change in this set of migrations

The CRD API groups (`ingress.k8s.ngrok.com/v1alpha1`,
`ngrok.k8s.ngrok.com/v1alpha1`, `bindings.k8s.ngrok.com/v1alpha1`) are
**unchanged**. A separate 1.0 workstream will consolidate these into
`ngrok.com/v1` with a conversion webhook; that migration will appear here
when it begins.
