# ngrok Ingress Controller

This is the helm chart to install the ngrok ingress controller

# Usage

## Prerequisites

The cluster Must be setup with a secret named `ngrok-operator-credentials` with the following keys:
* AUTHTOKEN
* API\_KEY

## Install the controller with helm

[Helm](https://helm.sh) must be installed to use the charts.  Please refer to
Helm's [documentation](https://helm.sh/docs) to get started.

Once Helm has been set up correctly, add the repo as follows:

`helm repo add ngrok https://charts.ngrok.com`

If you had already added this repo earlier, run `helm repo update` to retrieve
the latest versions of the packages.  You can then run `helm search repo ngrok` to see the charts.

To install the ngrok-operator chart:

`helm install my-ngrok-operator ngrok/ngrok-operator`

To uninstall the chart:

`helm delete my-ngrok-operator`

<!-- Parameters are auto generated via @bitnami/readme-generator-for-helm -->
## Parameters

### Common parameters

| Name                | Description                                                        | Value                                     |
| ------------------- | ------------------------------------------------------------------ | ----------------------------------------- |
| `nameOverride`      | String to partially override generated resource names              | `""`                                      |
| `fullnameOverride`  | String to fully override generated resource names                  | `""`                                      |
| `description`       | ngrok-operator description that will appear in the ngrok dashboard | `The official ngrok Kubernetes Operator.` |
| `commonLabels`      | Labels to add to all deployed objects                              | `{}`                                      |
| `commonAnnotations` | Annotations to add to all deployed objects                         | `{}`                                      |

### Image configuration

| Name                | Description                                                                       | Value                  |
| ------------------- | --------------------------------------------------------------------------------- | ---------------------- |
| `image.registry`    | The ngrok operator image registry.                                                | `docker.io`            |
| `image.repository`  | The ngrok operator image repository.                                              | `ngrok/ngrok-operator` |
| `image.tag`         | The ngrok operator image tag. Defaults to the chart's appVersion if not specified | `""`                   |
| `image.pullPolicy`  | The ngrok operator image pull policy.                                             | `IfNotPresent`         |
| `image.pullSecrets` | An array of imagePullSecrets to be used when pulling the image.                   | `[]`                   |

### ngrok configuration

| Name            | Description                                                                                                           | Value               |
| --------------- | --------------------------------------------------------------------------------------------------------------------- | ------------------- |
| `region`        | ngrok region to create tunnels in. Defaults to connect to the closest geographical region.                            | `""`                |
| `rootCAs`       | Set to "trusted" for the ngrok agent CA or "host" to trust the host's CA. Defaults to "trusted".                      | `""`                |
| `serverAddr`    | This is the address of the ngrok server to connect to. You should set this if you are using a custom ingress address. | `""`                |
| `apiURL`        | This is the URL of the ngrok API. You should set this if you are using a custom API URL.                              | `""`                |
| `metaData`      | DEPRECATED: Use ngrokMetadata instead                                                                                 |                     |
| `ngrokMetadata` | This is a map of key=value,key=value pairs that will be added as metadata to all ngrok api resources created          | `{}`                |
| `clusterDomain` | Configure the default cluster base domain for your kubernetes cluster DNS resolution                                  | `svc.cluster.local` |

### Operator Manager parameters

| Name                                 | Description                                                                               | Value   |
| ------------------------------------ | ----------------------------------------------------------------------------------------- | ------- |
| `podAnnotations`                     | Used to apply custom annotations to the ingress pods.                                     | `{}`    |
| `podLabels`                          | Used to apply custom labels to the ingress pods.                                          | `{}`    |
| `replicaCount`                       | The number of controllers to run.                                                         | `1`     |
| `affinity`                           | Affinity for the controller pod assignment                                                | `{}`    |
| `podAffinityPreset`                  | Pod affinity preset. Ignored if `affinity` is set. Allowed values: `soft` or `hard`       | `""`    |
| `podAntiAffinityPreset`              | Pod anti-affinity preset. Ignored if `affinity` is set. Allowed values: `soft` or `hard`  | `soft`  |
| `nodeAffinityPreset.type`            | Node affinity preset type. Ignored if `affinity` is set. Allowed values: `soft` or `hard` | `""`    |
| `nodeAffinityPreset.key`             | Node label key to match. Ignored if `affinity` is set.                                    | `""`    |
| `nodeAffinityPreset.values`          | Node label values to match. Ignored if `affinity` is set.                                 | `[]`    |
| `priorityClassName`                  | Priority class for pod scheduling                                                         | `""`    |
| `lifecycle`                          | an object containing lifecycle configuration                                              | `{}`    |
| `podDisruptionBudget.create`         | Enable a Pod Disruption Budget creation                                                   | `false` |
| `podDisruptionBudget.maxUnavailable` | Maximum number/percentage of pods that may be made unavailable                            | `""`    |
| `podDisruptionBudget.minAvailable`   | Minimum number/percentage of pods that should remain scheduled                            | `""`    |
| `resources.limits`                   | The resources limits for the container                                                    | `{}`    |
| `resources.requests`                 | The requested resources for the container                                                 | `{}`    |
| `extraVolumes`                       | An array of extra volumes to add to the controller.                                       | `[]`    |
| `extraVolumeMounts`                  | An array of extra volume mounts to add to the controller.                                 | `[]`    |
| `extraEnv`                           | an object of extra environment variables to add to the controller.                        | `{}`    |
| `serviceAccount.create`              | Specifies whether a ServiceAccount should be created                                      | `true`  |
| `serviceAccount.name`                | The name of the ServiceAccount to use.                                                    | `""`    |
| `serviceAccount.annotations`         | Additional annotations to add to the ServiceAccount                                       | `{}`    |

### Logging configuration

| Name                  | Description                                                   | Value   |
| --------------------- | ------------------------------------------------------------- | ------- |
| `log.level`           | The level to log at. One of 'debug', 'info', or 'error'.      | `info`  |
| `log.stacktraceLevel` | The level to report stacktrace logs one of 'info' or 'error'. | `error` |
| `log.format`          | The log format to use. One of console, json.                  | `json`  |

### Credentials configuration

| Name                      | Description                                                                                                        | Value |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------ | ----- |
| `credentials.secret.name` | The name of the secret the credentials are in. If not provided, one will be generated using the helm release name. | `""`  |
| `credentials.apiKey`      | Your ngrok API key. If provided, it will be written to the secret and the authtoken must be provided as well.      | `""`  |
| `credentials.authtoken`   | Your ngrok authtoken. If provided, it will be written to the secret and the apiKey must be provided as well.       | `""`  |

### Kubernetes Ingress feature configuration

| Name                           | Description                                                     | Value                              |
| ------------------------------ | --------------------------------------------------------------- | ---------------------------------- |
| `ingressClass.name`            | DEPRECATED: Use ingress.ingressClass.name instead               |                                    |
| `ingressClass.create`          | DEPRECATED: Use ingress.ingressClass.create instead             |                                    |
| `ingressClass.default`         | DEPRECATED: Use ingress.ingressClass.default instead            |                                    |
| `watchNamespace`               | DEPRECATED: Use ingress.watchNamespace instead                  |                                    |
| `controllerName`               | DEPRECATED: Use ingress.controllerName instead                  |                                    |
| `ingress.enabled`              | When true, enable the Ingress controller features               | `true`                             |
| `ingress.ingressClass.name`    | The name of the ingress class to use.                           | `ngrok`                            |
| `ingress.ingressClass.create`  | Whether to create the ingress class.                            | `true`                             |
| `ingress.ingressClass.default` | Whether to set the ingress class as default.                    | `false`                            |
| `ingress.watchNamespace`       | The namespace to watch for ingress resources (default all)      | `""`                               |
| `ingress.controllerName`       | The name of the controller to look for matching ingress classes | `k8s.ngrok.com/ingress-controller` |

### Agent configuration

| Name                               | Description                                                         | Value  |
| ---------------------------------- | ------------------------------------------------------------------- | ------ |
| `agent.priorityClassName`          | Priority class for pod scheduling.                                  | `""`   |
| `agent.replicaCount`               | The number of agents to run.                                        | `1`    |
| `agent.serviceAccount.create`      | Specifies whether a ServiceAccount should be created for the agent. | `true` |
| `agent.serviceAccount.name`        | The name of the ServiceAccount to use for the agent.                | `""`   |
| `agent.serviceAccount.annotations` | Additional annotations to add to the agent ServiceAccount           | `{}`   |

### Kubernetes Gateway feature configuration

| Name                        | Description                              | Value   |
| --------------------------- | ---------------------------------------- | ------- |
| `useExperimentalGatewayApi` | DEPRECATED: Use gateway.enabled instead  |         |
| `gateway.enabled`           | When true, enable the Gateway controller | `false` |

### Kubernetes Bindings feature configuration

| Name                                            | Description                                                                             | Value                                     |
| ----------------------------------------------- | --------------------------------------------------------------------------------------- | ----------------------------------------- |
| `bindings.enabled`                              | Whether to enable the Endpoint Bindings feature                                         | `false`                                   |
| `bindings.name`                                 | Unique name of this kubernetes binding in your ngrok account                            | `""`                                      |
| `bindings.description`                          | Description of this kubernetes binding in your ngrok account                            | `Created by ngrok-operator`               |
| `bindings.allowedURLs`                          | List of allowed endpoint URL formats that this binding will project into the cluster    | `["*"]`                                   |
| `bindings.serviceAnnotations`                   | Annotations to add to projected services bound to an endpoint                           | `{}`                                      |
| `bindings.serviceLabels`                        | Labels to add to projected services bound to an endpoint                                | `{}`                                      |
| `bindings.ingressEndpoint`                      | The hostname of the ingress endpoint for the bindings                                   | `kubernetes-binding-ingress.ngrok.io:443` |
| `bindings.forwarder.replicaCount`               | The number of bindings forwarders to run.                                               | `1`                                       |
| `bindings.forwarder.serviceAccount.create`      | Specifies whether a ServiceAccount should be created for the bindings forwarder pod(s). | `true`                                    |
| `bindings.forwarder.serviceAccount.name`        | The name of the ServiceAccount to use for the bindings forwarder pod(s).                | `""`                                      |
| `bindings.forwarder.serviceAccount.annotations` | Additional annotations to add to the bindings-forwarder ServiceAccount                  | `{}`                                      |
