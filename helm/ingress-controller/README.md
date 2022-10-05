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
| replicaCount | int | `2` |  |
| image.repository | string | `"ghcr.io/ngrok/ngrok-ingress-controller"` |  |
| image.pullPolicy | string | `"Never"` |  |
| image.tag | string | `"latest"` |  |
| ingressClass | string | `"ngrok"` |  |
| log | string | `"stdout"` |  |
| region | string | `"us"` |  |
| credentialSecretName | string | `"ngrok-ingress-controller-credentials"` |  |
| resources.limits.cpu | string | `"100m"` |  |
| resources.limits.memory | string | `"128Mi"` |  |
| resources.requests.cpu | string | `"10m"` |  |
| resources.requests.memory | string | `"64Mi"` |  |
| podAnnotations | object | `{}` |  |