# Ingress Annotations

Below, you can find a list of annotations that can be used to configure Ingress behavior with the ngrok Ingress Controller. These annotations apply to all routes defined in the Ingress resource. The ingress
controller will configure [Edge HTTPS Routes](https://ngrok.com/docs/api/resources/edges-https-routes)
based on the `spec.rules` defined in the Ingress resource. It will then configure each HTTPS Edge Route Module as defined by the annotations.

- [Compression](#compression)
- [Header Modification](#header-modification)
  - [Request Headers](#request-headers)
  - [Response Headers](#response-headers)
- [IP Restriction](#ip-restriction)
- [TLS Termination](#tls-termination)
  - [Setting Minimum TLS Version](#setting-minimum-tls-version)


## Compression

The `k8s.ngrok.com/https-compression` annotation can be used to enable or disable compression for
all routes defined in the Ingress resource. The annotation accepts a boolean value. If not specified(default), the `Compression` module will be disabled in ngrok for any Edge HTTPS Routes the annotation applies to.


```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: minimal-ingress
  annotations:
    k8s.ngrok.com/https-compression: "true"
spec:
  ...
```

## Header Modification

### Request Headers

The following annotations can be used to add or remove headers for each request before it is sent to the backend service.

* `k8s.ngrok.com/request-headers-add` - Add headers to the request before it is sent to the backend service.
* `k8s.ngrok.com/request-headers-remove` - Remove headers from the request before it is sent to the backend service.
  
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: minimal-ingress
  annotations:
    k8s.ngrok.com/request-headers-remove: "X-DROP-ME"
    k8s.ngrok.com/request-headers-add: |
      {
        "X-SEND-TO-BACKEND": "Value1"
      }
    
```

### Response Headers

The following annotations can be used to add or remove headers for each response before it is sent from the Edge to the client.

* `k8s.ngrok.com/response-headers-add` - Add headers to the response before it is sent from the Edge to the client.
* `k8s.ngrok.com/response-headers-remove` - Remove headers from the response before it is sent from the Edge to the client.

If none of the above annotations are present, the `Response Headers` module will be disabled in ngrok for any Edge HTTPS Routes the annotation applies to.

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: minimal-ingress
  annotations:
    k8s.ngrok.com/response-headers-remove: "X-DROP-ME"
    k8s.ngrok.com/response-headers-add: |
      {
        "X-SEND-TO-CLIENT": "Value1"
      }
    
```


## IP Restriction

The `k8s.ngrok.com/ip-policy-ids` annotation can be used to restrict access to all routes defined in the Ingress resource to a list of IP Policies. The annotation accepts a comma-separated list of IP Policy IDs. If not specified(default), the `IP Restriction` module will be disabled in ngrok for any Edge HTTPS Routes the annotation applies to.

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: minimal-ingress
  annotations:
    k8s.ngrok.com/ip-policy-ids: "ipp_ABC123tV8hrTPdf0Q0lS4KC,ipp_ABCD123V8hrTPdf0Q0lS4"
spec:
  ...
```

## TLS Termination

### Setting Minimum TLS Version

The `k8s.ngrok.com/tls-min-version` annotation can be used to set the minimum TLS version for all routes belonging to an Edge. *Note*: **This setting applies to all routes
belonging to an Edge, not just the routes defined in the Ingress resource**. The annotation accepts a string value. If not specified(default), the `TLS Termination` module will be disabled in ngrok and TLS will be terminated at the edge using the ngrok default minimum TLS version.

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: minimal-ingress
  annotations:
    k8s.ngrok.com/tls-min-version: "1.3"
spec:
  ...
```
