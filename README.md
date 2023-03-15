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



# ngrok Ingress Controller

This is a general purpose [kubernetes ingress controller](https://kubernetes.io/docs/concepts/services-networking/ingress-controllers/) provides to workloads running in a kubernetes cluster with a public URL via [ngrok](https://ngrok.com/). It dynamically provisions and deprovisions multiple highly available ngrok [tunnels](https://ngrok.com/docs/secure-tunnels#labeled-tunnels) and [edges](https://ngrok.com/docs/secure-tunnels#integrating-with-cloud-edge) as ingress resources are created and deleted. Take a guided tour through the architecture [here](https://s.icepanel.io/tPjIPc8Ifg/kj7w).

## Documentation

[Documentation](./docs/README.md)

## Project Status: Alpha

This project is currently in alpha status as we continue testing and receiving feedback. The functionality and CRD contracts may change. It is currently used internally at ngrok for providing ingress to some of our production workloads.


[ngrok-url]: https://ngrok.com
[ngrok-logo]: ./docs/images/ngrok-blue-lrg.png
