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

## What did *not* change in this set of migrations

The CRD API groups (`ingress.k8s.ngrok.com/v1alpha1`,
`ngrok.k8s.ngrok.com/v1alpha1`, `bindings.k8s.ngrok.com/v1alpha1`) are
**unchanged**. A separate 1.0 workstream will consolidate these into
`ngrok.com/v1` with a conversion webhook; that migration will appear here
when it begins.
