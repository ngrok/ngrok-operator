# Ngrok Ingress Controller Architecture and Design

* high level what this thing does
* What is a controller
* What is the manager vs controllers within the project
  * Link to a section later, but also get into technical details like
    * how reconciler loops work
    * any other specific technical bits of the controller-runtime
* Current known limits (portions of the ingress spec we don't support)
* credential and config management (paid only for now)
* how local agents are managed (tunnel controller)
* how ngrok api resources are managed (details about each resource)
  * 1 ingress == 1 edge
  * k8s object updates (status, finalizers, edge-id, etc)
* publicly distributed artifacts (containers, helm chart, and docs)
