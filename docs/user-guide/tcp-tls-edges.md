# TCP and TLS Edges

ngrok offers [TCP](https://ngrok.com/docs/cloud-edge/edges/tcp/) and
[TLS](https://ngrok.com/docs/cloud-edge/edges/tcp/) Edges which can be used to
provide ingress to TCP or TLS based services. Both are implemented as CRDs and
function similarly in broad strokes, albeit with slightly different
configuration options offered. [Their CRD reference](./crds.md#tcp-edges) is a
useful companion to this guide.

## (TLS Only) Get a Domain

At least one `hostports` must be specified when creating a TLSEdge resource,
which takes the form `<fqdn>:443`. The fully qualified domain name must first be
reserved either via the ngrok dashboard or the [Domain](./crds.md#domains) CRD.

Example:

```yaml
apiVersion: ingress.k8s.ngrok.com/v1alpha1
kind: Domain
metadata:
  name: tlsedgetest-ngrok-app
spec:
  domain: tlsedgetest.ngrok.app
```

## Create the Edge

Create the edge CRD. These resources are fairly similar, and both require you to
specify a [TunnelGroupBackend](./crds.md#tunnelgroupbackend). This consists of a
list of labels that determine which specific [Tunnel](./crds.md#tunnels) should
receive traffic from the edge. Both may also specify [IP
Policies](https://ngrok.com/docs/api/resources/ip-policies/) for limiting access
to the edge. At the time of writing, these policies must be provided as a
reference in the form `ipp_<id>`.

On top of the options available to TCP Edges, TLS Edges support (and require) a few other options:

- (required) `hostports`: A list of `"<fqdn>:443"` strings declaring the list of
  reserved domains for the edge to listen on.
- [`tlsTermination`](https://ngrok.com/docs/api/resources/tls-edge-tls-termination-module/): Configure the TLS Termination behavior. The `terminateAt` field may be set to `upstream` to pass the encrypted stream to the Tunnel backend, or `edge` to terminate the TLS stream at the ngrok edge, and pass plaintext bytes to the Tunnel.
- [`mutualTls`](https://ngrok.com/docs/api/resources/tls-edge-mutual-tls-module/): Configure client certificate validation at the edge. Requires a reference to a [Certificate Authority](https://ngrok.com/docs/api/resources/certificate-authorities/).

TCP Example:

```yaml
apiVersion: ingress.k8s.ngrok.com/v1alpha1
kind: TCPEdge
metadata:
  name: test-edge
spec:
  backend:
    labels:
      app: tcptestedge
```

Because TCP Edges don't currently support providing a reserved TCP address. On edge creation, one will be allocated for them, and will be visible by checking the status of the resource:

```bash
$ kubectl get tcpedges test-edge
NAME        ID                                   HOSTPORTS                  BACKEND ID                          AGE
test-edge   edgtcp_2Wg5AzVE878vQoNMP3Z8wONIr76   ["7.tcp.ngrok.io:27866"]   bkdtg_2Wg5Amjb4GiQoV7SAnpEdM0Dg3n   2m35s
```

TLS Example:

```yaml
apiVersion: ingress.k8s.ngrok.com/v1alpha1
kind: TLSEdge
metadata:
  name: test-edge
spec:
  hostports:
    - tlstestedge.ngrok.app:443
  backend:
    labels:
      app: tlstestedge
  tlsTermination:
    terminateAt: upstream
```

## Start the Tunnel

Finally, create a Tunnel to receive and forward traffic for your edge.

Important fields:

- `forwardsTo`: The `<hostname>:<port>` to forward traffic to. This can be any
  hostname resolvable and accessible from the ingress controller pod.
- `labels`: a map of labels corresponding to the edge to receive traffic for.
  These must match the labels specified when creating your edge.
- `backend.protocol`: The protocol understood by the backend service. `TCP` will
  forward connections to the backend as-is, while `TLS` will create a TLS
  connection to the backend _first_, and then forward the connection stream over
  that.

Example:

```yaml
apiVersion: ingress.k8s.ngrok.com/v1alpha1
kind: Tunnel
metadata:
  name: test-tunnel
spec:
  backend:
    protocol: TCP
  forwardsTo: kubernetes.default.svc:443
  labels:
    app: tlsedgetest
```

# Full Example

This is an example of using a TLS Edge to expose the kubernetes control plane via ngrok.

```yaml
---
apiVersion: ingress.k8s.ngrok.com/v1alpha1
kind: Domain
metadata:
  name: tlsedgetest-ngrok-app
spec:
  # Reserve the tlsedgetest.ngrok.app domain.
  domain: tlsedgetest.ngrok.app
---
apiVersion: ingress.k8s.ngrok.com/v1alpha1
kind: TLSEdge
metadata:
  name: test-edge
spec:
  hostports:
    # Listen for connections on the domain we reserved
    - tlsedgetest.ngrok.app:443
  backend:
    labels:
      app: tlsedgetest
  # Pass the TLS stream on to the backend - let the application do its own TLS
  # handshake.
  tlsTermination:
    terminateAt: upstream
---
apiVersion: ingress.k8s.ngrok.com/v1alpha1
kind: Tunnel
metadata:
  name: test-tunnel
spec:
  # Forward the raw TCP stream to our backend.
  # It will technically contain TLS, and the backend speaks TLS, but we don't
  # want the Tunnel to terminate TLS before forwarding incoming connections.
  # We don't want a TLS turducken.
  backend:
    protocol: TCP
  # Forward to the kubernetes control plane.
  forwardsTo: kubernetes.default.svc:443
  # Listen for connections using the labels from our edge.
  labels:
    app: tlsedgetest
```

Check the status of your resources:

```
$ kubectl get domain
NAME                    ID                               REGION   DOMAIN                  CNAME TARGET   AGE
tlsedgetest-ngrok-app   rd_2Wg986lvMqsiB1J5WV5lOcmT21a            tlsedgetest.ngrok.app                  4s
$ kubectl get tlsedge
NAME        ID                                   HOSTPORTS                       BACKEND ID                          AGE
test-edge   edgtls_2Wg989BMmZLWXixStL8BjAxMcxW   ["tlsedgetest.ngrok.app:443"]   bkdtg_2Wg981gcSnxaX5cTL28LWwVg4xD   12s
$ kubectl get tunnel
NAME          FORWARDSTO                   AGE
test-tunnel   kubernetes.default.svc:443   52m
```

Our domain and edge both have IDs allocated, so we know they've been created successfully!

Edit your kubeconfig and replace the `server` with
`https://tlsedgetest.ngrok.app`, comment out `certificate-authority-data` and
add `insecure-skip-tls-verify: true` to your `cluster` config. This is needed
because kubernetes is completing the TLS handshake with its own certificate,
which won't be valid for your ngrok domain.

Use `kubectl cluster-info` to verify that everything is still working:

```
$ kubectl cluster-info
Kubernetes control plane is running at https://tlsedgetest.ngrok.app
CoreDNS is running at https://tlsedgetest.ngrok.app/api/v1/namespaces/kube-system/services/kube-dns:dns/proxy

To further debug and diagnose cluster problems, use 'kubectl cluster-info dump'.
```
