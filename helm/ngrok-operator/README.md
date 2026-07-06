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

| Name                | Description                                           | Value |
| ------------------- | ----------------------------------------------------- | ----- |
| `nameOverride`      | String to partially override generated resource names | `""`  |
| `fullnameOverride`  | String to fully override generated resource names     | `""`  |
| `commonLabels`      | Labels to add to all deployed objects                 | `{}`  |
| `commonAnnotations` | Annotations to add to all deployed objects            | `{}`  |

### Image configuration

| Name                | Description                                                                       | Value                  |
| ------------------- | --------------------------------------------------------------------------------- | ---------------------- |
| `image.registry`    | The ngrok operator image registry.                                                | `docker.io`            |
| `image.repository`  | The ngrok operator image repository.                                              | `ngrok/ngrok-operator` |
| `image.tag`         | The ngrok operator image tag. Defaults to the chart's appVersion if not specified | `""`                   |
| `image.pullPolicy`  | The ngrok operator image pull policy.                                             | `IfNotPresent`         |
| `image.pullSecrets` | An array of imagePullSecrets to be used when pulling the image.                   | `[]`                   |

### ngrok configuration

| Name                        | Description                                                                                                           | Value                                     |
| --------------------------- | --------------------------------------------------------------------------------------------------------------------- | ----------------------------------------- |
| `ngrok.description`         | ngrok-operator description that will appear in the ngrok dashboard                                                    | `The official ngrok Kubernetes Operator.` |
| `ngrok.region`              | ngrok region to create tunnels in. Defaults to connect to the closest geographical region.                            | `""`                                      |
| `ngrok.rootCAs`             | Set to "trusted" for the ngrok agent CA or "host" to trust the host's CA. Defaults to "trusted".                      | `""`                                      |
| `ngrok.serverAddr`          | This is the address of the ngrok server to connect to. You should set this if you are using a custom ingress address. | `""`                                      |
| `ngrok.apiURL`              | This is the URL of the ngrok API. You should set this if you are using a custom API URL.                              | `""`                                      |
| `ngrok.metadata`            | This is a map of key/value pairs that will be added as metadata to all ngrok api resources created                    | `{}`                                      |
| `ngrok.clusterDomain`       | Configure the default cluster base domain for your kubernetes cluster DNS resolution                                  | `svc.cluster.local`                       |
| `ngrok.log.level`           | The level to log at. One of 'debug', 'info', or 'error'.                                                              | `info`                                    |
| `ngrok.log.format`          | The log format to use. One of console, json.                                                                          | `json`                                    |
| `ngrok.log.stacktraceLevel` | The level to report stacktrace logs one of 'info' or 'error'.                                                         | `error`                                   |

### Credentials configuration

| Name                      | Description                                                                                                        | Value |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------ | ----- |
| `credentials.secret.name` | The name of the secret the credentials are in. If not provided, one will be generated using the helm release name. | `""`  |
| `credentials.apiKey`      | Your ngrok API key. If provided, it will be written to the secret and the authtoken must be provided as well.      | `""`  |
| `credentials.authtoken`   | Your ngrok authtoken. If provided, it will be written to the secret and the apiKey must be provided as well.       | `""`  |

### Features configuration

| Name                                      | Description                                                                                                                                      | Value                                     |
| ----------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------- |
| `features.ingress.enabled`                | When true, enable the Ingress controller features                                                                                                | `true`                                    |
| `features.ingress.controllerName`         | The name of the controller to look for matching ingress classes                                                                                  | `k8s.ngrok.com/ingress-controller`        |
| `features.ingress.watchNamespace`         | The namespace to watch for ingress resources (default all)                                                                                       | `""`                                      |
| `features.ingress.ingressClass.name`      | The name of the ingress class to use.                                                                                                            | `ngrok`                                   |
| `features.ingress.ingressClass.create`    | Whether to create the ingress class.                                                                                                             | `true`                                    |
| `features.ingress.ingressClass.default`   | Whether to set the ingress class as default.                                                                                                     | `false`                                   |
| `features.gateway.enabled`                | When true, Gateway API support will be enabled if the CRDs are detected. When false, Gateway API support will never be enabled                   | `true`                                    |
| `features.gateway.disableReferenceGrants` | When true, disables required ReferenceGrants for cross-namespace references. Does nothing when features.gateway.enabled is false                 | `false`                                   |
| `features.bindings.enabled`               | Whether to enable the Endpoint Bindings feature                                                                                                  | `false`                                   |
| `features.bindings.endpointSelectors`     | List of cel expressions used to filter which kubernetes-bound endpoints should be projected into this cluster                                    | `["true"]`                                |
| `features.bindings.serviceAnnotations`    | Annotations to add to projected services bound to an endpoint                                                                                    | `{}`                                      |
| `features.bindings.serviceLabels`         | Labels to add to projected services bound to an endpoint                                                                                         | `{}`                                      |
| `features.bindings.ingressEndpoint`       | The hostname of the ingress endpoint for the bindings                                                                                            | `kubernetes-binding-ingress.ngrok.io:443` |
| `features.drainPolicy`                    | Policy for what to do with ngrok API resources while draining during an Uninstall. "Delete" removes ngrok API resources, "Retain" preserves them | `Retain`                                  |
| `features.defaultDomainReclaimPolicy`     | The default domain reclaim policy to use for domains created by the operator. Valid values are "Delete" and "Retain". The default is "Delete".   | `Delete`                                  |

