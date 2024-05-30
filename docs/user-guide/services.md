# Services

The ngrok ingress controller, soon to be renamed to the ngrok operator, is capable of exposing kubernetes `Service` resources to the internet. This is done by creating a ngrok tunnel and edge for the service. The controller will automatically create a ngrok tunnel and edge for the service when the service is created or updated. The controller will also automatically delete the ngrok tunnel and edge when the service is deleted.

- [Services](#services)
  - [TCP](#tcp)
    - [Example](#example)
    - [Modules](#modules)
  - [TLS](#tls)
    - [Example](#example-1)
    - [Modules](#modules-1)


## TCP

By default, services of type `LoadBalancer` are exposed using a ngrok [TCP Edge](https://ngrok.com/docs/tcp/#edges). A reserved address is automatically created for the service and the service's status will be updated with the reserved address. Other projects like [external-dns](../examples/external-dns/README.md) can be used to create a CNAME record for the reserved address automatically.


### Example
```yaml
apiVersion: v1
kind: Service
metadata:
  name: mysite
  annotations:
    k8s.ngrok.com/modules: only-trusted-ips # Optional. The ngrok modules used for this Service
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

### Modules

The `k8s.ngrok.com/modules` annotation can be used to specify the ngrok modules to use for the service. The following modules are available for TCP
services:
* [IP Restrictions](./route-modules.md#ip-restrictions)

If other modules are supplied that are not supported by the TCP edge, the controller will ignore them.


## TLS

### Example

**Note**: the `k8s.ngrok.com/domain` annotation is required to use TLS and will expose the service as a [TLS Edge](https://ngrok.com/docs/tls/). 
Once the reserved domain is ready and the `TLSEdge` is created, the service's status will be updated with the ngrok address. Other projects like [external-dns](../examples/external-dns/README.md) can be used to create a CNAME record for the reserved domain automatically.

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

### Modules

The `k8s.ngrok.com/modules` annotation can be used to specify the ngrok modules to use for the service. The following modules are available for TLS
services:
* [IP Restrictions](./route-modules.md#ip-restrictions)
* [Mutual TLS](./route-modules.md#mutual-tls)

If other modules are supplied that are not supported by the TLS edge, the controller will ignore them.
