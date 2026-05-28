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

### Annotation, label, finalizer, and IngressClass prefix: `k8s.ngrok.com/` → `ngrok.com/`

Status: in progress across 0.24 → 0.25 → 0.26.

The operator is unifying its annotation, label, finalizer, and
IngressClass-controller naming on a single prefix: `ngrok.com/`. Multiple
releases are required so the migration is rollback-safe even if you need to
revert to a prior release mid-flight.

#### What changes for you

**User-set annotations (you should edit your YAML before the cleanup
release):**

| Legacy                              | New                              | Applies to                          |
| ----------------------------------- | -------------------------------- | ----------------------------------- |
| `k8s.ngrok.com/url`                 | `ngrok.com/url`                  | Service (LoadBalancer)              |
| `k8s.ngrok.com/mapping-strategy`    | `ngrok.com/mapping-strategy`     | Service, Ingress, Gateway           |
| `k8s.ngrok.com/traffic-policy`      | `ngrok.com/traffic-policy`       | Service, Ingress, Gateway           |
| `k8s.ngrok.com/pooling-enabled`     | `ngrok.com/pooling-enabled`      | Service, Ingress, Gateway           |
| `k8s.ngrok.com/bindings`            | `ngrok.com/bindings`             | Service, Ingress, Gateway           |
| `k8s.ngrok.com/metadata`            | `ngrok.com/metadata`             | Ingress, Gateway                    |
| `k8s.ngrok.com/description`         | `ngrok.com/description`          | Ingress, Gateway                    |
| `k8s.ngrok.com/app-protocols`       | `ngrok.com/app-protocols`        | Service (LoadBalancer)              |

The legacy Service `appProtocol` field value `k8s.ngrok.com/http2` continues
to be recognized; `ngrok.com/http2` is the new preferred value.

**Gateway TLS option keys:**

| Legacy                                  | New                                |
| --------------------------------------- | ---------------------------------- |
| `k8s.ngrok.com/terminate-tls.<option>`  | `ngrok.com/terminate-tls.<option>` |

Each legacy-key hit on a user-owned resource emits a Warning event with
reason `LegacyAnnotation` on the affected object so you can find them with:

```sh
kubectl get events -A --field-selector reason=LegacyAnnotation
```

**IngressClass `spec.controller`:** the operator dual-matches both
`k8s.ngrok.com/ingress-controller` and `ngrok.com/ingress-controller` when
the configured `controllerName` is the new default. The helm chart still
renders the IngressClass with the legacy value through 0.24 to avoid a
rollout race; the rendered manifest flips in 0.25.

**External tooling:** if dashboards, monitoring, or GitOps tools select on
the operator-written labels listed below, plan to update those selectors
before the cleanup release.

| Legacy                                                | New                                    |
| ----------------------------------------------------- | -------------------------------------- |
| `k8s.ngrok.com/finalizer`                             | `ngrok.com/finalizer`                  |
| `k8s.ngrok.com/computed-url`                          | `ngrok.com/computed-url`               |
| `k8s.ngrok.com/controller-name`                       | `ngrok.com/controller-name`            |
| `k8s.ngrok.com/controller-namespace`                  | `ngrok.com/controller-namespace`       |
| `bindings.k8s.ngrok.com/endpoint-binding-name`        | `ngrok.com/endpoint-binding-name`      |
| `bindings.k8s.ngrok.com/endpoint-binding-namespace`   | `ngrok.com/endpoint-binding-namespace` |
| `bindings.k8s.ngrok.com/endpoint-url`                 | `ngrok.com/endpoint-url`               |

#### Action required, by release

| Release | Reads | Operator writes | What you do |
| ------- | ----- | --------------- | ----------- |
| 0.24 (this) | Both prefixes | Legacy keys on its own objects; both prefixes on labels / computed-url; legacy finalizer only | Nothing required. Optionally start updating your YAML to the new prefix; both work. |
| 0.25 | Both prefixes | New keys on its own objects; legacy keys removed on next reconcile; new finalizer | Finish updating your YAML and external selectors to the new prefix. |
| 0.26 | New prefix only | New prefix only | Confirm no legacy keys remain in your manifests. The operator no longer reads them. |

#### Supported upgrade path

`previous-stable → 0.24 → 0.25 → 0.26`. Skipping the 0.24 step leaves
in-flight deletions without a rollback-safe intermediate because of how
finalizers interact with the rename — see the developer guide for
specifics.

## What did *not* change in this set of migrations

The CRD API groups (`ingress.k8s.ngrok.com/v1alpha1`,
`ngrok.k8s.ngrok.com/v1alpha1`, `bindings.k8s.ngrok.com/v1alpha1`) are
**unchanged**. A separate 1.0 workstream will consolidate these into
`ngrok.com/v1` with a conversion webhook; that migration will appear here
when it begins.
