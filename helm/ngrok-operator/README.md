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

### Shared pod settings

| Name                                 | Description                                                                               | Value  |
| ------------------------------------ | ----------------------------------------------------------------------------------------- | ------ |
| `defaults.podAnnotations`            | Pod annotations                                                                           | `{}`   |
| `defaults.podLabels`                 | Pod labels                                                                                | `{}`   |
| `defaults.nodeSelector`              | Node labels for pod assignment                                                            | `{}`   |
| `defaults.tolerations`               | Tolerations for pod assignment                                                            | `[]`   |
| `defaults.affinity`                  | Affinity rules. Overrides the presets below when set                                      | `{}`   |
| `defaults.podAffinityPreset`         | Pod affinity preset. Ignored if `affinity` is set. Allowed values: `soft` or `hard`       | `""`   |
| `defaults.podAntiAffinityPreset`     | Pod anti-affinity preset. Ignored if `affinity` is set. Allowed values: `soft` or `hard`  | `soft` |
| `defaults.nodeAffinityPreset.type`   | Node affinity preset type. Ignored if `affinity` is set. Allowed values: `soft` or `hard` | `""`   |
| `defaults.nodeAffinityPreset.key`    | Node label key to match. Ignored if `affinity` is set.                                    | `""`   |
| `defaults.nodeAffinityPreset.values` | Node label values to match. Ignored if `affinity` is set.                                 | `[]`   |
| `defaults.topologySpreadConstraints` | Topology spread constraints for pod assignment                                            | `[]`   |
| `defaults.priorityClassName`         | Priority class for pod scheduling                                                         | `""`   |
| `defaults.extraEnv`                  | Additional environment variables, as a map of name to value                               | `{}`   |

### Operator configuration

| Name                                      | Description                                                                                                 | Value                              |
| ----------------------------------------- | ----------------------------------------------------------------------------------------------------------- | ---------------------------------- |
| `ngrok.description`                       | Description of this installation in the ngrok dashboard. Default: `The official ngrok Kubernetes Operator.` | `""`                               |
| `ngrok.region`                            | ngrok region to use. Default: the account's default region                                                  | `""`                               |
| `ngrok.serverAddr`                        | Address of the ngrok server to use for tunnels. Default: the ngrok default                                  | `""`                               |
| `ngrok.apiURL`                            | Base URL for the ngrok API. Default: the ngrok default                                                      | `""`                               |
| `ngrok.rootCAs`                           | Root CAs to trust: `trusted` for the ngrok CA, `host` for the host's CA bundle. Default: `trusted`          | `""`                               |
| `ngrok.clusterDomain`                     | Cluster domain used when resolving in-cluster service addresses. Default: `svc.cluster.local`               | `""`                               |
| `ngrok.metadata`                          | Key/value pairs added as metadata to the ngrok API resources the operator creates                           | `{}`                               |
| `log.level`                               | Log level: `debug`, `info`, `error`, `panic`, or an integer for more verbose debug levels. Default: `info`  | `""`                               |
| `log.format`                              | Log format: `json` or `console`. Default: `json`                                                            | `""`                               |
| `log.stacktraceLevel`                     | Level at and above which stacktraces are captured: `info`, `error` or `panic`. Default: `error`             | `""`                               |
| `features.ingress.enabled`                | Enable the Kubernetes Ingress controller                                                                    | `true`                             |
| `features.ingress.controllerName`         | Controller name matched by IngressClasses, and set on the IngressClass this chart creates                   | `k8s.ngrok.com/ingress-controller` |
| `features.ingress.watchNamespace`         | Namespace to watch for Ingress and AgentEndpoint resources. Default: all namespaces                         | `""`                               |
| `features.ingress.ingressClass.name`      | IngressClass resource name                                                                                  | `ngrok`                            |
| `features.ingress.ingressClass.create`    | Create the IngressClass resource                                                                            | `true`                             |
| `features.ingress.ingressClass.default`   | Set the IngressClass as the cluster default                                                                 | `false`                            |
| `features.gateway.enabled`                | Enable Gateway API support, if the Gateway API CRDs are detected                                            | `true`                             |
| `features.gateway.disableReferenceGrants` | Disable the ReferenceGrant requirement for cross-namespace references                                       | `false`                            |
| `features.bindings.enabled`               | Enable the Endpoint Bindings feature, including the bindings-forwarder                                      | `false`                            |
| `features.bindings.endpointSelectors`     | CEL expressions filtering which endpoints are projected into this cluster. Default: `["true"]`              | `[]`                               |
| `features.bindings.serviceAnnotations`    | Annotations applied to projected services                                                                   | `{}`                               |
| `features.bindings.serviceLabels`         | Labels applied to projected services                                                                        | `{}`                               |
| `features.bindings.ingressEndpoint`       | Hostname of the bindings ingress endpoint. Default: `kubernetes-binding-ingress.ngrok.io:443`               | `""`                               |
| `features.defaultDomainReclaimPolicy`     | Default reclaim policy for Domains: `Delete` or `Retain`. Default: `Delete`                                 | `""`                               |
| `features.drainPolicy`                    | What to do with ngrok API resources on uninstall: `Delete` or `Retain`. Default: `Retain`                   | `""`                               |

### Credentials configuration

