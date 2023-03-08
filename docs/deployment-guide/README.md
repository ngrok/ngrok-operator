# Deployment Guide

This guide is meant as the starting point for configuring, deploying, and operating the controller component itself. It focuses on configurations and settings for the whole controller, and thus its corresponding ingress class, rather than individual ingress resources and their annotations.


## Prerequisites
- An ngrok account - Currently, the controller only works with paid accounts. We are working on a free tier settings that will work with the controller.
- A k8s cluster and access to it via kubectl - We recommend using a recent version of k8s and will specify and test past versions as a part of https://github.com/ngrok/kubernetes-ingress-controller/issues/154
- helm - 3.0.0 or later.

## Installation

It is recommended to use helm to install the controller. Alternatively, the container is available on docker hub at `ngrok/ingress-controller` and can be ran directly with hand crafted manifests.

To install via helm, run the following command to export your credentials as environment variables and install the controller:

```bash
export NGROK_API_KEY=<YOUR Secret API KEY>
export NGROK_AUTHTOKEN=<YOUR Secret Auth Token>

helm repo add ngrok https://ngrok.github.io/kubernetes-ingress-controller
helm install ngrok-ingress-controller ngrok/kubernetes-ingress-controller \
  --namespace ngrok-ingress-controller \
  --create-namespace \
  --set credentials.apiKey=$(NGROK_API_KEY) \
  --set credentials.authtoken=$(NGROK_AUTHTOKEN)
```

For a more robust _infrastructure as code_ way of passing your credentials, see the [credentials](./credentials.md#setup) page.

Once installed and healthy, you can try creating an ingress object using the output example from the helm install!

### Alternatively

For a quick install, you can also use the combined manifests directly from the repo and begin changing resources:

```bash
kubectl apply -n ngrok-ingress-controller -f https://raw.githubusercontent.com/ngrok/kubernetes-ingress-controller/main/manifest-bundle.yaml
```

## Known Limits

A limitation is that it doesn't work with free accounts. Additionally there are soft limitation in place we hit that we still need to stress test and document and look at changing.


## Other Topics

Below are various other topics for a deployer or operator to be aware of.
- [credentials](./credentials.md)
- [common helm overrides](./common-helm-k8s-overrides.md)
- [multiple installations](./multiple-installations.md)
- [white label agent ingress](./white-label-agent-ingress.md)
- [metrics](./metrics.md)
- [ngrok regions](./ngrok-regions.md)
