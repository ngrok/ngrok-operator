# Gateway API Controllers

> Controllers that reconcile Kubernetes Gateway API resources into ngrok endpoints.

<!-- Last updated: 2026-04-08 -->

## Overview

The Gateway API controller group implements support for the Kubernetes Gateway API (`gateway.networking.k8s.io`). These controllers watch GatewayClass, Gateway, HTTPRoute, TCPRoute, TLSRoute, ReferenceGrant, and Namespace resources, translate them into the operator's intermediate representation via the Manager Driver, and ultimately materialize them as CloudEndpoint and AgentEndpoint CRDs.

All Gateway API controllers are registered by the **API Manager** binary and are gated behind the `--enable-feature-gateway` flag (enabled by default).

## Controllers

| Controller | Primary Resource | Secondary Watches | Owned Resources |
|------------|-----------------|-------------------|-----------------|
| `GatewayClassReconciler` | `GatewayClass` | `Gateway` | GatewayClass (status) |
| `GatewayReconciler` | `Gateway` | `GatewayClass`, `HTTPRoute`, `Domain`, `Secret`, `ConfigMap` | Gateway (status) |
| `HTTPRouteReconciler` | `HTTPRoute` | `Gateway`, `GatewayClass`, `Service`, `Domain` | HTTPRoute (status) |
| `TCPRouteReconciler` | `TCPRoute` | — | TCPRoute (status) |
| `TLSRouteReconciler` | `TLSRoute` | — | TLSRoute (status) |
| `ReferenceGrantReconciler` | `ReferenceGrant` | — | — (read-only) |
| `NamespaceReconciler` | `Namespace` | — | — (read-only) |

## Supported Gateway API Resources

The operator's Gateway controller name is `ngrok.com/gateway-controller`.

| Resource | API Version | Support Level |
|----------|-------------|---------------|
| GatewayClass | `gateway.networking.k8s.io/v1` | Core |
| Gateway | `gateway.networking.k8s.io/v1` | Core |
| HTTPRoute | `gateway.networking.k8s.io/v1` | Core |
| TCPRoute | `gateway.networking.k8s.io/v1alpha2` | Extended |
| TLSRoute | `gateway.networking.k8s.io/v1alpha2` | Extended |
| ReferenceGrant | `gateway.networking.k8s.io/v1beta1` | Core (can be disabled via `--disable-reference-grants`) |

## Reconciliation Logic

### GatewayClass

Accepts GatewayClasses where `spec.controllerName == "ngrok.com/gateway-controller"`. Sets the `Accepted` condition to `True`. Manages the `GatewayClassFinalizerGatewaysExist` finalizer — added when Gateways reference this class, removed when no Gateways reference it.

**Requeue strategy:** No requeue on success.

### Gateway

Validates each listener on the Gateway:
- HTTP listeners must use port 80 and specify a hostname.
- HTTPS listeners must use port 443 and specify a hostname.
- UDP listeners are unsupported.

Valid listeners are stored in the Driver, and `Driver.Sync()` is called to materialize endpoints. The Gateway's `Accepted` condition is set to `True` if at least one listener is valid.

Per-listener status conditions:

| Condition | Reason | When |
|-----------|--------|------|
| `Accepted` | `Accepted` | Listener passes validation |
| `Accepted` | `PortUnavailable` | HTTP listener not on port 80, or HTTPS not on 443 |
| `Accepted` | `HostnameRequired` | Listener missing hostname |
| `Accepted` | `UnsupportedProtocol` | UDP or unknown protocol |
| `Programmed` | `Pending` | Listener accepted but not yet active |
| `Programmed` | `Invalid` | Listener failed validation |

**Requeue strategy:** Driven by `managerdriver.HandleSyncResult()`.

### HTTPRoute

Validates that each `parentRef` references a Gateway managed by the ngrok GatewayClass. Sets per-parent `Accepted` conditions. When merging status, the controller preserves conditions set by other controllers (non-ngrok parent refs).

The controller watches `Service`, `Domain`, and `Gateway` resources to trigger re-reconciliation when backends or parent gateways change.

**Requeue strategy:** Driven by `managerdriver.HandleSyncResult()`.

### TCPRoute / TLSRoute

Simpler variants that check if the route references an ngrok-managed Gateway, register/remove finalizers, update the Driver store, and trigger a sync. No explicit status conditions are set by these controllers.

**Requeue strategy:** Driven by `managerdriver.HandleSyncResult()`.

### ReferenceGrant / Namespace

Read-only controllers that mirror these resources into the Driver's store. The Driver uses ReferenceGrants to validate cross-namespace references (unless disabled) and Namespaces for label-based Gateway listener filtering.

**Requeue strategy:** Driven by `managerdriver.HandleSyncResult()`.

## Gateway-to-Endpoint Mapping

The Manager Driver translates Gateway API resources into ngrok endpoints through the IR layer. The mapping follows this flow:

1. Each Gateway listener defines a hostname + protocol.
2. HTTPRoutes/TCPRoutes/TLSRoutes attached to a Gateway listener define routing rules.
3. The Driver constructs `IRVirtualHost` objects from listeners and their routes.
4. The IR layer applies the mapping strategy (collapsed or verbose) to determine whether to create a single AgentEndpoint or a CloudEndpoint + internal AgentEndpoints.
5. Traffic policy rules are generated from HTTPRoute match criteria (path, headers, query params, method) to route to the correct upstream.

## Source References

| Symbol / Concept | File | Lines |
|-----------------|------|-------|
| GatewayClass controller | `internal/controller/gateway/gatewayclass_controller.go` | — |
| Gateway controller | `internal/controller/gateway/gateway_controller.go` | — |
| HTTPRoute controller | `internal/controller/gateway/httproute_controller.go` | — |
| TCPRoute controller | `internal/controller/gateway/tcproute_controller.go` | — |
| TLSRoute controller | `internal/controller/gateway/tlsroute_controller.go` | — |
| ReferenceGrant controller | `internal/controller/gateway/referencegrant_controller.go` | — |
| Namespace controller | `internal/controller/gateway/namespace_controller.go` | — |
| Route helpers | `internal/controller/gateway/routes.go` | — |
| Gateway API translation | `pkg/managerdriver/translate_gatewayapi.go` | — |
