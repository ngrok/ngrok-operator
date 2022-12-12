# Ngrok Ingress Controller

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

`helm repo add ngrok https://ngrok.github.io/ngrok-ingress-controller`

If you had already added this repo earlier, run `helm repo update` to retrieve
the latest versions of the packages.  You can then run `helm search repo ngrok` to see the charts.

To install the ngrok-ingress-controller chart:

`helm install my-ngrok-ingress-controller ngrok/ngrok-ingress-controller`

To uninstall the chart:

`helm delete my-ngrok-ingress-controller`

<!-- Parameters are auto generated via @bitnami/readme-generator-for-helm -->
## Parameters

### Controller parameters

| Name                     | Description                                     | Value                                  |
| ------------------------ | ----------------------------------------------- | -------------------------------------- |
| `podAnnotations`         | Used to inject custom annotations directly into | `{}`                                   |
| `replicaCount`           | The number of controllers and agents to run.    | `2`                                    |
| `image.repository`       | The ngrok ingress controller image repository.  | `ngrok/ngrok-ingress-controller`       |
| `image.tag`              | The ngrok ingress controller image tag.         | `latest`                               |
| `image.pullPolicy`       | The ngrok ingress controller image pull policy. | `IfNotPresent`                         |
| `ingressClass`           | The ingress class this controller will satisfy. | `ngrok`                                |
| `log`                    | Agent log destination.                          | `stdout`                               |
| `region`                 | ngrok region to create tunnels in.              | `us`                                   |
| `credentialsSecret.name` | The name of the K8S secret that contains the    | `ngrok-ingress-controller-credentials` |
| `apiKey`                 | The ngrok API key to use                        | `""`                                   |
| `authtoken`              | The ngrok auth token to use                     | `""`                                   |
| `resources.limits`       | The resources limits for the container          | `{}`                                   |
| `resources.requests`     | The requested resources for the container       | `{}`                                   |

