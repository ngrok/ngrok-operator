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

TODO: These need descriptions. Perhaps we auto generate these via https://github.com/norwoodj/helm-docs
| Key | Type | Default | Description |
|-----|------|---------|-------------|
| replicaCount | int | `2` | The number of controllers and agents to run. A minimum of 2 is recommended in production for HA. |
| image.repository | string | `"ghcr.io/ngrok/ngrok-ingress-controller"` | The image repository to pull from |
| image.tag | string | `"latest"` | The image tag to pull |
| image.pullPolicy | string | `"Never"` | The image pull policy |
| ingressClass | string | `"ngrok"` | The ingress class this controller will satisfy. If not specified, controller will match all ingresses without ingress class annotation and ingresses of type ngrok |
| log | string | `"stdout"` | Agent log destination. |
| region | string | `"us"` | ngrok region to create tunnels in. |
| credentialSecretName | string | `"ngrok-ingress-controller-credentials"` | The name of the K8S secret that contains the credentials for the ingress controller. |
| resources.limits.cpu | string | `"100m"` | The cpu limit for the controller |
| resources.limits.memory | string | `"128Mi"` | The memory limit for the controller |
| resources.requests.cpu | string | `"10m"` | The cpu request for the controller |
| resources.requests.memory | string | `"64Mi"` | The memory request for the controller |
| podAnnotations | object | `{}` | Annotations to add to the controller pod |