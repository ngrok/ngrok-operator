# Multiple Ingress Controller Installations

The Ngrok Kubernetes Ingress Controller supports the Kubernetes concept of [Ingress Classes](https://kubernetes.io/docs/concepts/services-networking/ingress/) which allows you to run multiple ingress controllers in the same cluster. This feature is useful if you want to run multiple versions of the Ngrok Kubernetes Ingress Controller or other ingress controllers in the same cluster.

## What is Ingress Class Filtering?

Ingress controllers work by watching Kubernetes Ingress resources for changes and reconciling them to provide ingress to services. When multiple ingress controllers are installed in the same cluster, they can both try to reconcile all ingress objects by default, which can cause conflicts and unexpected behavior. To address this issue, Ingress Classes can be set on ingress objects and controllers can filter ingresses to only those that match their ingress class.

By default, the Ngrok Kubernetes Ingress Controller creates a non-default ingress class with the name `ngrok`, which the controller watches for changes. In order for the controller to reconcile an ingress, the ingress must have the same ingress class as the controller.

```yaml
spec:
  ...
  ingressClassName: ngrok
  rules:
  ...
```

You can override this name via the helm value `ingressClass.name`, or if there aren't other ingress controllers in the cluster, you can set it to default to be true and not have to add the `ingressClass` to the ingress objects' specs.

## Multiple ngrok Kubernetes Ingress Controller Installations

While it's possible to install multiple versions of the controller right now, they can't watch separate ingress classes yet which would cause them to conflict. https://github.com/ngrok/kubernetes-ingress-controller/issues/87 tracks this work.

## Watching Specific Namespaces

By default, the ingress controller watches all namespaces. It's a common use case to need a controller to watch only a specific namespace in the case where you may run a controller in a namespace for each team or environment. In order to watch only a specific namespace for ingress objects, you can set the helm value [`watchNamespace`](https://github.com/ngrok/kubernetes-ingress-controller/blob/main/helm/ingress-controller/README.md#controller-parameters) to the namespace you want to watch.