### API Manager parameters

| Name                                            | Description                                                                                                                    | Value   |
| ----------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ | ------- |
| `apiManager.config.oneClickDemoMode`            | If true, then the operator will startup without required fields or API registration, become Ready, but not actually be running | `false` |
| `apiManager.replicaCount`                       | The number of api-manager replicas to run.                                                                                     | `1`     |
| `apiManager.podAnnotations`                     | Custom pod annotations to apply to api-manager pods.                                                                           | `{}`    |
| `apiManager.podLabels`                          | Custom pod labels to apply to api-manager pods.                                                                                | `{}`    |
| `apiManager.affinity`                           | Affinity for the api-manager pod assignment                                                                                    | `{}`    |
| `apiManager.podAffinityPreset`                  | Pod affinity preset. Ignored if `apiManager.affinity` is set. Allowed values: `soft` or `hard`                                 | `""`    |
| `apiManager.podAntiAffinityPreset`              | Pod anti-affinity preset. Ignored if `apiManager.affinity` is set. Allowed values: `soft` or `hard`                            | `soft`  |
| `apiManager.nodeAffinityPreset.type`            | Node affinity preset type. Ignored if `apiManager.affinity` is set. Allowed values: `soft` or `hard`                           | `""`    |
| `apiManager.nodeAffinityPreset.key`             | Node label key to match. Ignored if `apiManager.affinity` is set.                                                              | `""`    |
| `apiManager.nodeAffinityPreset.values`          | Node label values to match. Ignored if `apiManager.affinity` is set.                                                           | `[]`    |
| `apiManager.nodeSelector`                       | Node labels for api-manager pod(s)                                                                                             | `{}`    |
| `apiManager.tolerations`                        | Tolerations for api-manager pod(s)                                                                                             | `[]`    |
| `apiManager.topologySpreadConstraints`          | Topology Spread Constraints for api-manager pod(s)                                                                             | `[]`    |
| `apiManager.priorityClassName`                  | Priority class for pod scheduling                                                                                              | `""`    |
| `apiManager.terminationGracePeriodSeconds`      | The amount of time to wait for the pod to gracefully terminate                                                                 | `30`    |
| `apiManager.lifecycle`                          | an object containing lifecycle configuration                                                                                   | `{}`    |
| `apiManager.podDisruptionBudget.create`         | Enable a Pod Disruption Budget creation                                                                                        | `false` |
| `apiManager.podDisruptionBudget.maxUnavailable` | Maximum number/percentage of pods that may be made unavailable                                                                 | `""`    |
| `apiManager.podDisruptionBudget.minAvailable`   | Minimum number/percentage of pods that should remain scheduled                                                                 | `""`    |
| `apiManager.resources.limits`                   | The resources limits for the container                                                                                         | `{}`    |
| `apiManager.resources.requests`                 | The requested resources for the container                                                                                      | `{}`    |
| `apiManager.extraVolumes`                       | An array of extra volumes to add to the api-manager.                                                                           | `[]`    |
| `apiManager.extraVolumeMounts`                  | An array of extra volume mounts to add to the api-manager.                                                                     | `[]`    |
| `apiManager.extraEnv`                           | an object of extra environment variables to add to the api-manager.                                                            | `{}`    |
| `apiManager.serviceAccount.create`              | Specifies whether a ServiceAccount should be created                                                                           | `true`  |
| `apiManager.serviceAccount.name`                | The name of the ServiceAccount to use.                                                                                         | `""`    |
| `apiManager.serviceAccount.annotations`         | Additional annotations to add to the ServiceAccount                                                                            | `{}`    |

