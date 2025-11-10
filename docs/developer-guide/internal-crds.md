# Internal CRDs

Kubernetes has the concept of [Custom Resource Definitions](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) (CRDs) which allow you to define your own custom resources. This document covers the CRDs created and managed by the controller internally to manage the state of the system across various controller components. It's generally unsafe to modify these directly unless you manually created them
and would likely result in strange effects as the controller fights you. They are useful however to:

* Customize how resources are created in ngrok
* inspect the state of the system
* to debug issues.

We maintain documentation on these CRDs in the [ngrok Docs](https://ngrok.com/docs/k8s/crds).
