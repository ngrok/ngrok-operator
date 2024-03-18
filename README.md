<p>
  <a href="https://ngrok.com">
    <img src="docs/assets/images/ngrok-blue-lrg.png" alt="ngrok Logo" width="300" url="https://ngrok.com" />
  </a>
  <a href="https://kubernetes.io/">
  <img src="docs/assets/images/Kubernetes-icon-color.svg.png" alt="Kubernetes logo" width="150" />
  </a>
</p>

<p>
  <a href="https://github.com/ngrok/kubernetes-ingress-controller/actions?query=branch%3Amain+event%3Apush">
      <img src="https://github.com/ngrok/kubernetes-ingress-controller/actions/workflows/ci.yaml/badge.svg" alt="CI Status"/>
  </a>
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

# ngrok Kubernetes Operator


Leverage [ngrok](https://ngrok.com/) for your ingress in your Kubernetes cluster.  Instantly add load balancing, authentication, and observability to your services via ngrok Cloud Edge modules using Custom Resource Definitions (CRDs) and Kubernetes-native tooling. This repo contains both our [Kubernetes Ingress Controller](https://kubernetes.io/docs/concepts/services-networking/ingress/) and the [Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/)

[Documentation](#documentation) | [Known Issues](#known-issues)

## Documentation

The full documentation for the ngrok Ingress Controller can be found on our [k8s docs](https://ngrok.com/docs/k8s/)

## Known Issues

> **Note** 
>
> This project is currently in beta as we continue testing and receiving feedback. The functionality and CRD contracts may change. It is currently used internally at ngrok for providing ingress to some of our production workloads. 

1. Current issues of concern for production workloads are being tracked [here](https://github.com/ngrok/kubernetes-ingress-controller/issues/208) and [here](https://github.com/ngrok/kubernetes-ingress-controller/issues/219).

## Support

The best place to get support using the ngrok Kubernetes Operator is through the [ngrok Slack Community](https://ngrok.com/slack). If you find bugs or would like to contribute code, please follow the instructions in the [contributing guide](./docs/developer-guide/README.md).

## License

The ngrok ingress controller is licensed under the terms of the MIT license.

See [LICENSE](./LICENSE.txt) for details.