### Agent parameters

| Name                                  | Description                                                                                     | Value           |
| ------------------------------------- | ----------------------------------------------------------------------------------------------- | --------------- |
| `agent.config`                        | Component-specific app config for the agent                                                     | `{}`            |
| `agent.replicaCount`                  | The number of agents to run.                                                                    | `1`             |
| `agent.podAnnotations`                | Custom pod annotations to apply to agent pods.                                                  | `{}`            |
| `agent.podLabels`                     | Custom pod labels to apply to agent pods.                                                       | `{}`            |
| `agent.affinity`                      | Affinity for the agent pod assignment                                                           | `{}`            |
| `agent.podAffinityPreset`             | Pod affinity preset. Ignored if `agent.affinity` is set. Allowed values: `soft` or `hard`       | `""`            |
| `agent.podAntiAffinityPreset`         | Pod anti-affinity preset. Ignored if `agent.affinity` is set. Allowed values: `soft` or `hard`  | `soft`          |
| `agent.nodeAffinityPreset.type`       | Node affinity preset type. Ignored if `agent.affinity` is set. Allowed values: `soft` or `hard` | `""`            |
| `agent.nodeAffinityPreset.key`        | Node label key to match. Ignored if `agent.affinity` is set.                                    | `""`            |
| `agent.nodeAffinityPreset.values`     | Node label values to match. Ignored if `agent.affinity` is set.                                 | `[]`            |
| `agent.nodeSelector`                  | Node labels for the agent pod(s)                                                                | `{}`            |
| `agent.tolerations`                   | Tolerations for the agent pod(s)                                                                | `[]`            |
| `agent.topologySpreadConstraints`     | Topology Spread Constraints for the agent pod(s)                                                | `[]`            |
| `agent.priorityClassName`             | Priority class for pod scheduling.                                                              | `""`            |
| `agent.terminationGracePeriodSeconds` | The amount of time to wait for the agent pod to gracefully terminate                            | `30`            |
| `agent.lifecycle`                     | an object containing lifecycle configuration                                                    | `{}`            |
| `agent.updateStrategy.type`           | Agent update strategy                                                                           | `RollingUpdate` |
| `agent.resources.limits`              | The resources limits for the container                                                          | `{}`            |
| `agent.resources.requests`            | The requested resources for the container                                                       | `{}`            |
| `agent.extraVolumes`                  | An array of extra volumes to add to the agent.                                                  | `[]`            |
| `agent.extraVolumeMounts`             | An array of extra volume mounts to add to the agent.                                            | `[]`            |
| `agent.extraEnv`                      | an object of extra environment variables to add to the agent.                                   | `{}`            |
| `agent.serviceAccount.create`         | Specifies whether a ServiceAccount should be created for the agent.                             | `true`          |
| `agent.serviceAccount.name`           | The name of the ServiceAccount to use for the agent.                                            | `""`            |
| `agent.serviceAccount.annotations`    | Additional annotations to add to the agent ServiceAccount                                       | `{}`            |

### Bindings Forwarder parameters

