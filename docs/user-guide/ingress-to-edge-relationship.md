# Ingress to Edge Relationship

This ingress controller aims to take the [ingress spec](https://kubernetes.io/docs/concepts/services-networking/ingress/#the-ingress-resource) and implement each specified concept into ngrok edges. The concept of an ngrok Edge is documented more [here](https://ngrok.com/docs/cloud-edge/). This document aims to explain how multiple ingress objects with rules and hosts that overlap combine to form edges in the ngrok API.

Overall
- a host correlates directly to an edge
- rules spread across ingress objects with matching hosts get merged into the same edge
- annotations on an ingress apply only to the routes in the rules in that ingress if possible

## Types of Ingress

[This Kubernetes "Types of Ingress" Doc](https://kubernetes.io/docs/concepts/services-networking/ingress/#types-of-ingress) breaks down a few common ingress examples. We'll use these examples to explain how the ingress controller will handle them and what the end result edge configurations are.

### Single Service Ingress

> There are existing Kubernetes concepts that allow you to expose a single Service (see alternatives). You can also do this with an Ingress by specifying a default backend with no rules.

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: test-ingress
spec:
  defaultBackend:
    service:
      name: test
      port:
        number: 80
```

While not implemented yet, when completed, this style of ingress should allow exposing a service across all edges configured by the controller. Without any other ingress objects with hosts configured though, this ingress would do nothing as there is no edge and host to attach to.

### Simple Fanout

> A fanout configuration routes traffic from a single IP address to more than one Service, based on the HTTP URI being requested. An Ingress allows you to keep the number of load balancers down to a minimum.

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: simple-fanout-example
spec:
  rules:
  - host: foo.bar.com
    http:
      paths:
      - path: /foo
        pathType: Prefix
        backend:
          service:
            name: service1
            port:
              number: 4200
      - path: /bar
        pathType: Prefix
        backend:
          service:
            name: service2
            port:
              number: 8080
```

This configuration would produce a single edge with two routes.
- edge: `foo.bar.com`
  - route: `/foo` -> `service1:4200`
  - route: `/bar` -> `service2:8080`

If another ingress object like this was created

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: simple-fanout-example-2
spec:
  rules:
  - host: foo.bar.com
    http:
      paths:
      - path: /baz
        pathType: Prefix
        backend:
          service:
            name: service3
            port:
              number: 8080
```

The edge would be updated to have three routes.
- edge: `foo.bar.com`
  - route: `/foo` -> `service1:4200`
  - route: `/bar` -> `service2:8080`
  - route: `/baz` -> `service3:8080`

Ingress rules with the same host are merged into the same edge.

### Name-based Virtual Hosting

> Name-based virtual hosting is a common way to implement virtual hosting on a single IP address. Each host name is associated with a distinct IP address, and the DNS resolver associates a given host name with its corresponding IP address(es). The Ingress resource does not require any special support for name-based virtual hosting, but it does require that the DNS resolver be configured to return the correct IP address for a given host name.

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: name-virtual-host-ingress
spec:
  rules:
  - host: foo.bar.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: service1
            port:
              number: 80
  - host: bar.foo.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: service2
            port:
              number: 80
```

This configuration would produce two edges with one route each.
- edge: `foo.bar.com`
  - route: `/` -> `service1:80`
- edge: `bar.foo.com`
  - route: `/` -> `service2:80`

#### No Host

> If you create an Ingress resource without any hosts defined in the rules, then any web traffic to the IP address of your Ingress controller can be matched without a name based virtual host being required.

While not implemented yet, this would work the same as default backends where the rule is applied to all edges.
TODO: The example has 2 rules with hosts and then a third without a host, so when a request matches neither, it does match that one. I'm not sure really if thats the same "default apply to everything" or if there is a fallback problem

## Annotations

Annotations are created and applied at the ingress object level. However, from the section above, multiple ingresses can combine and be shared to form multiple edges. When using annotations that apply specifically to routes, the annotations on the ingress apply to all routes, but routes for multiple edges across different ingresses don't have to have the same annotations or modules.

So while annotations are limited to being applied to the whole ingress object, and we'd like to apply different route module annotations to different routes in 1 edge, we can leverage the fact that multiple ingresses can be combined to form an edge if the rules hosts match, but each annotation only applies to those routes in the ingress object they came from.

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: example-header-add-1
  annotations:
    k8s.ngrok.com/response-headers-add: |
      {
        "X-SEND-TO-CLIENT": "Value1"
      }
spec:
  rules:
  - host: foo.bar.com
    http:
      paths:
      - path: /foo
        pathType: Prefix
        backend:
          service:
            name: service1
            port:
              number: 4200
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: example-header-add-2
  annotations:
    k8s.ngrok.com/response-headers-add: |
      {
        "X-SEND-TO-CLIENT": "Value2"
      }
spec:
  rules:
  - host: foo.bar.com
    http:
      paths:
      - path: /bar
        pathType: Prefix
        backend:
          service:
            name: service2
            port:
              number: 8080
```

This configuration would produce a single edge with two routes. Each route has a different http response header module

- edge: `foo.bar.com`
  - route: `/foo` -> `service1:4200`
    - module: `response-headers-add` -> `{"X-SEND-TO-CLIENT": "Value1"}`
  - route: `/bar` -> `service2:8080`
    - module: `response-headers-add` -> `{"X-SEND-TO-CLIENT": "Value2"}`

TODO: - need well defined rules for how the whole model of the store is built https://kubernetes.github.io/ingress-nginx/how-it-works/#building-the-nginx-model
