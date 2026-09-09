# Current state — how kubernetes bindings work today

End-to-end, both sides of the wire. This is what we're migrating away from.

## The user-facing model

A **kubernetes-bound endpoint** is an ngrok endpoint whose binding is `kubernetes` and whose URL follows a `service.namespace` convention (e.g. `https://my-service.my-namespace`). When such an endpoint exists in your account, any operator whose selectors match it projects a Kubernetes `Service` named `my-service` into namespace `my-namespace`. Pods in the cluster reach the endpoint by talking to that Service with plain TCP/HTTP — no SDK, no sidecar.

Docs: [kubernetes-bound endpoints](https://ngrok.com/docs/k8s/guides/bindings/).

## The API resources (ngrok API, via ngrok-api-go v9)

There is **no dedicated "kubernetes bound endpoint" resource.** It's a normal `Endpoint` plus a normal `KubernetesOperator`.

### `Endpoint` (generic)

```go
type Endpoint struct {
    ID, URL, Host string
    Port          int64
    Scheme, Proto string
    Type          string   // ephemeral | edge | cloud
    Bindings      []string // "public" | "internal" | "kubernetes" | (soon "private")
    TrafficPolicy string
    UpstreamURL   string
    Metadata      string   // free-form user string
    // ... no Kubernetes-target field anywhere
}
```

The only thing that makes an endpoint "kubernetes" is `Bindings` containing `"kubernetes"`, and the only place the k8s **target** (service + namespace) lives is the `Host`/`URL` itself. There is no structured service/namespace field.

> Note: at the backend's internal `rpx.Endpoint` level, an endpoint has exactly **one** binding (`binding` / `resolved_binding`), even though the public API models `bindings` as a list (`proto/rpx/model_endpoint.proto:146-147`).

### `KubernetesOperator` (one per install)

```go
type KubernetesOperator struct {
    ID              string
    EnabledFeatures []string // subset of "bindings", "ingress", "gateway"
    Region          string
    Binding *KubernetesOperatorBinding
}

type KubernetesOperatorBinding struct {
    EndpointSelectors []string               // CEL expressions, default ["true"]
    Cert              KubernetesOperatorCert // the mTLS cert — dies with private dial
    IngressEndpoint   string
}
```

`EndpointSelectors` are CEL over `endpoint.*` attributes (e.g. `endpoint.host.endsWith('.staging')`). They are the **per-cluster scoping knob**: which of the account's kubernetes-bound endpoints this cluster projects. Default `["true"]` = project all. Operator flag: `--bindings-endpoint-selectors` (`cmd/api-manager.go:171`).

## Operator side — poll → materialize → forward

### 1. Poll (`internal/controller/bindings/boundendpoint_poller.go`)

Every 10s the poller calls `NgrokClientset.KubernetesOperators().GetBoundEndpoints(koId, paging)`. The **server** evaluates that operator's `endpoint_selectors` and returns the matching endpoints, each with a `service.namespace`-shaped URL.

### 2. Parse / aggregate (`internal/ngrokapi/bindingendpoint_aggregator.go`)

Extracts `service`, `namespace`, `port`, `scheme` out of the URL host. Endpoints sharing a hostport are aggregated into one `BoundEndpoint`.

### 3. `BoundEndpoint` CR (`api/bindings/v1alpha1/boundendpoint_types.go`)

```go
// Spec.EndpointURL regex (line ~41) — the format is baked into the CRD:
//   ^((?P<scheme>(tcp|http|https|tls)?)://)?(?P<service>[a-z][a-zA-Z0-9-]{0,62})\.(?P<namespace>[a-z][a-zA-Z0-9-]{0,62})(:(?P<port>\d+))?$
```

`Spec.Target = {Service, Namespace, Port, Protocol}` (parsed from the URL). `Spec.Port` is an allocated port from `[10000, 65535]`.

### 4. Two Services (`internal/controller/bindings/boundendpoint_controller.go`)

- **Target Service** — `ExternalName`, named `<service>` in `<namespace>`, pointing at the upstream Service's FQDN.
- **Upstream Service** — `ClusterIP`, named `ngrok-<hash>` in the operator namespace, selecting `app.kubernetes.io/component: bindings-forwarder` pods, mapping `Port:<targetPort> → TargetPort:<allocated port>`.

Net effect: `my-service.my-namespace.svc` → (ExternalName) → unique ClusterIP → (kube-proxy DNAT) → forwarder pod on the endpoint's unique allocated port.

### 5. Forward (`internal/controller/bindings/forwarder_controller.go`)

Runs in the separate `bindings-forwarder-manager`. Listens on each endpoint's allocated port (`pkg/bindingsdriver`). Per accepted connection:

1. Look up the **client** pod by source IP → build `PodIdentity{uid,name,namespace,annotations}` (for traffic policy; not for endpoint demux).
2. Load the operator's mTLS client cert (`op.Spec.Binding.TlsSecretName`).
3. `tls.Dial` the ingress (`op.Status.BindingsIngressEndpoint`) — `forwarder_controller.go:~243`.
4. `mux.UpgradeToBindingConnection(host, port, podIdentity)` — sends a `ConnRequest` protobuf header (`internal/mux/header.go:70`).
5. Relay bytes both directions.

The `host` sent is `service.namespace`; the endpoint's identity to the forwarder is **the allocated port it's listening on**.

## Backend side — the mTLS binding ingress

Server counterpart lives at `go/svc/mux/mux_k8sop.go` (mono repo). The path (`muxKubernetesBinding`, ~line 97-144):

1. Authenticate via **mTLS client cert**; account id parsed from `clientCert.DNSNames[0]`.
2. Read the `ConnRequest` header (host, port, pod identity).
3. Resolve the endpoint: `endpointMap.Get(host, port, NewKubernetesOperatorBinding(accID))` — note the **kubernetes-operator binding key**.
4. `setPodIdentity(hdr.PodIdentity)` onto the subop → surfaces as `conn.k8s.pod.*` in traffic policy.
5. Hand off to the endpoint handler.

### The endpoint index (`go/svc/mux/muxmap/map.go`)

Endpoints are indexed by `(hostname, port, binding)` (`Key`, ~line 438-440; `Get`, ~line 180-188). **Binding is part of the key.** A kubernetes-bound `foo.myns:8080` and an internal-bound `foo.myns:8080` are *different entries*. This detail is the crux of the whole migration — see [06-poc-and-findings.md](06-poc-and-findings.md).

### Scoping is account-level, not operator-level

Kubernetes-bound endpoints are **account-scoped, unique by `(host, port, binding)`** (`go/lib/bindings/computation.go:44-48` → `NewKubernetesOperatorBinding(accountID)`). If two clusters both project `foo.myns`, that's **one** account endpoint resolved by both — not two endpoints owned by two operators. The operator's identity is not what disambiguates the endpoint; the account is. (Implication: PAT-based auth is sufficient for resolution — see [06](06-poc-and-findings.md).)

## What the operator identity actually buys today

- **Account auth** (mTLS cert). → replaceable by a PAT.
- **Which binding index to query** (the cert path queries the kubernetes-operator binding). → the thing private dial does differently.
- **Pod identity** carried in the `ConnRequest`. → must move to `DialReq.metadata` (GAT-475).
- **Projection scoping** via `endpoint_selectors`, applied at **poll** time, not dial time. → unaffected by the datapath swap.