| Name                                              | Description                                                                                                 | Value           |
| ------------------------------------------------- | ----------------------------------------------------------------------------------------------------------- | --------------- |
| `bindingsForwarder.config`                        | Component-specific app config for the bindings forwarder                                                    | `{}`            |
| `bindingsForwarder.replicaCount`                  | The number of bindings forwarders to run.                                                                   | `1`             |
| `bindingsForwarder.podAnnotations`                | Custom pod annotations to apply to bindings forwarder pods.                                                 | `{}`            |
| `bindingsForwarder.podLabels`                     | Custom pod labels to apply to bindings forwarder pods.                                                      | `{}`            |
| `bindingsForwarder.affinity`                      | Affinity for the bindings forwarder pod assignment                                                          | `{}`            |
| `bindingsForwarder.podAffinityPreset`             | Pod affinity preset. Ignored if `bindingsForwarder.affinity` is set. Allowed values: `soft` or `hard`       | `""`            |
| `bindingsForwarder.podAntiAffinityPreset`         | Pod anti-affinity preset. Ignored if `bindingsForwarder.affinity` is set. Allowed values: `soft` or `hard`  | `soft`          |
| `bindingsForwarder.nodeAffinityPreset.type`       | Node affinity preset type. Ignored if `bindingsForwarder.affinity` is set. Allowed values: `soft` or `hard` | `""`            |
| `bindingsForwarder.nodeAffinityPreset.key`        | Node label key to match. Ignored if `bindingsForwarder.affinity` is set.                                    | `""`            |
| `bindingsForwarder.nodeAffinityPreset.values`     | Node label values to match. Ignored if `bindingsForwarder.affinity` is set.                                 | `[]`            |
| `bindingsForwarder.nodeSelector`                  | Node labels for the bindings forwarder pod(s)                                                               | `{}`            |
| `bindingsForwarder.tolerations`                   | Tolerations for the bindings forwarder pod(s)                                                               | `[]`            |
| `bindingsForwarder.topologySpreadConstraints`     | Topology Spread Constraints for the bindings forwarder pod(s)                                               | `[]`            |
| `bindingsForwarder.priorityClassName`             | Priority class for pod scheduling.                                                                          | `""`            |
| `bindingsForwarder.terminationGracePeriodSeconds` | The amount of time to wait for the bindings forwarder pod to gracefully terminate                           | `30`            |
| `bindingsForwarder.lifecycle`                     | an object containing lifecycle configuration                                                                | `{}`            |
| `bindingsForwarder.updateStrategy.type`           | Bindings Forwarder update strategy type                                                                     | `RollingUpdate` |
| `bindingsForwarder.resources.limits`              | The resources limits for the container                                                                      | `{}`            |
| `bindingsForwarder.resources.requests`            | The requested resources for the container                                                                   | `{}`            |
| `bindingsForwarder.extraVolumes`                  | An array of extra volumes to add to the bindings forwarder.                                                 | `[]`            |
| `bindingsForwarder.extraVolumeMounts`             | An array of extra volume mounts to add to the bindings forwarder.                                           | `[]`            |
| `bindingsForwarder.extraEnv`                      | an object of extra environment variables to add to the bindings forwarder.                                  | `{}`            |
| `bindingsForwarder.serviceAccount.create`         | Specifies whether a ServiceAccount should be created for the bindings forwarder pod(s).                     | `true`          |
| `bindingsForwarder.serviceAccount.name`           | The name of the ServiceAccount to use for the bindings forwarder pod(s).                                    | `""`            |
| `bindingsForwarder.serviceAccount.annotations`    | Additional annotations to add to the bindings-forwarder ServiceAccount                                      | `{}`            |

### CRD Access Role Settings

| Name                         | Description                                                      | Value  |
| ---------------------------- | ---------------------------------------------------------------- | ------ |
| `crdAccessRoles.create`      | Whether to create editor/viewer ClusterRoles for CRDs            | `true` |
| `crdAccessRoles.annotations` | Annotations for CRD access ClusterRoles (e.g., RBAC aggregation) | `{}`   |

### Custom Resource Definitions installation

| Name          | Description                                                        | Value  |
| ------------- | ------------------------------------------------------------------ | ------ |
| `installCRDs` | When true, the ngrok CRDs will be installed alongside the operator | `true` |

### Cleanup Hook configuration

| Name                                    | Description                                                               | Value             |
| --------------------------------------- | ------------------------------------------------------------------------- | ----------------- |
| `cleanupHook.enabled`                   | Enable the pre-delete cleanup hook that drains resources before uninstall | `true`            |
| `cleanupHook.timeout`                   | Timeout in seconds for the cleanup process                                | `300`             |
| `cleanupHook.image.repository`          | The repository for the kubectl image used by the cleanup hook             | `bitnami/kubectl` |
| `cleanupHook.image.tag`                 | The tag for the kubectl image                                             | `latest`          |
| `cleanupHook.image.pullPolicy`          | The pull policy for the cleanup hook image                                | `IfNotPresent`    |
| `cleanupHook.resources.limits.cpu`      | CPU limit for the cleanup hook container                                  | `100m`            |
| `cleanupHook.resources.limits.memory`   | Memory limit for the cleanup hook container                               | `128Mi`           |
| `cleanupHook.resources.requests.cpu`    | CPU request for the cleanup hook container                                | `50m`             |
| `cleanupHook.resources.requests.memory` | Memory request for the cleanup hook container                             | `64Mi`            |
