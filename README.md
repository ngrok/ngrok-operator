# Ngrok Ingress Controller

This is a general purpose [kubernetes ingress controller](https://kubernetes.io/docs/concepts/services-networking/ingress-controllers/) that uses ngrok.


TOOD:
* Make readme good
* Setup CI to auto merge if approved and a label is applied and all status checks pass
* Determine at runtime if the current process is the leader (this might be embedded in the controller manager to only call reconcile on the leader)
* Generate a unique name for the controller installation.
  * It should first look for an existing config map with the unique and well known name `ngrok-ingress-controller-edge-prefix`. If it already exists, read it and proceed. Otherwise, attempt to make it. If it errors because it exists already (another controller may have just made it) swallow that error. Wait a tiny amount and then read the config map value. Use that as namespace for things in the ngrok account. Just do this in the main.go to start. All controller instances will run it, but being idempotent that should be fine where 1 works and the rest fail
  * TODO: Is a 6 digit hash good enough? Or should we make some human readable word like heroku?
  * TODO: would prefixing resources with this look/work right in the dashboard?
* maybe auto install helm
* ci to run make commands and then diff at end to make sure anything generated and checked in is all good
* perhaps use https://book.kubebuilder.io/component-config-tutorial/tutorial.html instead of a normal config map for agent configs
* use finalizers to handle deleting resources https://book.kubebuilder.io/reference/using-finalizers.html
* add ingress class
* create pr template
* helm lint

## Setup

* go 1.17 to make kubebuilder happy
* assume a k8s cluster is available via your kubectl client. Right now, I'm just using our ngrok local cluster.
* `make build`
* `make docker-build`
*  depending on how you are running your local k8s cluster, you may need to make the image available in its registry
* `k create namespace ngrok-ingress-controller`
* `kns ngrok-ingress-controller`
* create a k8s secret with an auth token
`k create secret generic ngrok-credentials --from-literal=NGROK_AUTHTOKEN=YOUR-TOKEN`
`make deploy`
