<!-- primary links -->
<p>
  <a href="https://ngrok.com">
    <img src="docs/assets/images/ngrok-blue-lrg.png" alt="ngrok Logo" width="300" url="https://ngrok.com" />
  </a>
  <a href="https://kubernetes.io/">
  <img src="docs/assets/images/Kubernetes-icon-color.svg.png" alt="Kubernetes logo" width="150" />
  </a>
</p>

<!-- badges -->
<p>
  <a href="https://github.com/ngrok/ngrok-operator/actions?query=branch%3Amain+event%3Apush">
      <img src="https://github.com/ngrok/ngrok-operator/actions/workflows/ci.yaml/badge.svg" alt="CI Status"/>
  </a>
  <a href="https://github.com/ngrok/ngrok-operator/blob/master/LICENSE">
    <img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License"/>
  </a>
  <a href="#features-and-beta-status">
    <img src="https://img.shields.io/badge/Status-Beta-orange.svg" alt="Status"/>
  </a>
   <a href="#gateway-api-status">
    <img src="https://img.shields.io/badge/Gateway_API-preview-rgba(159%2C122%2C234)" alt="Gateway API Preivew"/>
  </a>
  <a href="https://ngrok.com/slack">
    <img src="https://img.shields.io/badge/Join%20Our%20Community-Slack-blue" alt="Slack"/>
  </a>
  <a href="https://twitter.com/intent/follow?screen_name=ngrokHQ">
    <img src="https://img.shields.io/twitter/follow/ngrokHQ.svg?style=social&label=Follow" alt="Twitter"/>
  </a>
  <a href="https://artifacthub.io/packages/search?repo=ngrok&operators=true&sort=relevance&page=1">
    <img src="https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/ngrok" alt="Artifacthub"/>
  </a>
    <a href="https://github.com/ngrok/ngrok-operator/actions/workflows/trivy-image-scan.yaml">
    <img src="https://github.com/ngrok/ngrok-operator/actions/workflows/trivy-image-scan.yaml/badge.svg" alt="Trivy"/>
  </a>
</p>

# ngrok Kubernetes Operator


Leverage [ngrok](https://ngrok.com/) for your ingress in your Kubernetes cluster.  Instantly add load balancing, authentication, and observability to your services via ngrok Cloud Edge modules using Custom Resource Definitions (CRDs) and Kubernetes-native tooling. This repo contains both our [Kubernetes Ingress Controller](https://kubernetes.io/docs/concepts/services-networking/ingress/) and the [Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/)


[Installation](#installation) | [Getting Started](https://ngrok.com/docs/using-ngrok-with/k8s/) | [Documentation](#documentation) | [Developer Guide](https://github.com/ngrok/ngrok-operator/blob/main/docs/developer-guide/README.md) | [Known Issues](#known-issues)

## Installation

### Helm

> **Note** We recommend using the Helm chart to install the operator for a better upgrade experience.

Add the ngrok Kubernetes Operator Helm chart:

```sh
helm repo add ngrok https://charts.ngrok.com
```

Then, install the latest version (setting the appropriate values for your environment):

```sh
export NAMESPACE=[YOUR_K8S_NAMESPACE]
export NGROK_AUTHTOKEN=[AUTHTOKEN]
export NGROK_API_KEY=[API_KEY]

helm install ngrok-operator ngrok/ngrok-operator \
  --namespace $NAMESPACE \
  --create-namespace \
  --set credentials.apiKey=$NGROK_API_KEY \
  --set credentials.authtoken=$NGROK_AUTHTOKEN
```

> ** Note ** The values for `NGROK_API_KEY` and `NGROK_AUTHTOKEN` can be found in your [ngrok dashboard] (https://dashboard.ngrok.com/get-started/setup). The ngrok Kubernetes Operator uses them to authenticate with ngrok and configure and run your network ingress traffic at the edge.

For a more in-depth installation guide follow our step-by-step [Getting Started](https://ngrok.com/docs/using-ngrok-with/k8s/) guide.

#### Gateway API Preview

To install the developer preview of the gateway api we'll make the following changes to the above instructions.

Install the v1 gateway CRD before the helm installation.
```sh
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.0.0/standard-install.yaml
```

Then, during the helm install set the experimental gateway flag.

```sh
helm install ngrok-operator ngrok/ngrok-operator \
  --namespace $NAMESPACE \
  --create-namespace \
  --set credentials.apiKey=$NGROK_API_KEY \
  --set credentials.authtoken=$NGROK_AUTHTOKEN \
  --set useExperimentalGatewayApi=true  # gateway preview
```
### YAML Manifests

Apply the [sample combined manifest](manifest-bundle.yaml) from our repo:

```sh
kubectl apply -n ngrok-operator \
  -f https://raw.githubusercontent.com/ngrok/ngrok-operator/main/manifest-bundle.yaml
```

For a more in-depth installation guide follow our step-by-step [Getting Started](https://ngrok.com/docs/using-ngrok-with/k8s/) guide.

## Documentation

The full documentation for the ngrok Kubernetes Operator can be found on our [k8s docs](https://ngrok.com/docs/k8s/)

## Known Issues

> **Note**
>
> This project is currently in beta as we continue testing and receiving feedback. The functionality and CRD contracts may change. It is currently used internally at ngrok for providing ingress to some of our production workloads.

1. Current issues can be found in the GitHub issues. [Known/suspected bugs](https://github.com/ngrok/ngrok-operator/issues?q=is%3Aopen+is%3Aissue+label%3Abug) are labeled as `bug`.

## Support

The best place to get support using the ngrok Kubernetes Operator is through the [ngrok Slack Community](https://ngrok.com/slack). If you find bugs or would like to contribute code, please follow the instructions in the [contributing guide](./docs/CONTRIBUTING.md).

## License

The ngrok Kubernetes Operator is licensed under the terms of the MIT license.

See [LICENSE](./LICENSE.txt) for details.


## This is a test of the pr-size labeler

A

