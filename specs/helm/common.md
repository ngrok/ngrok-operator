# Helm Chart — Common Configuration

## Chart Structure

The ngrok-operator ships as two Helm charts:

| Chart             | Purpose                          |
|-------------------|----------------------------------|
| `ngrok-operator`  | Operator deployments and RBAC    |
| `ngrok-crds`      | Custom Resource Definitions      |

The CRD chart can be installed automatically via `installCRDs: true` (default) or separately.

## Top-Level Structure

```yaml
nameOverride: ""
fullnameOverride: ""
commonLabels: {}
commonAnnotations: {}

image: {}                  # shared by all components

defaults: {}               # k8s pod settings, deep-merged into each component

ngrok: {}                  # ngrok platform connection config (shared)
log: {}                    # logging config (shared)
features: {}               # feature flags WITH their feature's config (single source of truth)
credentials: {}            # secret for API key + authtoken

apiManager: {}             # k8s deployment settings + app config for api-manager
agent: {}                  # k8s deployment settings + app config for agent
bindingsForwarder: {}      # k8s deployment settings for bindings-forwarder

crdAccessRoles: {}
installCRDs: true
cleanupHook: {}            # pre-delete cleanup job
```

## `image:`

Shared by every component; there is one operator image.

| Parameter               | Description                                       | Default                |
|--------------------------|----------------------------------------------------|-------------------------|
| `image.registry`        | Docker registry                                   | `docker.io`             |
| `image.repository`      | Image repository                                  | `ngrok/ngrok-operator`  |
| `image.tag`             | Image tag (defaults to chart appVersion)          | `""`                    |
| `image.pullPolicy`      | Image pull policy                                 | `IfNotPresent`          |
| `image.pullSecrets`     | Array of imagePullSecrets                         | `[]`                    |

## `defaults:`

K8s pod settings that are **deep-merged** into each component section (`apiManager`, `agent`, `bindingsForwarder`). A component's own value wins on a key conflict.

**Why `defaults:` and not `global:`.** Helm reserves `global` for values that propagate into every subchart, and this chart depends on `bitnamicharts/common` — which already assigns meaning to `global.imageRegistry`, `global.imagePullSecrets`, `global.security.*` — and on `ngrok-crds`. Naming our own shared pod settings `global` would squat on a key this chart does not own and leak our defaults into those subcharts. `global:` stays free for genuine bitnami passthrough.

| Parameter                                     | Description                                                          | Default  |
|------------------------------------------------|-----------------------------------------------------------------------|----------|
| `defaults.podAnnotations`                      | Pod annotations for every component                                  | `{}`     |
| `defaults.podLabels`                           | Pod labels for every component                                       | `{}`     |
| `defaults.nodeSelector`                        | Node labels for pod assignment                                       | `{}`     |
| `defaults.tolerations`                         | Tolerations for every component's pods                                | `[]`     |
| `defaults.affinity`                            | Affinity rules. Overrides the presets below when set                 | `{}`     |
| `defaults.podAffinityPreset`                   | Pod affinity preset: `""`, `soft`, or `hard`. Ignored if `affinity` is set | `""` |
| `defaults.podAntiAffinityPreset`               | Pod anti-affinity preset: `""`, `soft`, or `hard`. Ignored if `affinity` is set | `soft` |
| `defaults.nodeAffinityPreset.type`             | Node affinity preset type. Ignored if `affinity` is set              | `""`     |
| `defaults.nodeAffinityPreset.key`              | Node label key to match. Ignored if `affinity` is set                | `""`     |
| `defaults.nodeAffinityPreset.values`           | Node label values to match. Ignored if `affinity` is set             | `[]`     |
| `defaults.topologySpreadConstraints`           | Topology spread constraints                                          | `[]`     |
| `defaults.priorityClassName`                   | Priority class for pod scheduling                                    | `""`     |
| `defaults.resources`                           | Container resource requests/limits                                   | `{}`     |
| `defaults.extraVolumes`                        | Additional volumes                                                   | `[]`     |
| `defaults.extraVolumeMounts`                   | Additional volume mounts                                             | `[]`     |
| `defaults.extraEnv`                            | Additional environment variables                                     | `{}`     |
| `defaults.lifecycle`                           | Container lifecycle hooks                                            | `{}`     |
| `defaults.terminationGracePeriodSeconds`       | Graceful shutdown period                                              | `30`     |

### Override Semantics

Maps deep-merge: `apiManager.podAnnotations` (etc.) is merged on top of `defaults.podAnnotations`, with the component's own keys winning on conflicts. Arrays replace wholesale: if a component sets `apiManager.tolerations`, `defaults.tolerations` is ignored entirely for that component, not concatenated.

Pod anti-affinity still defaults to `soft` — behavior here is unchanged from before this refactor.

Every one of the settings above can also be set per component (e.g. `agent.resources`), which is how a value that only applied to api-manager before this refactor (see [migration-v1.md](../migration-v1.md)) is preserved for just that component today.

## `ngrok:`

Platform connection config shared by all components. Rendered into the operator's ConfigMap (see [Config File System](#config-file-system) below).

Every key is present in `values.yaml` with a `null` value, which means *not set — use the binary's default*. Defaults live in the operator binary (`internal/config.Default`); setting a key here is what turns it into an override. See [configuration.md](../configuration.md#representing-unset) for why `null` rather than a commented-out key.

