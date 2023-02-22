# ngrok Ingress Controller

This is the helm chart to install the ngrok ingress controller

# Usage

## Prerequisites

The cluster Must be setup with a secret named `ngrok-ingress-controller-credentials` with the following keys:
* NGROK_AUTHTOKEN
* NGROK_API_KEY

## Install the controller with helm

[Helm](https://helm.sh) must be installed to use the charts.  Please refer to
Helm's [documentation](https://helm.sh/docs) to get started.

Once Helm has been set up correctly, add the repo as follows:

`helm repo add ngrok https://ngrok.github.io/kubernetes-ingress-controller`

If you had already added this repo earlier, run `helm repo update` to retrieve
the latest versions of the packages.  You can then run `helm search repo ngrok` to see the charts.

To install the ngrok-ingress-controller chart:

`helm install my-ngrok-ingress-controller ngrok/kubernetes-ingress-controller`

To uninstall the chart:

`helm delete my-ngrok-ingress-controller`

<!-- Parameters are auto generated via @bitnami/readme-generator-for-helm -->
## Parameters

### Common parameters

| Name                | Description                                           | Value |
| ------------------- | ----------------------------------------------------- | ----- |
| `nameOverride`      | String to partially override generated resource names | `""`  |
| `fullnameOverride`  | String to fully override generated resource names     | `""`  |
| `commonLabels`      | Labels to add to all deployed objects                 | `{}`  |
| `commonAnnotations` | Annotations to add to all deployed objects            | `{}`  |


### Controller parameters

| Name                         | Description                                                                                                           | Value                                 |
| ---------------------------- | --------------------------------------------------------------------------------------------------------------------- | ------------------------------------- |
| `podAnnotations`             | Used to inject custom annotations directly into                                                                       | `{}`                                  |
| `replicaCount`               | The number of controllers and agents to run.                                                                          | `1`                                   |
| `image.registry`             | The ngrok ingress controller image registry.                                                                          | `docker.io`                           |
| `image.repository`           | The ngrok ingress controller image repository.                                                                        | `ngrok/kubernetes-ingress-controller` |
| `image.tag`                  | The ngrok ingress controller image tag.                                                                               | `latest`                              |
| `image.pullPolicy`           | The ngrok ingress controller image pull policy.                                                                       | `IfNotPresent`                        |
| `image.pullSecrets`          | An array of imagePullSecrets to be used when pulling the image.                                                       | `[]`                                  |
| `ingressClass.name`          | The name of the ingress class to use.                                                                                 | `ngrok`                               |
| `ingressClass.create`        | Whether to create the ingress class.                                                                                  | `true`                                |
| `ingressClass.default`       | Whether to set the ingress class as default.                                                                          | `false`                               |
| `watchNamespace`             | The namespace to watch for ingress resources. Defaults to all                                                         | `""`                                  |
| `credentials.secret.name`    | The name of the secret the credentials are in. If not provided, one will be generated using the helm release name.    | `""`                                  |
| `credentials.apiKey`         | Your ngrok API key. If provided, it will be will be written to the secret and the authtoken must be provided as well. | `""`                                  |
| `credentials.authtoken`      | Your ngrok authtoken. If provided, it will be will be written to the secret and the apiKey must be provided as well.  | `""`                                  |
| `region`                     | ngrok region to create tunnels in. Defaults to empty to utilize the global network                                    | `""`                                  |
| `serverAddr`                 | This is the URL of the ngrok server to connect to. You should set this if you are using a custom ingress URL.         | `""`                                  |
| `resources.limits`           | The resources limits for the container                                                                                | `{}`                                  |
| `resources.requests`         | The requested resources for the container                                                                             | `{}`                                  |
| `extraVolumes`               | An array of extra volumes to add to the controller.                                                                   | `[]`                                  |
| `extraVolumeMounts`          | An array of extra volume mounts to add to the controller.                                                             | `[]`                                  |
| `extraEnv`                   | an object of extra environment variables to add to the controller.                                                    | `{}`                                  |
| `serviceAccount.create`      | Specifies whether a ServiceAccount should be created                                                                  | `true`                                |
| `serviceAccount.name`        | The name of the ServiceAccount to use.                                                                                | `""`                                  |
| `serviceAccount.annotations` | Additional annotations to add to the ServiceAccount                                                                   | `{}`                                  |

