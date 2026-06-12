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
global:              # Shared k8s deployment defaults (deep-merged into each component)
ngrok:               # ngrok platform connection config (shared by all components)
credentials:         # Secret for API key + authtoken
features:            # Feature flags and feature-specific config (single source of truth)
apiManager:          # K8s deployment settings + app config for api-manager
agent:               # K8s deployment settings + app config for agent
bindingsForwarder:   # K8s deployment settings for bindings-forwarder
nameOverride: ""
fullnameOverride: ""
installCRDs: true
cleanupHook:         # Pre-delete cleanup job
```

## `global:`

Contains k8s deployment defaults that are **deep-merged** into each component section. Component values win on conflicts. Any setting that applies to all three components (`apiManager`, `agent`, `bindingsForwarder`) MUST be present here.

| Parameter                                | Description                                      | Default        |
|------------------------------------------|--------------------------------------------------|----------------|
| `global.image.registry`                  | Docker registry                                  | `docker.io`    |
| `global.image.repository`               | Image repository                                 | `ngrok/ngrok-operator` |
| `global.image.tag`                       | Image tag (defaults to chart appVersion)         | `""`           |
| `global.image.pullPolicy`               | Image pull policy                                | `IfNotPresent` |
| `global.image.pullSecrets`              | Array of imagePullSecrets                        | `[]`           |
| `global.commonLabels`                    | Labels applied to all k8s resources              | `{}`           |
| `global.commonAnnotations`              | Annotations applied to all k8s resources         | `{}`           |
| `global.podAnnotations`                 | Pod annotations (merged into each component)     | `{}`           |
| `global.podLabels`                       | Pod labels (merged into each component)          | `{}`           |
| `global.nodeSelector`                    | Node labels for pod assignment                   | `{}`           |
| `global.tolerations`                     | Pod tolerations                                  | `[]`           |
| `global.affinity`                        | Affinity rules                                   | `{}`           |
| `global.topologySpreadConstraints`      | Topology spread constraints                      | `[]`           |
| `global.priorityClassName`              | Pod priority class                               | `""`           |
| `global.resources`                       | Container resource requests/limits               | `{}`           |
| `global.extraVolumes`                    | Additional volumes                               | `[]`           |
| `global.extraVolumeMounts`              | Additional volume mounts                         | `[]`           |
| `global.extraEnv`                        | Additional environment variables                 | `{}`           |
| `global.lifecycle`                       | Container lifecycle hooks                        | `{}`           |
| `global.terminationGracePeriodSeconds`  | Graceful shutdown time                           | `30`           |
| `global.podDisruptionBudget.create`     | Enable PDB for all components                    | `false`        |
| `global.podDisruptionBudget.maxUnavailable` | Max unavailable pods (global default)        | `"1"`          |
| `global.podDisruptionBudget.minAvailable` | Min available pods (global default)            | (unset)        |

### Override Semantics

Maps (`podAnnotations`, `nodeSelector`, `extraEnv`, etc.) use **deep merge** — component values are merged on top of global, with the component winning on key conflicts. Arrays (`tolerations`, `topologySpreadConstraints`, `extraVolumes`, etc.) use **replace** — if the component sets them, the global value is ignored entirely.

## `ngrok:`

Platform connection config shared by all components. Rendered into a common ConfigMap.

| Parameter              | Description                                           | Default             |
|------------------------|-------------------------------------------------------|---------------------|
| `ngrok.description`    | Operator description in ngrok dashboard               | `"The official ngrok Kubernetes Operator."` |
| `ngrok.region`         | ngrok region (empty = closest region)                 | `""`                |
| `ngrok.rootCAs`        | CA trust mode: `"trusted"` or `"host"`                | `""`                |
| `ngrok.serverAddr`     | Custom ngrok server address                           | `""`                |
| `ngrok.apiURL`         | Custom ngrok API URL                                  | `""`                |
| `ngrok.metadata`       | Key-value metadata for all ngrok API resources        | `{}`                |
| `ngrok.clusterDomain`  | Kubernetes cluster domain for DNS resolution          | `svc.cluster.local` |
| `ngrok.log.level`      | Log level: `debug`, `info`, `error`                   | `info`              |
| `ngrok.log.format`     | Log format: `console`, `json`                         | `json`              |
| `ngrok.log.stacktraceLevel` | Stacktrace level: `info`, `error`                | `error`             |

## `credentials:`

| Parameter                  | Description                                    | Default |
|----------------------------|------------------------------------------------|---------|
| `credentials.secret.name`  | Secret name (auto-generated if empty)          | `""`    |
| `credentials.apiKey`       | ngrok API key                                  | `""`    |
| `credentials.authtoken`    | ngrok auth token                               | `""`    |

See [authentication.md](../authentication.md) for details on credential management.

## Other Top-Level Parameters

| Parameter          | Description                              | Default |
|--------------------|------------------------------------------|---------|
| `nameOverride`     | Partially override generated resource names | `""`  |
| `fullnameOverride` | Fully override generated resource names  | `""`    |
| `installCRDs`      | Install CRDs alongside the operator      | `true`  |

## Configuration System

App config follows the Argo CD model: every app-config CLI flag can be set via
an `NGROK_OPERATOR_*` environment variable, and the Helm chart delivers config
to the pods as environment variables injected from per-component ConfigMaps.

Precedence (highest wins):

1. Explicit CLI flag
2. `NGROK_OPERATOR_*` environment variable
3. Built-in default

Helm renders one ConfigMap per component. Each ConfigMap holds the shared
config (`ngrok.*` + `features.*`) merged with that component's `config` map
(component keys win), one dotted key per entry (e.g. `log.level`,
`features.gateway.enabled`):

| ConfigMap | Source |
|-----------|--------|
| `{fullname}-api-manager-config` | `ngrok.*` + `features.*` merged with `apiManager.config` |
| `{fullname}-agent-config` | `ngrok.*` + `features.*` merged with `agent.config` |
| `{fullname}-bindings-forwarder-config` | `ngrok.*` + `features.*` merged with `bindingsForwarder.config` |

Each deployment injects the keys it consumes as environment variables via
`valueFrom.configMapKeyRef` with `optional: true`, so absent keys fall back to
the binary's built-in defaults (e.g. ConfigMap key `log.level` becomes
`NGROK_OPERATOR_LOG_LEVEL`). A `checksum/config` pod annotation rolls pods
when the rendered config changes. Environment variables that do not map to a
flag on the running component are ignored, so all components can share the
same variable namespace.

Per-component overrides: set keys under `<component>.config` (e.g.
`agent.config."log.level": debug` changes the log level for only the agent).
Empty environment variables are treated as unset, and values that fail to
parse (e.g. a malformed boolean) log a warning and fall back to the built-in
default.

### Local Development

```bash
NGROK_OPERATOR_LOG_LEVEL=debug NGROK_OPERATOR_LOG_FORMAT=console go run . api-manager
go run . api-manager --zap-log-level=debug   # CLI flags always win
```
