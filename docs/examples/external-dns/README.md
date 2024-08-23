# ngrok Ingress Controller + External DNS <!-- omit from toc -->

When creating an Ingress object with a custom domain you own, ngrok will wait until the domain ownership is verified before it will create an edge for it. While you can do this manually as seen in the [Custom Domain Guide](../../user-guide/custom-domains.md), you can also use [External DNS](https://github.com/kubernetes-sigs/external-dns) to automate this process since the controller adds the required CNAME record to the Ingress status object.

:warning: If the domain you are trying to use already has a CNAME record, ngrok will not be able to create the edge for it. If you describe a tlsedge that has this issue, you'll see the following:

```
Failed to create v1alpha1.TLSEdge default/something-s65zs: HTTP 400: The domain 'something.mydomain.com:443' is not reserved. [ERR_NGROK_7117]
```

You will need to remove the existing CNAME record before ngrok can create the edge. A common cause of this issue when using External DNS is that External DNS is not configured to delete the CNAME record when the Ingress/Service object is deleted because [its policy is set to upsert-only](https://github.com/kubernetes-sigs/external-dns/blob/bed56b7151be415a8af0d7182ed675fd7ced2b67/charts/external-dns/values.yaml#L208) which is the default. To fix this, you can change the External DNS policy to `sync` which will delete the CNAME record when the Ingress/Service object is deleted.


- [Ingress](#ingress)
- [Services](#services)
  - [TCP](#tcp)
  - [TLS](#tls)


## Ingress

Example:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: example-ingress
  annotations:
    k8s.ngrok.com/modules: compression,tls,oauth # Optional. The ngrok modules used for this Ingress
    external-dns.alpha.kubernetes.io/hostname: "mysite.mydomain.com"
spec:
  ingressClassName: ngrok
  rules:
  - host: mysite.mydomain.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
          name: mysite
          port:
            number: 80
```

## Services

### TCP

Example:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: mysite
  annotations:
    k8s.ngrok.com/modules: only-trusted-ips # Optional. The ngrok modules used for this Service
    external-dns.alpha.kubernetes.io/hostname: "mysite.mydomain.com"
spec:
  allocateLoadBalancerNodePorts: false # ngrok's tunneling technology does not require NodePorts to be allocated.
  loadBalancerClass: ngrok
  type: LoadBalancer
  selector:
    app: mysite
  ports:
  - name: traffic
    port: 9000
    protocol: TCP
    targetPort: 9000
```

By default, services of type `LoadBalancer` are exposed using a ngrok `TCPEdge`. Once the reserved address is ready and the `TCPEdge` is created, the
service's status will be updated with the ngrok address which external-dns will use to create the CNAME record.

```yaml
status:
  loadBalancer:
    ingress:
    - hostname: 1.tcp.ngrok.io
      ports:
      - port: 26349
        protocol: TCP
```

In this example, the `mysite.mydomain.com` domain will be point to `1.tcp.ngrok.io` once the CNAME record is created.

### TLS

Example:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: mysite
  annotations:
    k8s.ngrok.com/modules: only-trusted-ips # Optional. The ngrok modules used for this Service
    k8s.ngrok.com/domain: mysite.mydomain.com # Required to use TLS
    external-dns.alpha.kubernetes.io/hostname: "mysite.mydomain.com"
spec:
  allocateLoadBalancerNodePorts: false # ngrok's tunneling technology does not require NodePorts to be allocated.
  loadBalancerClass: ngrok
  type: LoadBalancer
  selector:
    app: mysite
  ports:
  - name: traffic
    port: 9000
    protocol: TCP
    targetPort: 9000
```

**Note**: the `k8s.ngrok.com/domain` annotation is required to use TLS. Once the reserved domain is ready and the `TLSEdge` is created, the service's
status will be updated with the ngrok address which external-dns will use to create the CNAME record.

```yaml
status:
  loadBalancer:
    ingress:
    - hostname: 3slhqz9jkjkv5x7z7qzq2.4raw8yu7nqzudp4.ngrok-cname.com
      ports:
      - port: 443
        protocol: TCP
```

In this example, the `mysite.mydomain.com` domain will be point to `3slhqz9jkjkv5x7z7qzq2.4raw8yu7nqzudp4.ngrok-cname.com` once the CNAME record is created.
