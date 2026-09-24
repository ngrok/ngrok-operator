# ngrok Kubernetes Operator

This is the Helm chart to install official the ngrok Kubernetes Operator

# Usage

## Prerequisites

The cluster Must be setup with a secret named `ngrok-operator-credentials` with the following keys:
* AUTHTOKEN
* API\_KEY

## Installation

[Helm](https://helm.sh) must be installed to use the charts.
Please refer to Helm's [documentation](https://helm.sh/docs) to get started.

Once Helm has been set up correctly, add the repo as follows:

`helm repo add ngrok https://charts.ngrok.com`

If you had already added this repo earlier, run `helm repo update` to retrieve the latest versions of the packages.
You can then run `helm search repo ngrok` to see the charts.

To install the ngrok-operator chart:

`helm install ngrok-operator ngrok/ngrok-operator`

To uninstall the chart:

`helm delete my-ngrok-operator`

## Multi-Instance Installations

To run multiple ngrok-operator instances in the same cluster (e.g., in different namespaces), install the CRDs once and disable CRD installation for each operator instance:

1. Install CRDs once per cluster:

   ```bash
   helm install ngrok-crds ngrok/ngrok-crds
   ```

2. Install operator instances with `installCRDs=false`:

   ```bash
   helm install ngrok-operator-ns1 ngrok/ngrok-operator \
     --namespace ns1 \
     --set installCRDs=false

   helm install ngrok-operator-ns2 ngrok/ngrok-operator \
     --namespace ns2 \
     --set installCRDs=false
   ```

> **Important:** Do not set `installCRDs=true` if CRDs are already managed by another Helm release. Helm will fail with ownership conflicts if multiple releases attempt to manage the same CRDs.


<!-- Parameters are auto generated via @bitnami/readme-generator-for-helm -->
## Parameters

### Common parameters

| Name                | Description                                                                       | Value                  |
| ------------------- | --------------------------------------------------------------------------------- | ---------------------- |
| `nameOverride`      | Partially override the generated resource names                                   | `""`                   |
| `fullnameOverride`  | Fully override the generated resource names                                       | `""`                   |
| `commonLabels`      | Labels added to all resources created by this chart                               | `{}`                   |
| `commonAnnotations` | Annotations added to all resources created by this chart                          | `{}`                   |
| `image.registry`    | The ngrok operator image registry.                                                | `docker.io`            |
| `image.repository`  | The ngrok operator image repository.                                              | `ngrok/ngrok-operator` |
| `image.tag`         | The ngrok operator image tag. Defaults to the chart's appVersion if not specified | `""`                   |
| `image.pullPolicy`  | The ngrok operator image pull policy.                                             | `IfNotPresent`         |
| `image.pullSecrets` | An array of imagePullSecrets to be used when pulling the image.                   | `[]`                   |

### Shared component defaults

| Name                                     | Description                                                                                          | Value  |
| ---------------------------------------- | ---------------------------------------------------------------------------------------------------- | ------ |
| `defaults.podAnnotations`                | Pod annotations for every component                                                                  | `{}`   |
| `defaults.podLabels`                     | Pod labels for every component                                                                       | `{}`   |
| `defaults.nodeSelector`                  | Node labels for pod assignment                                                                       | `{}`   |
| `defaults.tolerations`                   | Tolerations for every component's pods                                                               | `[]`   |
| `defaults.affinity`                      | Affinity rules for every component's pods. Overrides the presets below when set.                     | `{}`   |
| `defaults.podAffinityPreset`             | Pod affinity preset. Ignored if `affinity` is set. Allowed values: "" (none), "soft" or "hard"       | `""`   |
| `defaults.podAntiAffinityPreset`         | Pod anti-affinity preset. Ignored if `affinity` is set. Allowed values: "" (none), "soft" or "hard"  | `soft` |
| `defaults.nodeAffinityPreset.type`       | Node affinity preset type. Ignored if `affinity` is set. Allowed values: "" (none), "soft" or "hard" | `""`   |
| `defaults.nodeAffinityPreset.key`        | Node label key to match. Ignored if `affinity` is set.                                               | `""`   |
| `defaults.nodeAffinityPreset.values`     | Node label values to match. Ignored if `affinity` is set.                                            | `[]`   |
| `defaults.topologySpreadConstraints`     | Topology spread constraints                                                                          | `[]`   |
| `defaults.priorityClassName`             | Priority class for pod scheduling                                                                    | `""`   |
| `defaults.resources.limits`              | Resource limits for every component                                                                  | `{}`   |
| `defaults.resources.requests`            | Resource requests for every component                                                                | `{}`   |
| `defaults.extraVolumes`                  | Additional volumes                                                                                   | `[]`   |
| `defaults.extraVolumeMounts`             | Additional volume mounts                                                                             | `[]`   |
| `defaults.extraEnv`                      | Additional environment variables                                                                     | `{}`   |
| `defaults.lifecycle`                     | Container lifecycle hooks                                                                            | `{}`   |
| `defaults.terminationGracePeriodSeconds` | Graceful shutdown period                                                                             | `30`   |

### ngrok platform configuration

| Name                  | Description                                                                                                              | Value |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------ | ----- |
| `ngrok.description`   | Description attached to the ngrok API resources the operator creates. Default: `The official ngrok Kubernetes Operator.` | `nil` |
| `ngrok.region`        | ngrok region to use. Unset means the account default                                                                     | `nil` |
| `ngrok.serverAddr`    | Address of the ngrok server to use for tunnels. Unset means the ngrok default                                            | `nil` |
| `ngrok.apiURL`        | Base URL for the ngrok API. Unset means the ngrok default                                                                | `nil` |
| `ngrok.rootCAs`       | Root CAs to use: `trusted` for the ngrok CA, `host` for the host's CA bundle. Default: `trusted`                         | `nil` |
| `ngrok.clusterDomain` | Cluster domain used when resolving in-cluster service addresses. Default: `svc.cluster.local`                            | `nil` |
| `ngrok.metadata`      | Key/value pairs added as metadata to the ngrok API resources the operator creates                                        | `{}`  |

### Logging configuration

| Name                  | Description                                                                                                          | Value |
| --------------------- | -------------------------------------------------------------------------------------------------------------------- | ----- |
| `log.level`           | Log level: `debug`, `info`, `warn`, `error`, `dpanic`, `panic` or `fatal`. Default: `info`                           | `nil` |
| `log.format`          | Log format: `json` or `console`. Default: `json`                                                                     | `nil` |
| `log.stacktraceLevel` | Level at which to emit stacktraces: `debug`, `info`, `warn`, `error`, `dpanic`, `panic` or `fatal`. Default: `error` | `nil` |

### Feature configuration

| Name                                      | Description                                                                                          | Value                              |
| ----------------------------------------- | ---------------------------------------------------------------------------------------------------- | ---------------------------------- |
| `features.ingress.enabled`                | Enable the Kubernetes Ingress controller                                                             | `true`                             |
| `features.ingress.controllerName`         | Controller name for IngressClass matching. Must match the operator's own `--ingress-controller-name` | `k8s.ngrok.com/ingress-controller` |
| `features.ingress.watchNamespace`         | Namespace to watch for Ingress resources. Unset watches all namespaces                               | `nil`                              |
| `features.ingress.ingressClass.name`      | IngressClass resource name                                                                           | `ngrok`                            |
| `features.ingress.ingressClass.create`    | Create the IngressClass resource                                                                     | `true`                             |
| `features.ingress.ingressClass.default`   | Set as the default IngressClass                                                                      | `false`                            |
| `features.gateway.enabled`                | Enable Gateway API support, if the CRDs are detected                                                 | `true`                             |
| `features.gateway.disableReferenceGrants` | Disable the ReferenceGrant requirement for cross-namespace references                                | `false`                            |
| `features.bindings.enabled`               | Enable the Endpoint Bindings feature                                                                 | `false`                            |
| `features.bindings.endpointSelectors`     | CEL expressions filtering which endpoints to project. Default: `["true"]`                            | `nil`                              |
| `features.bindings.serviceAnnotations`    | Annotations applied to projected services                                                            | `{}`                               |
| `features.bindings.serviceLabels`         | Labels applied to projected services                                                                 | `{}`                               |
| `features.bindings.ingressEndpoint`       | Hostname of the bindings ingress endpoint. Default: `kubernetes-binding-ingress.ngrok.io:443`        | `nil`                              |
| `features.defaultDomainReclaimPolicy`     | Default reclaim policy for Domains: `Delete` or `Retain`. Default: `Delete`                          | `nil`                              |
| `features.drainPolicy`                    | Drain policy on uninstall: `Delete` or `Retain`. Default: `Retain`                                   | `nil`                              |

### Credentials configuration

| Name                      | Description                                                                                                        | Value |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------ | ----- |
| `credentials.secret.name` | The name of the secret the credentials are in. If not provided, one will be generated using the helm release name. | `""`  |
| `credentials.apiKey`      | Your ngrok API key. If provided, the authtoken must be provided as well.                                           | `""`  |
| `credentials.authtoken`   | Your ngrok authtoken. If provided, the apiKey must be provided as well.                                            | `""`  |

### API Manager component

Every key under `defaults` can also be set here as `apiManager.<key>` to
override it for the api-manager pods alone. Maps deep-merge with the
component winning per key; arrays replace wholesale.

`apiManager.log.*` overrides the shared logging section above for the
api-manager alone, and `apiManager.oneClickDemoMode` is an api-manager-only
setting with no shared equivalent. Both use a `null` default: "not set"
means the operator binary's own default applies.

| Name                                            | Description                                                                                                                           | Value           |
| ----------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- | --------------- |
| `apiManager.replicaCount`                       | The number of api-manager replicas to run                                                                                             | `1`             |
| `apiManager.podDisruptionBudget.create`         | Whether to create a PodDisruptionBudget                                                                                               | `false`         |
| `apiManager.podDisruptionBudget.maxUnavailable` | Maximum unavailable pods                                                                                                              | `1`             |
| `apiManager.podDisruptionBudget.minAvailable`   | Minimum available pods. Set this instead of `maxUnavailable`, not alongside it                                                        | `nil`           |
| `apiManager.serviceAccount.create`              | Whether to create a ServiceAccount                                                                                                    | `true`          |
| `apiManager.serviceAccount.name`                | ServiceAccount name; generated if empty                                                                                               | `""`            |
| `apiManager.serviceAccount.annotations`         | Additional ServiceAccount annotations                                                                                                 | `{}`            |
| `apiManager.updateStrategy.type`                | Deployment update strategy                                                                                                            | `RollingUpdate` |
| `apiManager.oneClickDemoMode`                   | Start the api-manager without credentials, for demo installs. Also stops the agent and bindings-forwarder Deployments from rendering. | `false`         |
| `apiManager.log.level`                          | Overrides `log.level` for the api-manager only                                                                                        | `nil`           |
| `apiManager.log.format`                         | Overrides `log.format` for the api-manager only                                                                                       | `nil`           |
| `apiManager.log.stacktraceLevel`                | Overrides `log.stacktraceLevel` for the api-manager only                                                                              | `nil`           |

### Agent component

Every key under `defaults` can also be set here as `agent.<key>` to
override it for the agent pods alone. Maps deep-merge with the
component winning per key; arrays replace wholesale.

`agent.log.*` overrides the shared logging section above for the agent
alone, and `agent.watchNamespace` is an agent-only setting with no shared
equivalent. Both use a `null` default: "not set" means the operator
binary's own default applies.

| Name                               | Description                                                                   | Value           |
| ---------------------------------- | ----------------------------------------------------------------------------- | --------------- |
| `agent.replicaCount`               | The number of agent replicas to run                                           | `1`             |
| `agent.serviceAccount.create`      | Whether to create a ServiceAccount                                            | `true`          |
| `agent.serviceAccount.name`        | ServiceAccount name; generated if empty                                       | `""`            |
| `agent.serviceAccount.annotations` | Additional ServiceAccount annotations                                         | `{}`            |
| `agent.updateStrategy.type`        | Deployment update strategy                                                    | `RollingUpdate` |
| `agent.watchNamespace`             | Namespace to watch for AgentEndpoint resources. Unset watches all namespaces. | `nil`           |
| `agent.log.level`                  | Overrides `log.level` for the agent only                                      | `nil`           |
| `agent.log.format`                 | Overrides `log.format` for the agent only                                     | `nil`           |
| `agent.log.stacktraceLevel`        | Overrides `log.stacktraceLevel` for the agent only                            | `nil`           |

### Bindings Forwarder component

Every key under `defaults` can also be set here as `bindingsForwarder.<key>` to
override it for the bindings-forwarder pods alone. Maps deep-merge with the
component winning per key; arrays replace wholesale.

`bindingsForwarder.log.*` overrides the shared logging section above for
the bindings-forwarder alone. A `null` default means "not set": the
operator binary's own default applies.

| Name                                           | Description                                                     | Value           |
| ---------------------------------------------- | --------------------------------------------------------------- | --------------- |
| `bindingsForwarder.replicaCount`               | The number of bindings-forwarder replicas to run                | `1`             |
| `bindingsForwarder.serviceAccount.create`      | Whether to create a ServiceAccount                              | `true`          |
| `bindingsForwarder.serviceAccount.name`        | ServiceAccount name; generated if empty                         | `""`            |
| `bindingsForwarder.serviceAccount.annotations` | Additional ServiceAccount annotations                           | `{}`            |
| `bindingsForwarder.updateStrategy.type`        | Deployment update strategy                                      | `RollingUpdate` |
| `bindingsForwarder.log.level`                  | Overrides `log.level` for the bindings-forwarder only           | `nil`           |
| `bindingsForwarder.log.format`                 | Overrides `log.format` for the bindings-forwarder only          | `nil`           |
| `bindingsForwarder.log.stacktraceLevel`        | Overrides `log.stacktraceLevel` for the bindings-forwarder only | `nil`           |

### RBAC

| Name                         | Description                                                      | Value  |
| ---------------------------- | ---------------------------------------------------------------- | ------ |
| `crdAccessRoles.create`      | Whether to create editor/viewer ClusterRoles for CRDs            | `true` |
| `crdAccessRoles.annotations` | Annotations for CRD access ClusterRoles (e.g., RBAC aggregation) | `{}`   |

### Custom Resource Definitions installation

| Name          | Description                                                        | Value  |
| ------------- | ------------------------------------------------------------------ | ------ |
| `installCRDs` | When true, the ngrok CRDs will be installed alongside the operator | `true` |

### Cleanup Hook configuration

| Name                             | Description                                                               | Value             |
| -------------------------------- | ------------------------------------------------------------------------- | ----------------- |
| `cleanupHook.enabled`            | Enable the pre-delete cleanup hook that drains resources before uninstall | `true`            |
| `cleanupHook.timeout`            | Timeout in seconds for the cleanup process                                | `300`             |
| `cleanupHook.image.repository`   | The repository for the kubectl image used by the cleanup hook             | `bitnami/kubectl` |
| `cleanupHook.image.tag`          | The tag for the kubectl image                                             | `latest`          |
| `cleanupHook.image.pullPolicy`   | The pull policy for the cleanup hook image                                | `IfNotPresent`    |
| `cleanupHook.resources.limits`   | The resources limits for the cleanup hook container                       | `{}`              |
| `cleanupHook.resources.requests` | The requested resources for the cleanup hook container                    | `{}`              |
