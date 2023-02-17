# CRDs

Kubernetes has the concept of [Custom Resource Definitions](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) (CRDs) which allow you to define your own custom resources. The ngrok Kubernetes Ingress Controller uses CRDs internally to represent the collection of ingress objects and other k8s resources as ngrok Edges and other resources which it synchronizes to the API. While all resources can be accessed via the k8s API, we don't recommend editing the internal resources directly. They are however useful to inspect and query the state of the system. This document will go over all the CRDs and note the ones that are internal implementations for now. These API differences will be more clear when the controller moves the primary resources out of alpha, and the internal CRDs remain as alpha for flexibility.

## IP Policies

The `IPPolicy` CRD manages the ngrok [API resource](https://ngrok.com/docs/api/resources/ip-policies) directly. It is a first class CRD that you can manage to control these policies in your account.

TODO: Add markdown table of the crd

Its optional to create IP Policies this way vs using the ngrok dashboard or [terraform provider](https://registry.terraform.io/providers/ngrok/ngrok/latest/docs/resources/ip_policy). Once created though, you can use it in your ingress objects using the [annotations](./user-guide/annotations.md#ip-restriction).

## Domains

Domains are automatically created by the controller based on the ingress objects host values. Standard ngrok subdomains will automatically be created and reserved for you. Custom domains will also be created and reserved, but will be up to you to configure the DNS records for them. See the [custom domain](./user-guide/custom-domain.md) guide for more details.

If you delete all the ingress objects for a particular host, as a saftey precaution, the ingress controller does *NOT* delete the domains and thus does not un-register them. This ensures you don't lose domains while modifying or recreating ingress objects. You can still manually delete a domain CRD via `kubectl delete domain <name>` if you want to un-register it.

TODO: Add markdown table of the crd

## Tunnels

Tunnels are automatically created by the controller based on the ingress objects' rules' backends. A tunnel will be created for each backend service name and port combination. This results in tunnels being created with those labels which can be matched by various edge backends. Tunnels are useful to inspect but are fully managed by the controller and should not be edited directly.

TODO: Add markdown table of the crd

## HTTPS Edges

HTTPS Edges are the primary representation of all the ingress objects and various configuration's states that will be reflected to the ngrok API. While you could create https edge CRDs directly, its not recommended because:
- the api is internal and will likely change in the future
- if your edge conflicts with any edge managed by the controller, it may be overwritten

This may stabilize to a first class CRD in the future, but for now, its not recommended to use directly but may be useful to inspect the state of the system.

TODO: Add markdown table of the crd

## TCP Edges

The Kubernetes ingress spec does not directly support TCP traffic. The ngrok Kubernetes Ingress Controller supports TCP traffic via the [TCP Edge](https://ngrok.com/docs/api#tcp-edge) resource. This is a first class CRD that you can manage to control these edges in your account.

TODO: Add markdown table of the crd