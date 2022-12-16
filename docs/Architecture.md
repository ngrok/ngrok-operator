# ngrok Ingress Controller Architecture and Design
The ngrok Ingress Controller is a series of Kubernetes style control loops that watch Ingress and other k8s resources and manage agent and the ngrok API to provide public endpoints for your Kubernetes services using ngrok's features. This page is meant to be a living document detailing noteworthy architecture notes and design decisions. This document should help:
- Consumers of the controller understand how it works
- Contributors to the controller understand how to make changes to the controller and plan for future changes
- Integration Partners can understand how it works to see how we can integrate together.

Before we jump directly into ngrok controller's specific architecture, lets first go over some core concepts about what controllers are and how they are built.

## Background Information
- Good docs https://book.kubebuilder.io/architecture.html
- a controller is fundamentally just something that watches a k8s resource and does some reconciliation logic with it to reach some eventual consistency
  - https://book.kubebuilder.io/cronjob-tutorial/controller-overview.html
  - https://kubernetes.io/docs/concepts/architecture/controller/#:~:text=In%20Kubernetes%2C%20controllers%20are%20control,closer%20to%20the%20desired%20state.
- controllers are built with the controller-runtime library https://github.com/kubernetes-sigs/controller-runtime which comes with a bunch of helpers
- controller manager will let you create multiple controller instances in 1 binary and they get shares k8s api client caches and watchers and stuff
- Copy in the definition of the manager, controller, and reconciler from the kubebuilder docs https://pkg.go.dev/sigs.k8s.io/controller-runtime#hdr-Managers

## Our Architecture
- Fundamentally the controller needs to manage tunnels via agents in the cluster and it needs to configure some ngrok api resources for the tunnels for those agents
- Today thats built by running 2 controllers in 1 binary with an agent running as a sidecar container
  - Tunnel Controller
    - watches for Ingress resources and manages the agent running as a sidecar via an http endpoint on localhost:4040
  - Ngrok API Controller
    - watches for ngrok api resources and manages the ngrok api resources
- 1 controller runtime manager runs these 2 controllers where the tunnel controller runs in every pod and the ingress controller only runs in the leader pod
- this means you can scale
- Both have a lot of shared functions they use because they fundamentally do a similar thing of watching ingress objects, handling cases like missing ingress objects or ingress classes, and converting them into some action on the ngrok side
- Add in diagram from the icepanel doc
- some sort of class diagram or something to show the separation of the code

### Tunnel Controller
- watches for Ingress resources and manages the agent running as a sidecar via an http endpoint on localhost:4040
  - document out how labels are set
- the plan has been to migrate this to ngrok-go hidden behind the existing interface
- because it needs to run in all the pods so we have multiple agents running labeled tunnels for redundancy, we have to override some stuff so it doesn't use the primary manager's leader election config.

### Ingress Controller
- similarly watches for Ingress resources and manages the ngrok api resources
- today its not very robust when it comes to failures or picking up where it left off. It doesn't even support updates yet. We should discuss if this is the route we want to go before putting significant effort into
* how ngrok api resources are managed (details about each resource)
  * 1 ingress == 1 edge
  * k8s object updates (status, finalizers, edge-id, etc)
  - explain how labels are set
  - explain how domains get reserved
  - hostports on other edges cause conflicts

## Current Known limits

- number of tunnels per agent and number of agents per account
- paid vs free accounts

