<p align="center">
  <a href="https://ngrok.com">
    <img src="docs/images/ngrok-blue-lrg.png" alt="Ngrok Logo" width="500" url="https://ngrok.com" />
  </a>
  <a href="https://kubernetes.io/">
  <img src="docs/images/Kubernetes-icon-color.svg.png" alt="Kubernetes logo" width="250" />
  </a>
</p>

<p align="center">
  <a href="https://github.com/ngrok/ngrok-ingress-controller/actions?query=branch%3Amain+event%3Apush">
      <img src="https://github.com/ngrok/ngrok-ingress-controller/actions/workflows/ci.yaml/badge.svg" alt="CI Status"/>
  </a>
  <!-- TODO: Add badges for things like docker build status, image pulls, helm build status, latest stable release version, etc -->
</p>
<p align="center">
  <a href="https://github.com/ngrok/ngrok-ingress-controller/blob/master/LICENSE">
    <img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License"/>
  </a>
  <a href="#features-and-alpha-status">
    <img src="https://img.shields.io/badge/Status-Alpha-orange.svg" alt="Status"/>
  </a>
  <a href="https://ngrok.com/slack">
    <img src="https://img.shields.io/badge/Join%20Our%20Community-Slack-blue" alt="Slack"/>
  </a>
  <a href="https://twitter.com/intent/follow?screen_name=ngrokHQ">
    <img src="https://img.shields.io/twitter/follow/ngrokHQ.svg?style=social&label=Follow" alt="Twitter"/>
  </a>
</p>



# Ngrok Ingress Controller

This is a general purpose [kubernetes ingress controller](https://kubernetes.io/docs/concepts/services-networking/ingress-controllers/) provides to workloads running in a kubernetes cluster with a public URL via [ngrok](https://ngrok.com/). It dynamically provisions and deprovisions multiple highly available Ngrok [tunnels](https://ngrok.com/docs/secure-tunnels#labeled-tunnels) and [edges](https://ngrok.com/docs/secure-tunnels#integrating-with-cloud-edge) as ingress resources are created and deleted. Take a guided tour through the architecture [here](https://s.icepanel.io/tPjIPc8Ifg/kj7w).

## Features and Alpha Status

This project is currently in alpha status. It is not yet recommended for production use. The following features are currently supported:
* Create, update, and delete ingress objects and have their corresponding tunnels and edges to be updated in response.
* Install via Helm
* Supports multiple routes, BUT ONLY ONE host per ingress object at this time.
* MUST have a pro account to use this controller. The controller will not work with a free account right now as it requires the usage of Ngrok Edges.

### Looking Forward

An official roadmap is coming soon. In the meantime, here are some of the features we are working on:
* Stability and HA testing and improvements. Especially during ingress updates, or controller rollouts.
* Support for multiple hosts per ingress object.
* Support for all of Ngrok's Edge Modules such as [Oauth](https://ngrok.com/docs/api#api-edge-route-o-auth-module)
* Free tier support


## Getting Started

Get your [Ngrok API Key](https://ngrok.com/docs/api#authentication) and [Ngrok Auth Token](https://dashboard.ngrok.com/) and export them as environment variables

  ```bash
export NGROK_API_KEY=<YOUR Secret API KEY>
export NGROK_AUTHTOKEN=<YOUR Secret Auth Token>
  ```
Install via Helm:

```bash
helm repo add ngrok https://ngrok.github.io/ngrok-ingress-controller
helm install ngrok-ingress-controller ngrok/ngrok-ingress-controller \
  --namespace ngrok-ingress-controller \
  --create-namespace \
  --set credentials.secret.create = true \
  --set credentials.apiKey=$(NGROK_API_KEY) \
  --set credentials.authtoken=$(NGROK_AUTHTOKEN)
```

See [Helm Chart](./helm/ingress-controller/README.md#install-the-controller-with-helm) for more details.

## Documentation

<!-- TODO: https://ngrok.com/docs/ngrok-ingress-controller -->
  <img src="./docs/images/Under-Construction-Sign.png" alt="Kubernetes logo" width="350" />


## Local Development

Ensure you have the following prerequisites installed:

* [Go 1.19](https://go.dev/dl/)
* [Helm](https://helm.sh/docs/intro/install/)
* A k8s cluster is available via your kubectl client. This can be a remote cluster or a local cluster like [minikube](https://minikube.sigs.k8s.io/docs/start/)
  * NOTE: Depending on your cluster, you may have to take additional steps to make the image available. For example with minikube, you may need to run `eval $(minikube docker-env)` to make the image available to the cluster.

### Setup

```sh
export NGROK_API_KEY=<YOUR Secret API KEY>
export NGROK_AUTHTOKEN=<YOUR Secret Auth Token>
# kubectl can connect to your cluster and images built locally are available to the cluster
kubectl create namespace ngrok-ingress-controller
kubectl config set-context --current --namespace=ngrok-ingress-controller
kubectl create secret generic ngrok-ingress-controller-credentials \
  --from-literal=API_KEY=$NGROK_API_KEY  \
  --from-literal=AUTHTOKEN=$NGROK_AUTHTOKEN

make deploy
```

### Auth and Credentials

The controller assumes a k8s secret named `ngrok-ingress-controller-credentials` exists with the following keys:
  * AUTHTOKEN
  * API_KEY

The name can technically be changed via a helm value, but for now its required while we still use ngrok Cloud Edges and Edges are a pro feature.

Example:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: ngrok-ingress-controller-credentials
  namespace: ngrok-ingress-controller
data:
  API_KEY: "YOUR-API-KEY-BASE64"
  AUTHTOKEN: "YOUR-AUTHTOKEN-BASE64"
```

## How to Configure the Agent

> Warning: This will be deprecated soon when moving to the new lib-ngrok library
* assumes configs will be in a config map named `ngrok-ingress-controller-agent-cm` in the same namespace
* setup automatically via helm. Values and config map name can be configured in the future via helm
* subset of these that should be configurable https://ngrok.com/docs/ngrok-agent/config#config-full-example
* example config map showing all optional values with their defaults.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: ngrok-ingress-controller-agent-cm
  namespace: ngrok-ingress-controller
data:
  METADATA: "{}"
  REGION: us
  REMOTE_MANAGEMENT: true
```

## Using the Examples
Several examples are provided in the [`examples` folder](./examples).  To use an example, make a copy of the included `EXAMPLE*config.yaml` in the same directory, like this:
- `cp examples/hello-world-ingress/EXAMPLE-config.yaml examples/hello-world-ingress/config.yaml`
- `cp examples/ingress-class/EXAMPLE-config-different.yaml examples/ingress-class/config-different.yaml`

Then, you need to update the `value` field in that new file.

You can then apply the given example via `kubectl apply -k examples/<example in question>`, i.e.
`kubectl apply -k examples/hello-world-ingess`.

## E2E Tests

If you run the script `./scripts/e2e.sh` it will run the e2e tests against your current kubectl context. These tests tear down any existing ingress controller and examples, re-installs them, and then runs the tests. It creates a set of different ingresses and verifies that they all behave as expected

[ngrok-url]: https://ngrok.com
[ngrok-logo]: ./docs/images/ngrok-blue-lrg.png
