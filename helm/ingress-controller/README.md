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

## Values

<!-- TODO: auto generate these via https://github.com/norwoodj/helm-docs -->
| Key | Type | Default | Description |
|-----|------|---------|-------------|
| replicaCount | int | `1` | The number of controllers and agents to run. A minimum of 1 is set for testing purposes but more should be used in production for high availability. |
| image.repository | string | `"ghcr.io/ngrok/ngrok-ingress-controller"` | The image repository to pull from |
| image.tag | string | `"latest"` | The image tag to pull |
| image.pullPolicy | string | `"Never"` | The image pull policy |
| ingressClass | string | `"ngrok"` | The ingress class this controller will satisfy. If not specified, controller will match all ingresses without ingress class annotation and ingresses of type ngrok |
| serverAddr | string | `""` | This is the URL of the ngrok server to connect to. You should set this if you are using a custom ingress URL. |
| region | string | `"us"` | ngrok region to create tunnels in. |
| credentials.secret.name | string | `"ngrok-ingress-controller-credentials"` | The name of the K8S secret that contains the credentials for the ingress controller. |
| credentials.secret.create | bool | `false` | If true, the controller will create the secret with the provided credentials. |
| credentials.apiKey | string | `""` | The ngrok API key to use. If not specified, the controller will use the API key from the credentials secret. |
| credentials.authtoken | string | `""` | The ngrok auth token to use. If not specified, the controller will use the auth token from the credentials secret. |
| resources.limits.cpu | string | `"100m"` | The cpu limit for the controller |
| resources.limits.memory | string | `"128Mi"` | The memory limit for the controller |
| resources.requests.cpu | string | `"10m"` | The cpu request for the controller |
| resources.requests.memory | string | `"64Mi"` | The memory request for the controller |
| podAnnotations | object | `{}` | Annotations to add to the controller pod |