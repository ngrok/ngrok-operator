# Kubernetes Gateway API Feature

## Overview

The ngrok-operator supports the Kubernetes Gateway API, providing a role-oriented and extensible alternative to Ingress for managing external access to services.

> **Prerequisites**: The Gateway API CRDs are **not** installed by the ngrok-operator. You must install them separately before enabling this feature. See the [Gateway API installation guide](https://gateway-api.sigs.k8s.io/guides/#installing-gateway-api). The operator supports Gateway API `v1` (stable) resources and `v1alpha2` for TCPRoute and TLSRoute.

## Configuration

| Helm Value                         | Description                                          | Default |
|------------------------------------|------------------------------------------------------|---------|
| `ngrok.features.gateway.enabled`                  | Enable Gateway API support (if CRDs detected)        | `true`  |
| `ngrok.features.gateway.disableReferenceGrants`   | Disable ReferenceGrant requirement                   | `false` |
| `ngrok.features.gateway.conformanceHostnameSuffix` | Test only: appended to every Gateway API hostname so the conformance suite can run. See [gateway-api-conformance.md](../gateway-api-conformance.md) | `""` |

## Supported Resources

| Resource         | API Version                          | Description                        |
|------------------|--------------------------------------|------------------------------------|
| `GatewayClass`   | `gateway.networking.k8s.io/v1`       | Defines the controller type        |
| `Gateway`        | `gateway.networking.k8s.io/v1`       | Configures listeners and addresses |
| `HTTPRoute`      | `gateway.networking.k8s.io/v1`       | HTTP routing rules                 |
| `TCPRoute`       | `gateway.networking.k8s.io/v1alpha2` | TCP routing rules                  |
| `TLSRoute`       | `gateway.networking.k8s.io/v1alpha2` | TLS routing rules                  |
| `ReferenceGrant` | `gateway.networking.k8s.io/v1beta1`  | Cross-namespace reference grants   |

## Behavior

When enabled and Gateway API CRDs are detected:

1. A user creates a `GatewayClass` resource with `spec.controllerName` matching the operator's controller name (`ngrok.com/gateway-controller`).
2. `Gateway` resources referencing that GatewayClass are reconciled by the operator.
3. Route resources (`HTTPRoute`, `TCPRoute`, `TLSRoute`) referencing a managed Gateway are materialized as ngrok endpoints.
4. `ReferenceGrant` resources enable cross-namespace references (e.g., a route in namespace A referencing a service in namespace B).
5. Domain resources are managed for the hostnames the Gateway's endpoints use (see [Hostnames](#hostnames)). A hostname that is a direct child of an already-reserved wildcard gets a Domain CR but no separate ngrok reservation — see [controllers/domain.md](../controllers/domain.md#wildcard-domain-coverage).

## Hostnames

ngrok routes by hostname, so every HTTP, HTTPS and TLS endpoint needs one. The endpoints for a listener get their hostnames this way:

1. If the Gateway sets `spec.addresses` (type `Hostname`), those addresses, for every listener.
2. Otherwise, each route attached to the listener contributes the intersection of its hostnames with the listener's. A listener `*.example.com` with a route for `foo.example.com` gets an endpoint for `foo.example.com`; a route without hostnames takes the listener's.
3. When neither the listener nor the route names a hostname, a domain ngrok assigns the Gateway, the way `ngrok http 80` gets one. The operator doesn't make the name up, because a made-up name could already belong to someone else. TCP listeners still need `spec.addresses`.

### The assigned domain

See `pkg/managerdriver/gateway_address.go`. ngrok only hands out a name to an agent endpoint started without one (`url: https://`): the reserved-domains API needs a name, and a CloudEndpoint can't be created without one (`ERR_NGROK_18006`). The name changes every time such an endpoint restarts, so the operator pins the first one:

1. While a Gateway with a hostname-less HTTP, HTTPS or TLS listener has no domain, the Driver adds a claim: AgentEndpoint `<gateway>-address-claim` with `url: https://`, a policy that answers 503, and labels `ngrok.com/gateway-uid` and `ngrok.com/gateway-name`. The agent writes the domain ngrok gave it to `status.assignedURL`.
2. The Gateway controller watches claims, so the next Sync sees the domain. It creates a Domain for it labeled `ngrok.com/gateway-uid`, and stops adding the claim, which deletes it. ngrok refuses to reserve a domain an endpoint is using ("You may not reserve domains that are actively in use"); the Domain controller retries until the claim is gone.
3. From then on the labeled Domain is the Gateway's domain (`assignedGatewayDomains` reads it from the API server, not the cache, so a sync right after the pin can't miss it and claim again). Its endpoints serve on it and it is the Gateway's `status.addresses` entry. Route changes and restarts keep it. A re-created Gateway has a new UID and gets a new domain.

