# Custom domain

In the Kubernetes Ingress spec, ingresses can have multiple rules with different hostnames. The full relationship of these rules' hostnames to ngrok reserved domains and edges can be found in the [ingress to edge relationship](./ingress-to-edge-relationship.md#name-based-virtual-hosting) documentation. While standard ngrok domains are available for use immediately after reservation, custom white label domains may require a couple extra steps to get working. The following outlines 2 options for getting custom white label domains working with the ngrok Kubernetes Ingress Controller.

## Managed by Kubernetes

If you create an ingress object with a hostname that is not a standard ngrok domain, the controller will attempt to create a custom white label domain for you. This domain will be reserved and registered with ngrok, but will not be configured until you have configured the DNS records for it. This will be registered in the ngrok API and also show up as a domain CRD For example:

If you create an ingress object such as this

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: example-ingress
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
```

The controller will attempt to reserve and register the domain `foo.bar.com` with ngrok. This will be registered in the ngrok API and also show up as a domain CRD. The controller will then wait for the DNS records to be configured for this domain. Once the DNS records are configured, the controller will configure the edge to route traffic to the ingress object.
You should be able to see the domain CRD via kubectl

`kubectl describe domain foo.bar.com`

For custom domains, the domain resource contains the CNAME target value that needs to be created for DNS resolution and certificates to work properly. This value is added to the ingress object's status.loadBalancer field.

`kubectl describe ingress example-ingress`

```yaml
Status:
  loadBalancer:
    ingress:
      hostname:  12jkh25.cname.ngrok.app
```

From here you can create the DNS record and everything should work as expected. To automate this fully though, see the [example on integrating with external-dns](../examples/external-dns.md).

## Externally managed

Domains can also be created
- manually via the dashboard UI
- via the ngrok API
- via the ngrok terraform provider

If created externally, the controller will discover the domain is already registered, and not interact with it unless you modify or delete the crd itself. Deleting the ingress objects that use that host won't result in the domain CRD in being deleted.
