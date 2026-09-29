# Helm Chart — Common Configuration

## Chart Structure

The ngrok-operator ships as two Helm charts:

| Chart             | Purpose                          |
|-------------------|----------------------------------|
| `ngrok-operator`  | Operator deployments and RBAC    |
| `ngrok-crds`      | Custom Resource Definitions      |

The CRD chart can be installed automatically via `installCRDs: true` (default) or separately.

## Top-Level Structure

Values fall into three groups:

- **Chart**: the usual Helm chart settings (`nameOverride`, `fullnameOverride`, `commonLabels`, `commonAnnotations`, `image`) and what the chart installs (`installCRDs`, `crdAccessRoles`).
- **`ngrok`**: the operator's configuration: credentials, connection, `clusterDomain`, `log` and `features`. Set once for every component; users don't need to know which component reads a setting, and that can change without a values change. Only `log` can be overridden per component.
- **`components`**: Kubernetes settings for each pod the chart runs. `components.common` holds the ones shared by the api-manager, agent and bindings-forwarder; each component overrides them and adds its own, such as `resources`.

Every feature is an object, so it can gain options without a rename. A feature that can be turned off has `enabled`.

```yaml
nameOverride: ""
fullnameOverride: ""
commonLabels: {}
commonAnnotations: {}
image:               # Operator image, shared by every component
installCRDs: true
crdAccessRoles:      # Editor/viewer ClusterRoles for the CRDs

ngrok:
  credentials:       # Secret for the access token
  description: ""    # ...and the other connection settings
  clusterDomain: ""
  log:
  features:          # Features and their settings

components:
  common:            # Pod settings shared by every component
  apiManager:        # api-manager pod settings; `log:` for its log overrides
  agent:             # agent pod settings; `log:` for its log overrides
  bindingsForwarder: # bindings-forwarder pod settings; `log:` for its log overrides
  cleanupHook:       # Pod settings of the pre-delete hook (see ngrok.features.cleanup)
```

