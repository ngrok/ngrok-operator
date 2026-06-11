# Migrating to v1

## API Group Changes

Previous releases of the ngrok-operator used three separate API groups at `v1alpha1`:

| Old API Group                    | Resources                                                   |
|----------------------------------|-------------------------------------------------------------|
| `ngrok.k8s.ngrok.com/v1alpha1`   | KubernetesOperator, TrafficPolicy                           |
| `ingress.k8s.ngrok.com/v1alpha1` | Domain, IPPolicy, CloudEndpoint, AgentEndpoint              |
| `bindings.k8s.ngrok.com/v1alpha1`| BoundEndpoint, BindingConfiguration                         |

All resources are consolidated into a single group in v1:

| New API Group    | Version | Resources                                                                             |
|------------------|---------|---------------------------------------------------------------------------------------|
| `ngrok.com`      | `v1`    | AgentEndpoint, CloudEndpoint, KubernetesOperator, TrafficPolicy, Domain, IPPolicy, BoundEndpoint |

## Upgrade Path

A conversion webhook handles in-place conversion from the old group/version combinations to `ngrok.com/v1`. The webhook is installed automatically as part of the operator upgrade.

Steps:

1. Upgrade the operator via Helm. The conversion webhook is registered automatically.
2. Existing `v1alpha1` resources continue to function — the API server converts them transparently on read/write.
3. Migrate your manifests to use `apiVersion: ngrok.com/v1` at your own pace.
4. Once all manifests are updated, the old API group aliases can be removed in a future release.

## Helm Values Changes

The Helm values tree was restructured around the environment-based configuration system described
in [helm/common.md](helm/common.md). Shared platform config lives under `ngrok.*`, feature flags
under `features.*`, and each component (`apiManager`, `agent`, `bindingsForwarder`) owns its
Kubernetes deployment settings plus an app-config map (`<component>.config`) that overrides the
shared config for that component only. Config reaches the binaries as `NGROK_OPERATOR_*`
environment variables injected from per-component ConfigMaps; explicit CLI flags override the
environment.

| Old value | New value |
|-----------|-----------|
| `description`, `region`, `rootCAs`, `serverAddr`, `apiURL`, `clusterDomain` | `ngrok.*` |
| `ngrokMetadata` / `metaData` | `ngrok.metadata` |
| `log.level`, `log.format`, `log.stacktraceLevel` | `ngrok.log.*` |
| `ingress.*` (and deprecated `ingressClass.*`, `watchNamespace`, `controllerName`) | `features.ingress.*` |
| `gateway.*` | `features.gateway.*` |
| `bindings.enabled`, `bindings.endpointSelectors`, `bindings.serviceAnnotations`, `bindings.serviceLabels`, `bindings.ingressEndpoint` | `features.bindings.*` |
| `drainPolicy` | `features.drainPolicy` |
| `defaultDomainReclaimPolicy` | `features.defaultDomainReclaimPolicy` |
| `oneClickDemoMode` | `apiManager.config.oneClickDemoMode` |
| Top-level pod settings (`replicaCount`, `resources`, `nodeSelector`, `tolerations`, `affinity` + presets, `topologySpreadConstraints`, `priorityClassName`, `terminationGracePeriodSeconds`, `lifecycle`, `podDisruptionBudget`, `extraVolumes`, `extraVolumeMounts`, `extraEnv`, `podAnnotations`, `podLabels`, `serviceAccount`) | `apiManager.*` (set equivalents on `agent.*` / `bindingsForwarder.*` as needed) |
| `bindings.forwarder.*` | `bindingsForwarder.*` |

Unchanged: `image.*`, `credentials.*`, `installCRDs`, `cleanupHook.*`, `crdAccessRoles.*`,
`nameOverride`, `fullnameOverride`, `commonLabels`, `commonAnnotations`.

There are no compatibility shims for the old paths — values must be migrated when upgrading.

## Removal Timeline

Support for the `v1alpha1` API groups will be removed in a future minor release after v1. The exact version will be announced in the release notes. After removal, manifests still using the old API groups will stop being accepted by the API server.

> This document will be deleted once the `v1alpha1` API groups are fully removed.
