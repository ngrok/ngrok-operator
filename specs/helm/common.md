# Helm Chart — Common Configuration

## Chart Structure

The ngrok-operator ships as two Helm charts:

| Chart             | Purpose                          |
|-------------------|----------------------------------|
| `ngrok-operator`  | Operator deployments and RBAC    |
| `ngrok-crds`      | Custom Resource Definitions      |

The CRD chart can be installed automatically via `installCRDs: true` (default) or separately.

## Top-Level Structure

Values fall into three buckets:

- **Pod settings**: `defaults` holds the Kubernetes settings shared by every component. Each component section overrides them and adds settings of its own, such as `resources`.
- **Operator configuration**: `ngrok`, `log` and `features` are set once. Users don't need to know which component reads a setting, and that can change without a values change. Only `log` can be overridden per component.
- **Component-only settings**: `<component>.config` holds settings that exist for one component alone, and its `log` overrides.

```yaml
image:               # Operator image, shared by every component
defaults:            # Shared pod settings (merged into each component)
ngrok:               # Operator configuration: ngrok platform connection
log:                 # Operator configuration: logging
features:            # Operator configuration: feature flags and feature settings
credentials:         # Secret for the access token
apiManager:          # api-manager pod settings; `config:` for its own settings and log overrides
agent:               # agent pod settings; `config:` for its own settings and log overrides
bindingsForwarder:   # bindings-forwarder pod settings; `config:` for its own settings and log overrides
nameOverride: ""
fullnameOverride: ""
commonLabels: {}
commonAnnotations: {}
installCRDs: true
cleanupHook:         # Pre-delete cleanup job
```

## Merge Rule

Where a shared value has a per-component override, the two combine the same way: **maps merge, with the component winning per key; lists replace.**

- Pod settings: `defaults.<key>` merged with `<component>.<key>`.
- Logging: `log` merged with `<component>.config.log`.

## `defaults:`

Pod settings applied to every component. Any of these can also be set as `apiManager.<key>`, `agent.<key>` or `bindingsForwarder.<key>` to override it for that component alone.

| Parameter                                | Description                                      | Default        |
|------------------------------------------|--------------------------------------------------|----------------|
| `defaults.podAnnotations`                | Pod annotations                                  | `{}`           |
| `defaults.podLabels`                     | Pod labels                                       | `{}`           |
| `defaults.nodeSelector`                  | Node labels for pod assignment                   | `{}`           |
| `defaults.tolerations`                   | Pod tolerations                                  | `[]`           |
| `defaults.affinity`                      | Affinity rules; overrides the presets when set   | `{}`           |
| `defaults.podAffinityPreset`             | `soft` or `hard`                                 | `""`           |
| `defaults.podAntiAffinityPreset`         | `soft` or `hard`                                 | `soft`         |
| `defaults.nodeAffinityPreset`            | `type`, `key` and `values` for a node affinity preset | (unset)   |
| `defaults.topologySpreadConstraints`     | Topology spread constraints                      | `[]`           |
| `defaults.priorityClassName`             | Pod priority class                               | `""`           |
| `defaults.extraEnv`                      | Additional environment variables                 | `{}`           |

`defaults` is not named `global` because Helm reserves `global` for values passed down to subcharts.

Settings that rarely apply to every component, such as `resources`, live only in each component's section. See [operator.md](operator.md), [agent.md](agent.md) and [bindings-forwarder.md](bindings-forwarder.md).

## Operator Configuration

`ngrok.*`, `log.*` and `features.*` (see [features.md](features.md)) reach every component. Each component reads the settings it needs and ignores the rest.

`ngrok` and `features` cannot be overridden per component: two components disagreeing on, say, `features.gateway.enabled` is a misconfiguration. Setting `<component>.config.ngrok` or `<component>.config.features` fails the render.

`log` can be overridden for one component:

```yaml
log:
  level: info
agent:
  config:
    log:
      level: debug   # the agent logs at debug; the others at info
```

`log` is the only key `<component>.config` takes; any other key fails the render.

**An empty value means "not set"**: the chart leaves it out and the operator's built-in default applies. The defaults live only in the operator's flag definitions, so the chart cannot disagree with them. A test asserts that every value in `values.yaml` is either empty or equal to the binary's default. Booleans always render, because Helm cannot tell `false` from unset.

The same rule means a component cannot set a log value back to empty: an empty `<component>.config.log` value inherits the shared value.

### `ngrok:`

| Parameter              | Description                                           | Default             |
|------------------------|-------------------------------------------------------|---------------------|
| `ngrok.description`    | Operator description in ngrok dashboard               | `The official ngrok Kubernetes Operator.` |
| `ngrok.region`         | ngrok region                                          | account default     |
| `ngrok.rootCAs`        | CA trust mode: `trusted` or `host`                    | `trusted`           |
| `ngrok.serverAddr`     | Custom ngrok server address                           | ngrok default       |
| `ngrok.apiURL`         | Custom ngrok API URL                                  | ngrok default       |
| `ngrok.metadata`       | Key-value metadata for all ngrok API resources        | `{}`                |
| `ngrok.clusterDomain`  | Kubernetes cluster domain for DNS resolution          | `svc.cluster.local` |

### `log:`

| Parameter              | Description                                                   | Default |
|------------------------|---------------------------------------------------------------|---------|
| `log.level`            | `debug`, `info`, `error`, `panic`, or an integer for more verbose debug levels | `info` |
| `log.format`           | `json` or `console`                                           | `json`  |
| `log.stacktraceLevel`  | `info`, `error` or `panic`                                    | `error` |

## Delivery

Each Deployment carries its component's operator configuration as environment variables in the pod spec: one variable per setting, named in `files/operator-env.yaml` (`features.gateway.enabled` is `NGROK_OPERATOR_FEATURES__GATEWAY__ENABLED`), with the component's log overrides applied. There is no ConfigMap.

Because the values are part of the pod spec, changing one component's configuration rolls only that component, and each ReplicaSet keeps the configuration it was rolled out with.

Adding an operator configuration value needs no template change. See [configuration.md](../configuration.md).

## Upgrading From the Pre-0.25 Layout

Values were moved without compatibility shims. A values file that still sets a key at its old location fails the render with a list of each old key and its new location.

## `credentials:`

| Parameter                  | Description                                    | Default |
|----------------------------|------------------------------------------------|---------|
| `credentials.secret.name`  | Secret name (auto-generated if empty)          | `""`    |
| `credentials.accessToken`  | ngrok access token                             | `""`    |
| `credentials.agent.accessToken` | Access token for the agent only; falls back to `credentials.accessToken` | `""` |
| `credentials.apiManager.accessToken` | Access token for the api-manager only; falls back to `credentials.accessToken` | `""` |

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