Until step 2, routes without hostnames on that listener aren't served and the Gateway reports `Programmed=False`, `AddressNotAssigned`.

A route's rules are added to every endpoint of the listener whose hostname one of the route's hostnames covers, so a route for `*.example.com` also serves `foo.example.com` behind a more specific route for that name. Within an endpoint, matches are ordered by the Gateway API precedence rules: the most specific matching hostname, then exact path over prefix and the longest prefix, then method, then the most header matches, then the most query parameter matches, then the oldest route, then `namespace/name`, then rule order.

## Backends

A backendRef that is not a core `Service`, is in another namespace without a ReferenceGrant, or names a Service that doesn't exist, gets a 500 for its share of the route's traffic. A rule with neither backendRefs nor filters answers every request with a 500. The route stays `Accepted`; its `ResolvedRefs` condition reports the problem.

## Driver Pattern

Gateway API controllers use a Driver pattern:

1. Controllers update an internal store with the desired state from Gateway API resources.
2. `Driver.Sync()` is called to materialize the combined state as ngrok endpoint resources.
3. This allows the driver to consider the full picture (Gateway + Routes + Services) when generating endpoints.

## Annotations

The following annotations influence behavior. They are supported on `Gateway` resources only — per-route annotation overrides are not supported.

- `ngrok.com/mapping-strategy` — Controls endpoint creation strategy
- `ngrok.com/traffic-policy` — References a TrafficPolicy
- `ngrok.com/pooling-enabled` — Enables endpoint pooling
- `ngrok.com/description` — Sets endpoint description
- `ngrok.com/metadata` — Sets endpoint metadata

See [annotations.md](../annotations.md) for details.

## Listener TLS Options

Keys in a Gateway listener's `spec.listeners[].tls.options` map with the `ngrok.com/terminate-tls.` prefix configure the `terminate-tls` traffic-policy action on endpoints generated for that listener; the suffix after the prefix becomes the action option name.

```yaml
listeners:
- name: https
  port: 443
  protocol: HTTPS
  tls:
    options:
      ngrok.com/terminate-tls.min_version: "1.3"
```

| Detail          | Value                                                  |
|-----------------|--------------------------------------------------------|
| Key form        | `ngrok.com/terminate-tls.<option>`                     |
| Reserved suffixes (rejected) | `server_private_key`, `server_certificate`, `mutual_tls_certificate_authorities` — managed via `certificateRefs`, not options |

Keys without the prefix are ignored. Supplying a reserved suffix fails translation for the Gateway.

## ReferenceGrants

By default, cross-namespace references require a `ReferenceGrant` in the target namespace. This can be disabled via `ngrok.features.gateway.disableReferenceGrants: true`, which allows cross-namespace references without explicit grants.

## When Disabled

When `ngrok.features.gateway.enabled: false` or Gateway API CRDs are not installed:
- Gateway API resources are not watched or reconciled
- The feature is excluded from the KubernetesOperator's `enabledFeatures`