The environment variable of a setting is its path under `ngrok`: `ngrok.features.gateway.enabled` is `NGROK_OPERATOR_FEATURES__GATEWAY__ENABLED`. See [configuration.md](../configuration.md#names).

## Merge Rule

Pod settings: `components.<component>.<key>` overrides `components.common.<key>`.

- A key the component leaves unset inherits the common value.
- A non-empty map merges with the common one, the component winning per key.
- Any other value replaces it, including an empty `{}`, `[]` or `""`: `components.agent.tolerations: []` drops the common tolerations for the agent alone.
- Helm removes null values before the chart sees them, so setting one key of a map to null clears the whole map for that component. To drop one common key, clear the map and re-list the keys to keep.

`values.yaml` lists each component's overrides commented out, so they are visible without being set: a key set there would always override the common value. `TestChartComponentOverrides` checks every common key is listed for every component. The cleanup hook does not take `components.common`.

Logging: `ngrok.log` merged with `components.<component>.log`, per key. An empty component log value inherits the shared one.

## `components.common:`

Pod settings applied to the api-manager, agent and bindings-forwarder. Any of these can also be set as `components.apiManager.<key>`, `components.agent.<key>` or `components.bindingsForwarder.<key>` to override it for that component alone.

| Parameter                                         | Description                                      | Default        |
|---------------------------------------------------|--------------------------------------------------|----------------|
| `components.common.podAnnotations`                | Pod annotations                                  | `{}`           |
| `components.common.podLabels`                     | Pod labels                                       | `{}`           |
| `components.common.nodeSelector`                  | Node labels for pod assignment                   | `{}`           |
| `components.common.tolerations`                   | Pod tolerations                                  | `[]`           |
| `components.common.affinity`                      | Affinity rules; overrides the presets when set   | `{}`           |
| `components.common.podAffinityPreset`             | `soft` or `hard`                                 | `""`           |
| `components.common.podAntiAffinityPreset`         | `soft` or `hard`                                 | `soft`         |
| `components.common.nodeAffinityPreset`            | `type`, `key` and `values` for a node affinity preset | (unset)   |
| `components.common.topologySpreadConstraints`     | Topology spread constraints                      | `[]`           |
| `components.common.priorityClassName`             | Pod priority class                               | `""`           |
| `components.common.extraEnv`                      | Additional environment variables                 | `{}`           |
| `components.common.terminationGracePeriodSeconds` | Graceful shutdown period, in seconds          | (Kubernetes: `30`) |
| `components.common.updateStrategy`                | Deployment update strategy                       | (Kubernetes: `RollingUpdate`) |

Helm reserves `global` for values passed down to subcharts, so the shared section is not named that.

Settings that rarely apply to every component, such as `resources`, live only in each component's section. See [operator.md](operator.md), [agent.md](agent.md) and [bindings-forwarder.md](bindings-forwarder.md).

## Operator Configuration

Everything under `ngrok` (see [features.md](features.md) for `ngrok.features`) reaches every component. Each component reads the settings it needs and ignores the rest.

Only `log` can be overridden for one component: two components disagreeing on, say, `ngrok.features.gateway.enabled` is a misconfiguration, so setting `components.<component>.ngrok` or `components.<component>.features` fails the render.

```yaml
ngrok:
  log:
    level: info
components:
  agent:
    log:
      level: debug   # the agent logs at debug; the others at info
```

**An empty value means "not set"**: the chart leaves it out and the operator's built-in default applies. The defaults live only in the operator's flag definitions, so the chart cannot disagree with them. A test asserts that every value in `values.yaml` is either empty or equal to the binary's default. Booleans always render, because Helm cannot tell `false` from unset.

The same rule means a component cannot set a log value back to empty: an empty `components.<component>.log` value inherits the shared value.

### `ngrok:`

| Parameter              | Description                                           | Default             |
|------------------------|-------------------------------------------------------|---------------------|
| `ngrok.description`    | Operator description in ngrok dashboard               | `The official ngrok Kubernetes Operator.` |
| `ngrok.region`         | ngrok region                                          | account default     |
| `ngrok.rootCAs`        | CA trust mode: `trusted` or `host`                    | `trusted`           |
| `ngrok.serverAddr`     | Custom ngrok server address                           | ngrok default       |
| `ngrok.apiURL`         | Custom ngrok API URL                                  | ngrok default       |
| `ngrok.metadata`       | Key-value metadata for all ngrok API resources        | `{}`                |

`ngrok.clusterDomain` is the Kubernetes cluster domain, used to build in-cluster service addresses by the Ingress, Gateway API, Service and bindings controllers. Default: `svc.cluster.local`.

### `ngrok.log:`

| Parameter              | Description                                                   | Default |
|------------------------|---------------------------------------------------------------|---------|
| `ngrok.log.level`            | `debug`, `info`, `error`, `panic`, or an integer for more verbose debug levels | `info` |
| `ngrok.log.format`           | `json` or `console`                                           | `json`  |
| `ngrok.log.stacktraceLevel`  | `info`, `error` or `panic`                                    | `error` |

## Delivery

Each Deployment carries its component's operator configuration as environment variables in the pod spec: one variable per setting, named in `files/operator-env.yaml` (`ngrok.features.gateway.enabled` is `NGROK_OPERATOR_FEATURES__GATEWAY__ENABLED`), with the component's log overrides applied. There is no ConfigMap.

Because the values are part of the pod spec, changing one component's configuration rolls only that component, and each ReplicaSet keeps the configuration it was rolled out with.

Adding an operator configuration value needs no template change. See [configuration.md](../configuration.md).

## Upgrading From the Pre-0.25 Layout

Values were moved without compatibility shims. A values file that still sets a key at its old location fails the render with a list of each old key and its new location.

## `credentials:`

| Parameter                  | Description                                    | Default |
|----------------------------|------------------------------------------------|---------|
| `ngrok.credentials.secret.name`  | Secret name (auto-generated if empty)          | `""`    |
| `ngrok.credentials.accessToken`  | ngrok access token                             | `""`    |
| `ngrok.credentials.agent.accessToken` | Access token for the agent only; falls back to `ngrok.credentials.accessToken` | `""` |
| `ngrok.credentials.apiManager.accessToken` | Access token for the api-manager only; falls back to `ngrok.credentials.accessToken` | `""` |

See [authentication.md](../authentication.md) for details on credential management.

## Other Top-Level Parameters

| Parameter           | Description                                 | Default |
|---------------------|---------------------------------------------|---------|
| `nameOverride`      | Partially override generated resource names | `""`    |
| `fullnameOverride`  | Fully override generated resource names     | `""`    |
| `commonLabels`      | Labels applied to all resources             | `{}`    |
| `commonAnnotations` | Annotations applied to all resources        | `{}`    |
| `image.*`           | Operator image registry, repository, tag, pull policy and pull secrets | see chart README |
| `installCRDs`       | Install CRDs alongside the operator         | `true`  |
