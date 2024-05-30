# ngrok Ingress Controller + External DNS

When creating an Ingress object with a custom domain you own, ngrok will wait until the domain ownership is verified before it will create an edge for it. While you can do this manually as seen in the [Custom Domain Guide](../../user-guide/custom-domains.md), you can also use [External DNS](https://github.com/kubernetes-sigs/external-dns) to automate this process since the controller adds the required CNAME record to the Ingress status object.


- [ngrok Ingress Controller + External DNS](#ngrok-ingress-controller--external-dns)
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