| Parameter               | Description                                            | Built-in default |
|--------------------------|----------------------------------------------------------|--------------------|
| `ngrok.description`     | Operator description in ngrok dashboard                 | `The official ngrok Kubernetes Operator.` |
| `ngrok.region`          | ngrok region (empty = closest region)                   | `""`               |
| `ngrok.rootCAs`         | CA trust mode: `"trusted"` or `"host"`                  | `trusted`          |
| `ngrok.serverAddr`      | Custom ngrok server address                             | `""`               |
| `ngrok.apiURL`          | Custom ngrok API URL                                    | `""`               |
| `ngrok.metadata`        | Key-value metadata for all ngrok API resources          | `{}`               |
| `ngrok.clusterDomain`   | Kubernetes cluster domain for DNS resolution            | `svc.cluster.local` |

These are deliberately not overridable per component: a flag inventory of `main` showed `region` and `serverAddr` read identically by both api-manager and agent, and `rootCAs` read only by agent. Divergent values across components would almost certainly be a misconfiguration, not a use case. Adding a per-component override later is easy; removing one that was promised is not.

## `log:`

Logging config, shared by all components. Also `null` by default in `values.yaml`, and overridable per component (`apiManager.log.level` and friends).

| Parameter                  | Description                                              | Built-in default |
|------------------------------|-------------------------------------------------------------|--------------------|
| `log.level`                | Log level: `debug`, `info`, `warn`, or `error`             | `info`             |
| `log.format`               | Log format: `console` or `json`                            | `json`             |
| `log.stacktraceLevel`      | Level at which to emit stacktraces: `info` or `error`      | `error`            |

`zap.Options.BindFlags` and the `--zap-*` flags it registered are gone. Each component's `zap.Options` is built from this config, exposed as `--log-level`, `--log-format`, and `--log-stacktrace-level`.

## `credentials:`

| Parameter                  | Description                                    | Default |
|----------------------------|--------------------------------------------------|---------|
| `credentials.secret.name`  | Secret name (auto-generated if empty)          | `""`    |
| `credentials.apiKey`       | ngrok API key                                  | `""`    |
| `credentials.authtoken`    | ngrok auth token                               | `""`    |

See [authentication.md](../authentication.md) for details on credential management.

## Other Top-Level Parameters

| Parameter            | Description                                 | Default |
|-----------------------|-----------------------------------------------|---------|
| `nameOverride`        | Partially override generated resource names | `""`    |
| `fullnameOverride`    | Fully override generated resource names     | `""`    |
| `commonLabels`        | Labels added to all resources this chart creates | `{}` |
| `commonAnnotations`   | Annotations added to all resources this chart creates | `{}` |
| `installCRDs`         | Install CRDs alongside the operator         | `true`  |

## Config Delivery

One ConfigMap, `{fullname}-config`, with one key per component — `apiManager.yaml`, `agent.yaml`, `bindingsForwarder.yaml`. Each key holds that component's fully resolved app config; the chart performs the shared → component merge at render time. Every component mounts the ConfigMap read-only at `/etc/ngrok-operator` and is started with `--config=/etc/ngrok-operator/<component>.yaml`.

```yaml
# ConfigMap {fullname}-config
data:
  agent.yaml: |
    log:
      level: debug        # agent.log.level won over the shared log.level
    ngrok:
      region: eu          # inherited from the shared ngrok.region
```

Only values a user actually set are rendered. Anything absent falls back to the operator binary's built-in default (`internal/config.Default`), so a value never has two defaults to keep in sync.

The Go side does no merging. `internal/config.Load` decodes the file onto the defaults and stops there — it has no concept of a component, no section to select, and no overlay step.

**Precedence, highest wins:**

1. An explicitly passed CLI flag (e.g. `--region`).
2. An `NGROK_OPERATOR_*` environment variable.
3. The config file(s) passed via `--config`; later files override earlier ones.
4. The built-in Go default.

The environment layer is generic: after flags are parsed, any flag the user did not explicitly pass takes its value from `NGROK_OPERATOR_` + its name uppercased with `-` replaced by `_`. Nothing is wired per flag, so a new flag gets env support and correct precedence for free. The chart does not use this layer — it exists for overriding a setting locally or on a running pod.

Secrets are not part of this chain. `NGROK_API_KEY` and `NGROK_AUTHTOKEN` are read only from the environment, sourced from the credentials Secret, and are never accepted from a config file or a flag.

**Checksum scoping:** each Deployment's `checksum/config` annotation is computed from its own key in the ConfigMap, so editing the agent's config rolls the agent Deployment and nothing else.

### Local Development

```bash
NGROK_API_KEY=... POD_NAMESPACE=ngrok-operator go run . api-manager
```

No config file is required. Credentials and `POD_NAMESPACE` are the only things a component cannot default: the first must not travel through config, and the second names the namespace the operator manages its own resources in, where guessing wrong is worse than refusing to start. `--config=./hack/config.yaml` is available for local conveniences (console logging and the like), and checked-in config files replace the long `--set` chains in the `make deploy` targets and chainsaw fixtures.

See [configuration.md](../configuration.md) for the design rationale behind all of the above.