| Name                                 | Description                                                                                                        | Value |
| ------------------------------------ | ------------------------------------------------------------------------------------------------------------------ | ----- |
| `credentials.secret.name`            | The name of the secret the credentials are in. If not provided, one will be generated using the helm release name. | `""`  |
| `credentials.accessToken`            | Your ngrok access token. Used by every component that needs one, unless overridden below.                          | `""`  |
| `credentials.agent.accessToken`      | Optional access token for the agent-manager only. Falls back to credentials.accessToken.                           | `""`  |
| `credentials.apiManager.accessToken` | Optional access token for the api-manager only. Falls back to credentials.accessToken.                             | `""`  |

### API Manager

| Name                                            | Description                                                                                                                                     | Value           |
| ----------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- | --------------- |
| `apiManager.replicaCount`                       | The number of api-manager replicas to run                                                                                                       | `1`             |
| `apiManager.resources.limits`                   | Resource limits                                                                                                                                 | `{}`            |
| `apiManager.resources.requests`                 | Resource requests                                                                                                                               | `{}`            |
| `apiManager.lifecycle`                          | Container lifecycle hooks                                                                                                                       | `{}`            |
| `apiManager.terminationGracePeriodSeconds`      | Graceful shutdown period                                                                                                                        | `30`            |
| `apiManager.extraVolumes`                       | Additional volumes                                                                                                                              | `[]`            |
| `apiManager.extraVolumeMounts`                  | Additional volume mounts                                                                                                                        | `[]`            |
| `apiManager.podDisruptionBudget.create`         | Whether to create a PodDisruptionBudget                                                                                                         | `false`         |
| `apiManager.podDisruptionBudget.maxUnavailable` | Maximum unavailable pods                                                                                                                        | `1`             |
| `apiManager.podDisruptionBudget.minAvailable`   | Minimum available pods. Set this instead of `maxUnavailable`, not alongside it                                                                  | `""`            |
| `apiManager.serviceAccount.create`              | Whether to create a ServiceAccount                                                                                                              | `true`          |
| `apiManager.serviceAccount.name`                | ServiceAccount name. Generated if empty                                                                                                         | `""`            |
| `apiManager.serviceAccount.annotations`         | ServiceAccount annotations                                                                                                                      | `{}`            |
| `apiManager.updateStrategy.type`                | Deployment update strategy                                                                                                                      | `RollingUpdate` |
| `apiManager.config.oneClickDemoMode`            | Start without credentials and become Ready without reconciling, for marketplace installs. Also skips rendering the agent and bindings-forwarder | `false`         |

### Agent

| Name                                  | Description                                                        | Value           |
| ------------------------------------- | ------------------------------------------------------------------ | --------------- |
| `agent.replicaCount`                  | The number of agent replicas to run                                | `1`             |
| `agent.resources.limits`              | Resource limits                                                    | `{}`            |
| `agent.resources.requests`            | Resource requests                                                  | `{}`            |
| `agent.lifecycle`                     | Container lifecycle hooks                                          | `{}`            |
| `agent.terminationGracePeriodSeconds` | Graceful shutdown period                                           | `30`            |
| `agent.extraVolumes`                  | Additional volumes                                                 | `[]`            |
| `agent.extraVolumeMounts`             | Additional volume mounts                                           | `[]`            |
| `agent.serviceAccount.create`         | Whether to create a ServiceAccount                                 | `true`          |
| `agent.serviceAccount.name`           | ServiceAccount name. Generated if empty                            | `""`            |
| `agent.serviceAccount.annotations`    | ServiceAccount annotations                                         | `{}`            |
| `agent.updateStrategy.type`           | Deployment update strategy                                         | `RollingUpdate` |
| `agent.config`                        | Settings only the agent has (none yet), and `log` overrides for it | `{}`            |

### Bindings Forwarder

| Name                                              | Description                                                                     | Value           |
| ------------------------------------------------- | ------------------------------------------------------------------------------- | --------------- |
| `bindingsForwarder.replicaCount`                  | The number of bindings-forwarder replicas to run                                | `1`             |
| `bindingsForwarder.resources.limits`              | Resource limits                                                                 | `{}`            |
| `bindingsForwarder.resources.requests`            | Resource requests                                                               | `{}`            |
| `bindingsForwarder.lifecycle`                     | Container lifecycle hooks                                                       | `{}`            |
| `bindingsForwarder.terminationGracePeriodSeconds` | Graceful shutdown period                                                        | `30`            |
| `bindingsForwarder.extraVolumes`                  | Additional volumes                                                              | `[]`            |
| `bindingsForwarder.extraVolumeMounts`             | Additional volume mounts                                                        | `[]`            |
| `bindingsForwarder.serviceAccount.create`         | Whether to create a ServiceAccount                                              | `true`          |
| `bindingsForwarder.serviceAccount.name`           | ServiceAccount name. Generated if empty                                         | `""`            |
| `bindingsForwarder.serviceAccount.annotations`    | ServiceAccount annotations                                                      | `{}`            |
| `bindingsForwarder.updateStrategy.type`           | Deployment update strategy                                                      | `RollingUpdate` |
| `bindingsForwarder.config`                        | Settings only the bindings-forwarder has (none yet), and `log` overrides for it | `{}`            |

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
