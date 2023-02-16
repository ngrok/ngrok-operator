# Multiple Ingress Controller Installations

Kubernetes has the concept of [Ingress Classes](https://kubernetes.io/docs/concepts/services-networking/ingress/) which allows you to run multiple ingress controllers in the same cluster. This is useful if you want to run multiple ingress controllers, or if you want to run multiple versions of the ngrok Kubernetes Ingress Controller.

Currently, we support full ingress class filtering, but we have some work on the road map still to enable use cases like running multiple ngrok Kubernetes Ingress Controllers in the same cluster watching specific namespaces.

## Ingress Class Filtering

Ingress controllers work by watching the k8s ingress resources for changes and reconciling them to provide ingress. By default, if you installed multiple controllers, they would both try to reconcile all ingress objects which could cause conflicts and unexpected behavior. To solve for this use case, Ingress classes can be set on ingress objects and controllers can filter ingresses to only those that match their ingress class.

By default, the helm chart will make a non-default ingress class with the name `ngrok` that the controller will watch.
In order for the controller to reconcile an ingress, the ingress must have the same ingress class as the controller via:

  ```yaml
  ...
  spec:
    ingressClassName: ngrok
    rules:
    ...
```

You can override this name via the helm value `ingressClass.name`, or if there aren't other ingress controllers in the cluster, you can set it to default to be true and not have to add the `ingressClass` to the ingress objects' specs.

## Multiple ngrok Kubernetes Ingress Controller Installations

While its possible to install multiple versions of the controller right now, they can't watch separate ingress classes yet which would cause them to conflict. https://github.com/ngrok/kubernetes-ingress-controller/issues/152 tracks this work.

## Watching Specific Namespaces

Currently, the ingress controller watches all namespaces. Its a common use case to need a controller to watch only a specific namespace in the case where people run a controller in a namespace for a particular team or environment. https://github.com/ngrok/kubernetes-ingress-controller/issues/64 tracks this work.