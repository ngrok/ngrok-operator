# ngrok Ingress Controller Architecture and Design
The ngrok Ingress Controller is a series of Kubernetes style control loops that watch Ingress and other k8s resources and manage tunnels in your cluster and the ngrok API to provide public endpoints for your Kubernetes services leveraging ngrok's edge features. This page is meant to be a living document detailing noteworthy architecture notes and design decisions. This document should help:
- Consumers of the controller understand how it works
- Contributors to the controller understand how to make changes to the controller and plan for future changes
- Integration Partners can understand how it works to see how we can integrate together.

Before we jump directly into ngrok controller's specific architecture, let's first go over some core concepts about what controllers are and how they are built.

# What is a controller

The word _controller_ can sometimes be used in ways that can be confusing. While we commonly refer to the whole thing as the ingress _controller_, in reality, many _controllers_ are actually a Controller Runtime Manager which runs multiple individual [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/) and provides common things to them like shared Kubernetes client caches and leader election.

![Kubebuilder Architecture Diagram](../assets/images/kubebuilder_architecture_diagram.svg)

> From: https://book.kubebuilder.io/architecture.html

Individual controllers and the overall Manager are built using the kubernetes controller-runtime library which k8s itself leverages for its internal controllers (such as Deployment or StatefulSet controllers). Its go-docs explain each concept fairly well starting here https://pkg.go.dev/sigs.k8s.io/controller-runtime#hdr-Managers

## Learned Lessons

### 1 controller per resource

One thing that wasn't clear originally was that it is an anti-pattern to have multiple controllers **managing** the same resource. Multiple controllers can watch the same resource without problems, for example multiple of them could watch for specific config map changes. However, if a controller needs to utilize the resources status or finalizers, or anything that requires a write or update, then multiple controllers will step on each other's toes if pointed at the same resource. We originally created 2 different controllers: 1 to manage the ngrok api resources and 1 to manage the tunnels. Both watched for ingress objects and relied on the finalizer in order to clean up their respective ngrok api resources and agent tunnels. But there was a race condition where if 1 finished first and removed the finalizer, the other would leak resources.

# Our Architecture

Today, our supported method of configuring ingress is via the Kubernetes Ingress Kind with things like Gateway and maybe CRD's in the future but not presently. Ultimately the goal is to watch for Ingress configuration changes and in turn manage
- ngrok API resources such as reserved domains, edges, backends, etc
- ngrok sessions and tunnels originating in your cluster to our edge infrastructure

## Controllers

This is a running list of the controllers we have today and what they do. We will go into more detail on each of these in the following sections. Today these are all built into a single binary and run as a single manager. In the future we may split them out into separate binaries and/or managers.

### Ingress Controller

The ingress controller is responsible for watching for Ingress resources and managing the ngrok API resources. It watches for Ingress resources and creates, updates, and deletes ngrok API resources as needed. It only runs in the leader elected pod to ensure there is only 1 actor trying to converge the ngrok api resources to match the desired state represented by the ingress objects.

Today, an ingress object is mapped 1-1 with an edge. The controller knows what edge it maps to because it stamps each object with an annotation denoting the edge id. This comes with limitations and will likely change so the controller manages multiple ingress objects and can merge them into a single edge.

The ingress controller relies on using a finalizer on the ingress object to clean up resources in the API. Essentially, if you delete an ingress object from k8s, the controller receives the delete event, but the k8s api server will not fully remove the resource until the controller removes the finalizer. This allows the controller to clean up any resources it created in the
ngrok API before the object is fully removed from k8s.

The ingress controller is responsible for reserving domains today. If you create an ingress object with a host, it will automatically try to reserve that domain for you. If you delete an ingress object though, we do not unreserve the domain automatically to avoid you losing domains.

The status of the ingress object is updated with the public url of the edge. If this is an ngrok domain, it will simply be the domain name. If it is a custom domain, it will be the CNAME record you need to create (or let something like external-dns create for you) to point to the edge.

### Tunnel Controller

Note: At the time of writing this, the tunnel controller is different from what it will be. This short section is representing what it _will_ be.

From the ingress resources, we end up needing to create specific tunnels for each of the unique service backends in each ingress object. Since the ingress controller is responsible for managing the ingress objects, the tunnel controller needs to be given this information from the ingress controller in a separate way so they aren't both fighting over the same resource. The tunnel controller will watch for a new custom resource called a Tunnel. The ingress controller will create a tunnel for each unique service backend in each ingress object. The tunnel controller will then create the tunnel in the ngrok agent and update the tunnel resource.


### TODO:
- driver and store pattern
- ngrok-go usage
- annotations system