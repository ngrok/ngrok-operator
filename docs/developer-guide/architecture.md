<!-- omit in toc -->
# ngrok Ingress Controller Architecture and Design

The ngrok Operator is a series of Kubernetes style control loops that watch Ingresses, Services, and other k8s resources. It manages tunnels in your cluster and the ngrok API to provide public endpoints for your Kubernetes services leveraging ngrok's edge features. This page is meant to be a living document detailing noteworthy architecture notes and design decisions. This document should help:
- Consumers of the controller understand how it works
- Contributors to the controller understand how to make changes to the controller and plan for future changes
- Integration Partners can understand how it works to see how we can integrate together.

Before we jump directly into ngrok controller's specific architecture, let's first go over some core concepts about what controllers are and how they are built.

- [What is a controller](#what-is-a-controller)
  - [Controllers](#controllers)
    - [Tunnel Controller](#tunnel-controller)
    - [Ingress Controller](#ingress-controller)
- [Architecture](#architecture)
  - [Overview](#overview)
  - [Agent](#agent)
  - [API](#api)
  - [Bindings](#bindings)


## What is a controller

The word _controller_ can sometimes be used in ways that can be confusing. While we commonly refer to the whole thing as the ingress _controller_, in reality, many _controllers_ are actually a Controller Runtime Manager which runs multiple individual [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/) and provides common things to them like shared Kubernetes client caches and leader election.

![Kubebuilder Architecture Diagram](../assets/images/kubebuilder_architecture_diagram.svg)

> From: https://book.kubebuilder.io/architecture.html

Individual controllers and the overall Manager are built using the kubernetes controller-runtime library which k8s itself leverages for its internal controllers (such as Deployment or StatefulSet controllers). Its go-docs explain each concept fairly well starting here https://pkg.go.dev/sigs.k8s.io/controller-runtime#hdr-Managers

### Controllers

Internally, the ngrok Kubernetes Operator is made up of multiple controllers working in concert with each other, communicating via the Kubernetes API to interpret Ingress objects and convert them into managed ngrok Edges and other resources.

Each of these controllers uses the same basic workflow to manage its resources. This will be dried up and documented as a part of [this issue](https://github.com/ngrok/ngrok-operator/issues/118)

The following controllers for the most part manage a single resource and reflect those changes in the ngrok API.
- [IP Policy Controller](../../internal/controller/ingress/ippolicy_controller.go): It simply watches these CRDs and reflects the changes in the ngrok API.
- [Domain Controller](../../internal/controller/ingress/domain_controller.go): It will watch for domain CRDs and reflect those changes in the ngrok API. It will also update the domain CRD objects' status fields with the current state of the domain in the ngrok API, such as a CNAME target if it's a white label domain.
- [HTTPS Edge Controller](../../internal/controller/ingress/httpsedge_controller.go): This CRD contains all the data necessary to build not just the edge, but also all routes, backends, and route modules by calling various ngrok APIs to combine resources. The HTTPSEdge CRD is the common type other controllers can create based on different source inputs like Ingress objects or Gateway objects.

The following controllers are more complex and manage multiple resources and reflect those changes in the ngrok API.

#### Tunnel Controller

All of the controllers except this tunnel controller use the controller-runtime's Leader Election process so when multiple instances of the controller only 1 is set up to actually try to call the ngrok api to prevent multiple pods from fighting with each other. The tunnel controller is the only controller that does not use this leader election and instead this controller runs in all pods, even non-leaders. This is because this controller is meant to read the Tunnel CRDs created by the ingress controller, and to dynamically manage a list of tunnels using the [ngrok-go](https://github.com/ngrok/ngrok-go) library. It creates these tunnels using labels specified on the Tunnel CRD so they should match an edge's backend created by the ingress controller.


#### Ingress Controller

The ingress controller is the primary piece of functionality in the overall project right now. It is meant to watch Ingress objects and CRDs used by those objects like IPPolicies, NgrokModuleSets, or even secrets.

TODO: Update more about the various pieces of the ingress controller portion such as the store, the driver, how annotations work, etc.

<img src="../assets/images/Under-Construction-Sign.png" alt="Under Construction" width="350" />

## Architecture

### Overview

Previously, in the ngrok Ingress Controller, the Tunnel Controller(non leader-elected) ran under the same manager as the rest of the API controllers(leader-elected). This did not allow users to independetly scale the data forwarding and API management features. Going forward, we will be splitting the Tunnel Controller into its own manager so that it can be scaled independently of the rest of the controllers. This will allow users to scale the data forwarding and API management features independently.

### Agent

The agent manager is a non-leader elected manager that runs the tunnel controller. Each replica of the agent manager watches the tunnel CRs and creates or deletes tunnels based on their state. Each agent pod creates a new agent(tunnel session) which you can view in the ngrok dashboard [here](https://dashboard.ngrok.com/agents) or with the ngrok CLI by running `ngrok api tunnel-sessions list`.


### API

The API manager is a leader elected manager that runs the following controllers based on configuration:

* Ingress Controller
* Domain Controller
* IP Policy Controller
* HTTPS Edge Controller
* TLS Edge Controller
* Service Controller
* NgrokModuleSet Controller
* Gateway Controller

### Bindings

TODO: Add more information about the bindings manager.
