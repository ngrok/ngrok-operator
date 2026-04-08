# Specification: ngrok-operator

> The official ngrok Kubernetes Operator — integrates Kubernetes networking primitives (Ingress, Gateway API, Services) with the ngrok platform for secure, production-grade ingress.

<!-- Last updated: 2026-04-08 -->

**Source:** `ngrok/ngrok-operator` (local)

## Specification Files

### Architecture & Data Model

| File | Description |
|------|-------------|
| [architecture.md](architecture.md) | Component map, module responsibilities, and interaction patterns |
| [data-schema.md](data-schema.md) | CRD entity relationships and shared types |

### Controllers

| File | Description |
|------|-------------|
| [controllers/common.md](controllers/common.md) | Shared reconciliation framework (BaseController, finalizers, error handling, drain) |
| [controllers/gateway.md](controllers/gateway.md) | Gateway API controllers (GatewayClass, Gateway, HTTPRoute, TCPRoute, TLSRoute, ReferenceGrant, Namespace) |
| [controllers/ingress.md](controllers/ingress.md) | Ingress controllers (Ingress, Domain, IPPolicy) |
| [controllers/ngrok.md](controllers/ngrok.md) | ngrok controllers (CloudEndpoint, KubernetesOperator, NgrokTrafficPolicy) |
| [controllers/agent.md](controllers/agent.md) | AgentEndpoint controller and agent tunnel driver |
| [controllers/service.md](controllers/service.md) | Service LoadBalancer controller |
| [controllers/bindings.md](controllers/bindings.md) | Bindings controllers (BoundEndpoint, Forwarder, Poller) |

### Custom Resource Definitions

| File | Description |
|------|-------------|
| [crds/common.md](crds/common.md) | Shared types and enums across API groups |
| [crds/ngrok/agentendpoint.md](crds/ngrok/agentendpoint.md) | AgentEndpoint CRD — agent tunnel endpoints |
| [crds/ngrok/cloudendpoint.md](crds/ngrok/cloudendpoint.md) | CloudEndpoint CRD — cloud-managed endpoints |
| [crds/ngrok/kubernetesoperator.md](crds/ngrok/kubernetesoperator.md) | KubernetesOperator CRD — operator registration and drain lifecycle |
| [crds/ngrok/ngroktrafficpolicy.md](crds/ngrok/ngroktrafficpolicy.md) | NgrokTrafficPolicy CRD — reusable traffic policy definitions |
| [crds/ingress/domain.md](crds/ingress/domain.md) | Domain CRD — reserved domain management |
| [crds/ingress/ippolicy.md](crds/ingress/ippolicy.md) | IPPolicy CRD — IP-based access control |
| [crds/bindings/boundendpoint.md](crds/bindings/boundendpoint.md) | BoundEndpoint CRD — ngrok endpoints bound to Kubernetes Services |

### Product-Specific Abstractions

| File | Description |
|------|-------------|
| [ir.md](ir.md) | Intermediate Representation layer for Ingress/Gateway-to-endpoint translation |
| [traffic-policy.md](traffic-policy.md) | Traffic policy DSL types and action builders |
| [ngrok-api-client.md](ngrok-api-client.md) | ngrok API client abstraction (Clientset) |
| [domain-manager.md](domain-manager.md) | Domain CRD lifecycle management |
| [drain.md](drain.md) | Drain orchestration for graceful operator uninstall |

### Deployment & Configuration

| File | Description |
|------|-------------|
| [helm.md](helm.md) | Helm chart structure and configurable parameters |
| [configuration.md](configuration.md) | CLI commands, flags, and runtime configuration |

## Pending Clarifications

- [ ] `controllers/bindings.md` — The bindings feature is marked as in-development. Exact GA timeline and feature completeness are unclear.
- [ ] `configuration.md` — The `oneClickDemoMode` flag's full behavior beyond skipping required fields is not fully documented in the code.
