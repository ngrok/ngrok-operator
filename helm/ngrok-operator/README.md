# ngrok Kubernetes Operator

This is the Helm chart to install official the ngrok Kubernetes Operator

# Usage

## Prerequisites

The cluster must be set up with a secret named `ngrok-operator-credentials` holding an ngrok
[access token](https://dashboard.ngrok.com/access-tokens) under both of these keys:
* AGENT_ACCESS_TOKEN
* API_MANAGER_ACCESS_TOKEN

Both keys may hold the same token. Setting `credentials.accessToken` creates this secret for you.

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

### RBAC

| Name                         | Description                                                      | Value  |
| ---------------------------- | ---------------------------------------------------------------- | ------ |
| `crdAccessRoles.create`      | Whether to create editor/viewer ClusterRoles for CRDs            | `true` |
| `crdAccessRoles.annotations` | Annotations for CRD access ClusterRoles (e.g., RBAC aggregation) | `{}`   |

### Custom Resource Definitions installation

| Name          | Description                                                        | Value  |
| ------------- | ------------------------------------------------------------------ | ------ |
| `installCRDs` | When true, the ngrok CRDs will be installed alongside the operator | `true` |

### ngrok

| Name                                            | Description                                                                                                                                     | Value                              |
| ----------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------- |
| `ngrok.credentials.secret.name`                 | The name of the secret the credentials are in. If not provided, one will be generated using the helm release name.                              | `""`                               |
| `ngrok.credentials.accessToken`                 | Your ngrok access token. Used by every component that needs one, unless overridden below.                                                       | `""`                               |
| `ngrok.credentials.agent.accessToken`           | Optional access token for the agent-manager only. Falls back to ngrok.credentials.accessToken.                                                  | `""`                               |
| `ngrok.credentials.apiManager.accessToken`      | Optional access token for the api-manager only. Falls back to ngrok.credentials.accessToken.                                                    | `""`                               |
| `ngrok.description`                             | Description of this installation in the ngrok dashboard. Default: `The official ngrok Kubernetes Operator.`                                     | `""`                               |
| `ngrok.region`                                  | ngrok region to use. Default: the account's default region                                                                                      | `""`                               |
| `ngrok.serverAddr`                              | Address of the ngrok server to use for tunnels. Default: the ngrok default                                                                      | `""`                               |
| `ngrok.apiURL`                                  | Base URL for the ngrok API. Default: the ngrok default                                                                                          | `""`                               |
| `ngrok.rootCAs`                                 | Root CAs to trust: `trusted` for the ngrok CA, `host` for the host's CA bundle. Default: `trusted`                                              | `""`                               |
| `ngrok.metadata`                                | Key/value pairs added as metadata to the ngrok API resources the operator creates                                                               | `{}`                               |
| `ngrok.clusterDomain`                           | Cluster domain used when resolving in-cluster service addresses. Default: `svc.cluster.local`                                                   | `""`                               |
| `ngrok.log.level`                               | Log level: `debug`, `info`, `error`, `panic`, or an integer for more verbose debug levels. Default: `info`                                      | `""`                               |
| `ngrok.log.format`                              | Log format: `json` or `console`. Default: `json`                                                                                                | `""`                               |
| `ngrok.log.stacktraceLevel`                     | Level at and above which stacktraces are captured: `info`, `error` or `panic`. Default: `error`                                                 | `""`                               |
| `ngrok.features.ingress.enabled`                | Enable the Kubernetes Ingress controller                                                                                                        | `true`                             |
| `ngrok.features.ingress.controllerName`         | Controller name matched by IngressClasses, and set on the IngressClass this chart creates                                                       | `k8s.ngrok.com/ingress-controller` |
| `ngrok.features.ingress.watchNamespace`         | Namespace to watch for Ingress and AgentEndpoint resources. Default: all namespaces                                                             | `""`                               |
| `ngrok.features.ingress.ingressClass.name`      | IngressClass resource name                                                                                                                      | `ngrok`                            |
| `ngrok.features.ingress.ingressClass.create`    | Create the IngressClass resource                                                                                                                | `true`                             |
| `ngrok.features.ingress.ingressClass.default`   | Set the IngressClass as the cluster default                                                                                                     | `false`                            |
| `ngrok.features.gateway.enabled`                | Enable Gateway API support, if the Gateway API CRDs are detected                                                                                | `true`                             |
| `ngrok.features.gateway.disableReferenceGrants` | Disable the ReferenceGrant requirement for cross-namespace references                                                                           | `false`                            |
| `ngrok.features.bindings.enabled`               | Enable the Endpoint Bindings feature, including the bindings-forwarder                                                                          | `false`                            |
| `ngrok.features.bindings.endpointSelectors`     | CEL expressions filtering which endpoints are projected into this cluster. Default: `["true"]`                                                  | `[]`                               |
| `ngrok.features.bindings.serviceAnnotations`    | Annotations applied to projected services                                                                                                       | `{}`                               |
| `ngrok.features.bindings.serviceLabels`         | Labels applied to projected services                                                                                                            | `{}`                               |
| `ngrok.features.bindings.ingressEndpoint`       | Hostname of the bindings ingress endpoint. Default: `kubernetes-binding-ingress.ngrok.io:443`                                                   | `""`                               |
| `ngrok.features.domains.defaultReclaimPolicy`   | Reclaim policy given to the Domains the operator creates: `Delete` or `Retain`. Default: `Delete`                                               | `""`                               |
| `ngrok.features.cleanup.enabled`                | Run a pre-delete hook on uninstall that deletes the KubernetesOperator resource, so the operator drains before it is removed                    | `true`                             |
| `ngrok.features.cleanup.timeout`                | Seconds the hook waits for the drain to finish                                                                                                  | `300`                              |
| `ngrok.features.cleanup.drainPolicy`            | What the drain does with the ngrok API resources the operator created: `Delete` or `Retain`. Default: `Retain`                                  | `""`                               |
| `ngrok.features.oneClickDemoMode.enabled`       | Start without credentials and become Ready without reconciling, for marketplace installs. Also skips rendering the agent and bindings-forwarder | `false`                            |

### Components

| Name                                                         | Description                                                                               | Value             |
| ------------------------------------------------------------ | ----------------------------------------------------------------------------------------- | ----------------- |
| `components.common.podAnnotations`                           | Pod annotations                                                                           | `{}`              |
| `components.common.podLabels`                                | Pod labels                                                                                | `{}`              |
| `components.common.nodeSelector`                             | Node labels for pod assignment                                                            | `{}`              |
| `components.common.tolerations`                              | Tolerations for pod assignment                                                            | `[]`              |
| `components.common.affinity`                                 | Affinity rules. Overrides the presets below when set                                      | `{}`              |
| `components.common.podAffinityPreset`                        | Pod affinity preset. Ignored if `affinity` is set. Allowed values: `soft` or `hard`       | `""`              |
| `components.common.podAntiAffinityPreset`                    | Pod anti-affinity preset. Ignored if `affinity` is set. Allowed values: `soft` or `hard`  | `soft`            |
| `components.common.nodeAffinityPreset.type`                  | Node affinity preset type. Ignored if `affinity` is set. Allowed values: `soft` or `hard` | `""`              |
| `components.common.nodeAffinityPreset.key`                   | Node label key to match. Ignored if `affinity` is set.                                    | `""`              |
| `components.common.nodeAffinityPreset.values`                | Node label values to match. Ignored if `affinity` is set.                                 | `[]`              |
| `components.common.topologySpreadConstraints`                | Topology spread constraints for pod assignment                                            | `[]`              |
| `components.common.priorityClassName`                        | Priority class for pod scheduling                                                         | `""`              |
| `components.common.extraEnv`                                 | Additional environment variables, as a map of name to value                               | `{}`              |
| `components.apiManager.replicaCount`                         | The number of api-manager replicas to run                                                 | `1`               |
| `components.apiManager.resources.limits`                     | Resource limits                                                                           | `{}`              |
| `components.apiManager.resources.requests`                   | Resource requests                                                                         | `{}`              |
| `components.apiManager.lifecycle`                            | Container lifecycle hooks                                                                 | `{}`              |
| `components.apiManager.terminationGracePeriodSeconds`        | Graceful shutdown period                                                                  | `30`              |
| `components.apiManager.extraVolumes`                         | Additional volumes                                                                        | `[]`              |
| `components.apiManager.extraVolumeMounts`                    | Additional volume mounts                                                                  | `[]`              |
| `components.apiManager.podDisruptionBudget.create`           | Whether to create a PodDisruptionBudget                                                   | `false`           |
| `components.apiManager.podDisruptionBudget.maxUnavailable`   | Maximum unavailable pods                                                                  | `1`               |
| `components.apiManager.podDisruptionBudget.minAvailable`     | Minimum available pods. Set this instead of `maxUnavailable`, not alongside it            | `""`              |
| `components.apiManager.serviceAccount.create`                | Whether to create a ServiceAccount                                                        | `true`            |
| `components.apiManager.serviceAccount.name`                  | ServiceAccount name. Generated if empty                                                   | `""`              |
| `components.apiManager.serviceAccount.annotations`           | ServiceAccount annotations                                                                | `{}`              |
| `components.apiManager.updateStrategy.type`                  | Deployment update strategy                                                                | `RollingUpdate`   |
| `components.apiManager.log`                                  | Overrides of the shared `log` settings for the api-manager                                | `{}`              |
| `components.agent.replicaCount`                              | The number of agent replicas to run                                                       | `1`               |
| `components.agent.resources.limits`                          | Resource limits                                                                           | `{}`              |
| `components.agent.resources.requests`                        | Resource requests                                                                         | `{}`              |
| `components.agent.lifecycle`                                 | Container lifecycle hooks                                                                 | `{}`              |
| `components.agent.terminationGracePeriodSeconds`             | Graceful shutdown period                                                                  | `30`              |
| `components.agent.extraVolumes`                              | Additional volumes                                                                        | `[]`              |
| `components.agent.extraVolumeMounts`                         | Additional volume mounts                                                                  | `[]`              |
| `components.agent.serviceAccount.create`                     | Whether to create a ServiceAccount                                                        | `true`            |
| `components.agent.serviceAccount.name`                       | ServiceAccount name. Generated if empty                                                   | `""`              |
| `components.agent.serviceAccount.annotations`                | ServiceAccount annotations                                                                | `{}`              |
| `components.agent.updateStrategy.type`                       | Deployment update strategy                                                                | `RollingUpdate`   |
| `components.agent.log`                                       | Overrides of the shared `log` settings for the agent                                      | `{}`              |
| `components.bindingsForwarder.replicaCount`                  | The number of bindings-forwarder replicas to run                                          | `1`               |
| `components.bindingsForwarder.resources.limits`              | Resource limits                                                                           | `{}`              |
| `components.bindingsForwarder.resources.requests`            | Resource requests                                                                         | `{}`              |
| `components.bindingsForwarder.lifecycle`                     | Container lifecycle hooks                                                                 | `{}`              |
| `components.bindingsForwarder.terminationGracePeriodSeconds` | Graceful shutdown period                                                                  | `30`              |
| `components.bindingsForwarder.extraVolumes`                  | Additional volumes                                                                        | `[]`              |
| `components.bindingsForwarder.extraVolumeMounts`             | Additional volume mounts                                                                  | `[]`              |
| `components.bindingsForwarder.serviceAccount.create`         | Whether to create a ServiceAccount                                                        | `true`            |
| `components.bindingsForwarder.serviceAccount.name`           | ServiceAccount name. Generated if empty                                                   | `""`              |
| `components.bindingsForwarder.serviceAccount.annotations`    | ServiceAccount annotations                                                                | `{}`              |
| `components.bindingsForwarder.updateStrategy.type`           | Deployment update strategy                                                                | `RollingUpdate`   |
| `components.bindingsForwarder.log`                           | Overrides of the shared `log` settings for the bindings-forwarder                         | `{}`              |
| `components.cleanupHook.image.repository`                    | The repository for the kubectl image used by the cleanup hook                             | `bitnami/kubectl` |
| `components.cleanupHook.image.tag`                           | The tag for the kubectl image                                                             | `latest`          |
| `components.cleanupHook.image.pullPolicy`                    | The pull policy for the cleanup hook image                                                | `IfNotPresent`    |
| `components.cleanupHook.resources.limits`                    | The resources limits for the cleanup hook container                                       | `{}`              |
| `components.cleanupHook.resources.requests`                  | The requested resources for the cleanup hook container                                    | `{}`              |
