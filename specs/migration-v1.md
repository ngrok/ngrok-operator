# Migrating to v1

## API Group Changes

Previous releases of the ngrok-operator used three separate API groups at `v1alpha1`:

| Old API Group                    | Resources                                                   |
|----------------------------------|-------------------------------------------------------------|
| `ngrok.k8s.ngrok.com/v1alpha1`   | KubernetesOperator, NgrokTrafficPolicy                      |
| `ingress.k8s.ngrok.com/v1alpha1` | Domain, IPPolicy, CloudEndpoint, AgentEndpoint              |
| `bindings.k8s.ngrok.com/v1alpha1`| BoundEndpoint, BindingConfiguration                         |

All resources are consolidated into a single group in v1:

| New API Group    | Version | Resources                                                                             |
|------------------|---------|---------------------------------------------------------------------------------------|
| `ngrok.com`      | `v1`    | AgentEndpoint, CloudEndpoint, KubernetesOperator, TrafficPolicy, Domain, IPPolicy, BoundEndpoint |

`NgrokTrafficPolicy` is also **renamed** to `TrafficPolicy` as part of the move.
The `Ngrok` prefix was redundant once the group is `ngrok.com`. The spec is
unchanged, so migrating an object is a manifest re-stamp — see
[Upgrade Path](#upgrade-path).

## Status Field Changes

### KubernetesOperator

The KubernetesOperator status was restructured around standard status conditions
ahead of 1.0. Tooling reading the old fields must switch to the replacements:

| Removed field              | Replacement                                                                 |
|----------------------------|------------------------------------------------------------------------------|
| `registrationStatus`       | `Registered` condition (`True`/`False`; reason `Pending` before registration) |
| `registrationErrorCode`    | `Registered` condition message for registration failures (the reason is the stable token `RegistrationFailed`) |
| `errorMessage`             | `Ready` condition message                                                    |
| `drainStatus`              | `Draining` condition (`True` = in progress/retrying, `False` + reason `DrainCompleted` = done) |
| `drainMessage`             | `Draining` condition message                                                 |
| `drainProgress` (`"X/Y"`)  | `drain.drainedResources` (successes only) / `drain.failedResources` / `drain.totalResources` integers |
| `drainErrors`              | `drain.errors`                                                               |

`Registered.message` describes registration failures. Failures after a remote
registration already exists leave `Registered=True` and are reported through
`Ready=False` and `Ready.message`.

`enabledFeatures` changed from a comma-separated string to a `[]string`. The
v1alpha1 Go type contains a temporary, read-only compatibility decoder that
accepts both representations during an in-place upgrade. Current versions
always write an array, so the next status update passively normalizes the
stored value. The decoder can be removed once upgrades from versions that
wrote the string representation are outside the supported upgrade window.

## Upgrade Path

The migration is **not** served by a conversion webhook. A CRD conversion
webhook is registered on a single CRD — identified by `plural.group` — and
converts between the versions declared on *that* CRD. It cannot cross group
boundaries, and it cannot fold two different kinds onto the same storage.
`ngroktrafficpolicies.ngrok.k8s.ngrok.com` and `trafficpolicies.ngrok.com` are
two separate CRDs with separate storage, so there is nothing for a webhook to
convert between.

Instead the operator ships **both CRDs** during the migration window and
dual-reads them: every lookup consults the canonical `ngrok.com/v1` object
first and falls back to the deprecated `ngrok.k8s.ngrok.com/v1alpha1` object
only when no canonical object of that name exists in the namespace. When the
fallback answers a lookup, the operator emits a `DeprecatedAPIGroup` warning
event on the referring endpoint. Objects are never copied between the two
kinds — that would create duplicate storage rows with diverging status — so
you re-stamp manifests on your own schedule.

Steps:

1. Upgrade the operator via Helm. Both CRDs are installed; existing
   `v1alpha1` objects keep working and keep being resolved. 
   If installCRDS=false, upgrade ngrok-crds directly before migrating objects.
2. Re-stamp each manifest's `apiVersion` and `kind`, apply it, then delete the
   old object. For `NgrokTrafficPolicy`:

   ```sh
   # Discover
   kubectl get ngroktrafficpolicies -A

   # Migrate one object
   kubectl get ngroktrafficpolicy foo -n bar -o json \
     | jq '.kind = "TrafficPolicy"
           | .apiVersion = "ngrok.com/v1"
           | del(.metadata.resourceVersion, .metadata.uid,
                 .metadata.creationTimestamp, .metadata.generation, .status)' \
     | kubectl apply -f -

   kubectl delete ngroktrafficpolicy foo -n bar
   ```

   References from AgentEndpoint, CloudEndpoint, Ingress, and Gateway API
   routes are resolved by name and namespace, so they keep working across the
   re-stamp without any edit of their own.
3. If both a canonical and a deprecated object exist under the same name, the
   canonical one wins silently — that is the expected transitional state
   between the `apply` and the `delete` above.
4. Once all manifests are updated, the deprecated CRDs are removed in the
   cleanup release. Any objects still stored under a deprecated CRD at that
   point are removed by the API server along with it.

## Breaking Field Removals

| Resource      | Removed field       | Replacement         | Notes |
|---------------|---------------------|---------------------|-------|
| BoundEndpoint | `spec.endpointURI`  | `spec.endpointURL`  | `endpointURL` is now required. `endpointURI` was deprecated in favor of `endpointURL` (same format) and the dual-read fallback has been removed. BoundEndpoints are created by the operator's poller, which has written `endpointURL` since the rename, so user action is only needed for manually created resources. |
| Domain        | `status.region`, `spec.region` | — | Region selection for reserved domains is deprecated in the ngrok API; both fields are removed. |
| Domain        | `Progressing` condition | —               | Was only set to True in a rare certificate-provisioning error path and never reset to False. Use the `Ready`, `CertificateReady`, and `DNSConfigured` conditions instead. |
| IPPolicy      | `status.rules`      | `spec.rules`        | Never populated by the controller; rule state is reflected by the `IPPolicyRulesConfigured` condition. |

## Changed Condition Types

| Resource | Old condition type | New condition type        | Notes |
|----------|--------------------|---------------------------|-------|
| IPPolicy | `RulesConfigured`  | `IPPolicyRulesConfigured` | Renamed for consistency with the `IPPolicyCreated` condition. |

## Removal Timeline

Support for the `v1alpha1` API groups will be removed in a future minor release after v1. The exact version will be announced in the release notes. After removal, manifests still using the old API groups will stop being accepted by the API server.

> This document will be deleted once the `v1alpha1` API groups are fully removed.
