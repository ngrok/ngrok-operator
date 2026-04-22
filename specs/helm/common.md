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

Contains k8s deployment defaults that are **deep-merged** into each component section. Component values win on conflicts.

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

## Config File System

Each component binary accepts multiple `--config` flags pointing to key=value config files. Helm produces ConfigMaps that are mounted as config files:

| ConfigMap | Source | Mount path |
|-----------|--------|------------|
| `{fullname}-common-config` | `ngrok.*` + `features.*` | `/etc/ngrok/common.conf` |
| `{fullname}-api-manager-config` | `apiManager.config` | `/etc/ngrok/component.conf` |
| `{fullname}-agent-config` | `agent.config` | `/etc/ngrok/component.conf` |
| `{fullname}-bindings-forwarder-config` | `bindingsForwarder.config` | `/etc/ngrok/component.conf` |

Files are loaded in order; later values override earlier ones. Individual CLI flags override config file values.

### Local Development

```bash
go run . api-manager --config=./hack/api-manager.conf
go run . api-manager --config=./hack/api-manager.conf --log-level=debug
```
