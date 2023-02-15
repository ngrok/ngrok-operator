# Deployment Guide

This is meant for those deploying and operating the controller itself.


## Prerequisites
- account (no free tier yet).
- k8s cluster (with supported version)
- helm

## Installation

Recommended to use helm to install the controller.

## Known Limits

A limitation is that it doesn't work with free accounts. Additionally there are soft limitation in place we hit that we still need to stress test and document and look at changing.


## Other Topics

Below are various other topics for a deployer or operator to be aware of.
- [rbac](./rbac.md)
- [ingress class](./ingress-class.md)
- [ngrok regions](./ngrok-regions.md)
- [multiple installations](./multiple-installations.md)
- [watching specific namespaces](./single-namespace.md)
- [common helm overrides](./common-helm-k8s-overrides.md)
- [credentials](./credentials.md)
- [white label ingress](./white-label-ingress.md)
- [metrics](./metrics.md)

