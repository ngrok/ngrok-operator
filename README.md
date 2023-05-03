<p align="center">
  <a href="https://ngrok.com">
    <img src="docs/assets/images/ngrok-blue-lrg.png" alt="ngrok Logo" width="500" url="https://ngrok.com" />
  </a>
  <a href="https://kubernetes.io/">
  <img src="docs/assets/images/Kubernetes-icon-color.svg.png" alt="Kubernetes logo" width="250" />
  </a>
</p>

<p align="center">
  <a href="https://github.com/ngrok/kubernetes-ingress-controller/actions?query=branch%3Amain+event%3Apush">
      <img src="https://github.com/ngrok/kubernetes-ingress-controller/actions/workflows/ci.yaml/badge.svg" alt="CI Status"/>
  </a>
  <!-- TODO: Add badges for things like docker build status, image pulls, helm build status, latest stable release version, etc -->
</p>
<p align="center">
  <a href="https://github.com/ngrok/kubernetes-ingress-controller/blob/master/LICENSE">
    <img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License"/>
  </a>
  <a href="#features-and-beta-status">
    <img src="https://img.shields.io/badge/Status-Beta-orange.svg" alt="Status"/>
  </a>
  <a href="https://ngrok.com/slack">
    <img src="https://img.shields.io/badge/Join%20Our%20Community-Slack-blue" alt="Slack"/>
  </a>
  <a href="https://twitter.com/intent/follow?screen_name=ngrokHQ">
    <img src="https://img.shields.io/twitter/follow/ngrokHQ.svg?style=social&label=Follow" alt="Twitter"/>
  </a>
</p>

> _*Warning*_: Currently this project has an issue being tracked [here](https://github.com/ngrok/kubernetes-ingress-controller/issues/208) which can cause the controller to provide ingress to a service even if it failed to configure an authentication module like Oauth on a particular route due to a configuration or intermittent error. Until this issue is resolved, it's not recommended to use this with security sensitive applications.

# ngrok Ingress Controller for Kubernetes

ngrok is a simplified API-first ingress-as-a-service that adds connectivity, security, and observability to your apps and services.

The ngrok Ingress Controller for Kubernetes is an open source controller for adding public and secure ingress traffic to your K8s services. If you’ve used ngrok in the past, you can think of the ingress controller as the ngrok agent built as an idiomatic K8s resource — available as a helm chart, configurable via K8s manifests, scalable for production usage, and leveraging Kubernetes best practices.

The ngrok Ingress Controller for Kubernetes lets developers define public and secure ingress traffic to their K8s resources directly from the deployment manifest, without configuring low-level network primitives — like DNS, IPs, NAT, and VPCs — outside of their K8s cluster. This makes it easy to add global traffic with security and scalability into K8s resources regardless of the underlying network infrastructure.

For more details on the internal architecture, see [here](https://github.com/ngrok/kubernetes-ingress-controller/blob/main/docs/developer-guide/README.md).

## Installation

> As of today, the ngrok Ingress Controller works only on ngrok accounts with the Pro subscription or above. If you would like to use the ngrok Ingress Controller with a free ngrok account, please [reach us out in our slack community](https://ngrok.com/slack) and we will be happy to help.

The ngrok ingress controller is available as a helm chart. To add the ngrok helm chart repository, run:

```bash
helm repo add ngrok https://ngrok.github.io/kubernetes-ingress-controller
```

To install the latest version of the ngrok ingress controller, run the following command (setting the appropriate values for your environment):

```bash
export NAMESPACE=[YOUR_K8S_NAMESPACE]
export NGROK_AUTHTOKEN=[AUTHTOKEN]
export NGROK_API_KEY=[API_KEY]
helm install ngrok-ingress-controller ngrok/kubernetes-ingress-controller \
  --namespace $NAMESPACE \
  --create-namespace \
  --set credentials.apiKey=$NGROK_API_KEY \
  --set credentials.authtoken=$NGROK_AUTHTOKEN
```

**Note:** The `NGROK_API_KEY` and `NGROK_AUTHTOKEN` are used by your ingress controller to authenticate with ngrok for configuring and running your network ingress traffic at the edge. You can find these values in your [ngrok dashboard](https://dashboard.ngrok.com/get-started/setup).

## Documentation

You can find the full documentation for the ngrok ingress controller [here](./docs/README.md). You can also visit our comprehensive [get started tutorial in ngrok's official docs](https://ngrok.com/docs/using-ngrok-with/k8s/).

For more in depth guides, see:
- [Deployment Guide](./docs/deployment-guide/README.md): for installing the controller for the first time
- [User Guide](./docs/user-guide/README.md): for an in depth view of the ngrok ingress configuration options and primitives
- [Examples](./docs/examples/README.md): for examples of how to configure ingress in different scenarios (e.g. Hello World, Consul, OAuth, etc.)
- [Developer Guide](./docs/developer-guide/README.md): for those interested in contributing to the project

## Quickstart

> As of today, the ngrok Ingress Controller works only on ngrok accounts with the Pro subscription or above. If you would like to use the ngrok Ingress Controller with a free ngrok account, please [reach us out in our slack community](https://ngrok.com/slack) and we will be happy to help.

For a quick start, apply the [sample combined manifest](manifest-bundle.yaml) from our repo:

```bash
kubectl apply -n ngrok-ingress-controller -f https://raw.githubusercontent.com/ngrok/kubernetes-ingress-controller/main/manifest-bundle.yaml
```

For a comprehensive, step-by-step quick start, check our [get started guide in ngrok's official docs](https://ngrok.com/docs/using-ngrok-with/k8s/).


## Known Limitations

1. This project is currently in beta as we continue testing and receiving feedback. The functionality and CRD contracts may change. It is currently used internally at ngrok for providing ingress to some of our production workloads.

## Support

The best place to get support using the ngrok Ingress Controller is through the [ngrok Slack Community](https://ngrok.com/slack). If you find bugs or would like to contribute code, please follow the instructions in the [contributing guide](./docs/developer-guide/README.md).

## License

The ngrok ingress controller is licensed under the terms of the MIT license.

See [LICENSE](./LICENSE.txt) for details.