# ngrok-operator Specifications

This directory contains the v1 specifications for the ngrok-operator. These specs document all public-facing behavior, APIs, and configuration — serving as the authoritative reference for what the operator does and how it behaves.

## Purpose

1. **Implementation planning**: Provides a baseline for planning changes as we work toward v1.
2. **Bug vs. unspecified behavior**: Defines expected behavior so we can distinguish bugs from unspecified behavior.
3. **Living documentation**: Serves as the canonical reference for the operator's public surface area.

## Glossary

| Term | Definition | Details |
|------|-----------|---------|
| **Agent Endpoint** | An endpoint backed by an ngrok agent tunnel running inside the cluster. Created as an `AgentEndpoint` CR. | [crds/agentendpoint.md](crds/agentendpoint.md) |
| **Bindings** | A feature that projects external ngrok endpoints into the cluster as local Kubernetes Services, enabling ngrok-to-cluster traffic. | [features/bindings.md](features/bindings.md) |
| **Cloud Endpoint** | An endpoint managed entirely in the ngrok cloud (no local agent tunnel). Created as a `CloudEndpoint` CR. | [crds/cloudendpoint.md](crds/cloudendpoint.md) |
| **Drain / Draining** | The process of gracefully removing ngrok API resources when the operator is uninstalled. Triggered by deletion of the `KubernetesOperator` CR. | [features/draining.md](features/draining.md) |
| **Driver** | An internal pattern used by Ingress and Gateway API controllers that collects state from multiple resources and materializes the combined state as ngrok endpoints. | [controllers/ingress.md](controllers/ingress.md) |
| **Endpoint Pooling** | Allows multiple endpoints to share the same public URL, distributing traffic across them. Controlled via the `ngrok.com/pooling-enabled` annotation. | [annotations.md](annotations.md) |
| **Mapping Strategy** | Controls which ngrok endpoint resources are created: `endpoints` (AgentEndpoint only) or `endpoints-verbose` (CloudEndpoint + internal AgentEndpoint). | [annotations.md](annotations.md) |
| **Traffic Policy** | A set of rules (rate limiting, header manipulation, authentication, etc.) applied to ngrok endpoints. Defined as an `TrafficPolicy` CR or inline JSON. | [features/traffic-policy.md](features/traffic-policy.md) |

## Directory Structure

### Top-Level Specs

- [authentication.md](authentication.md) — Credentials, secrets, API key/authtoken management
- [annotations.md](annotations.md) — Central reference for all `ngrok.com/` annotations
- [design-decisions.md](design-decisions.md) — Settled architectural trade-offs and their rationale

### [rbac/](rbac/) — RBAC Configuration

- [README.md](rbac/README.md) — RBAC model overview and design principles
- [operator.md](rbac/operator.md) — Operator (api-manager) permissions
- [agent.md](rbac/agent.md) — Agent permissions
- [bindings-forwarder.md](rbac/bindings-forwarder.md) — Bindings forwarder permissions
- [aggregation.md](rbac/aggregation.md) — Editor/viewer aggregation roles

### [helm/](helm/) — Helm Chart Configuration

- [common.md](helm/common.md) — Top-level structure, global defaults, ngrok config, credentials, config file system
- [operator.md](helm/operator.md) — API manager (`apiManager`) deployment values
- [agent.md](helm/agent.md) — Agent deployment values
- [bindings-forwarder.md](helm/bindings-forwarder.md) — Bindings forwarder (`bindingsForwarder`) deployment values
- [features.md](helm/features.md) — Feature flags, feature config, drain/domain policies, cleanup hook

### [features/](features/) — Cross-Cutting Features

- [draining.md](features/draining.md) — Drain policy, cleanup hook, uninstall behavior
- [multi-install.md](features/multi-install.md) — Multiple operator installations in one cluster
- [ingress.md](features/ingress.md) — Kubernetes Ingress feature
- [gateway-api.md](features/gateway-api.md) — Kubernetes Gateway API feature
- [bindings.md](features/bindings.md) — Endpoint bindings feature
- [high-availability.md](features/high-availability.md) — Replicas, leader election, PDB
- [traffic-policy.md](features/traffic-policy.md) — Traffic policy resolution across controllers
- [namespace-watching.md](features/namespace-watching.md) — Namespace scoping configuration

### [crds/](crds/) — Custom Resource Definitions

- [common.md](crds/common.md) — Shared patterns: conditions, finalizers, shared types
- [agentendpoint.md](crds/agentendpoint.md) — AgentEndpoint (`ngrok.com`)
- [cloudendpoint.md](crds/cloudendpoint.md) — CloudEndpoint (`ngrok.com`)
- [kubernetesoperator.md](crds/kubernetesoperator.md) — KubernetesOperator (`ngrok.com`)
- [trafficpolicy.md](crds/trafficpolicy.md) — TrafficPolicy (`ngrok.com`)
- [domain.md](crds/domain.md) — Domain (`ngrok.com`)
- [ippolicy.md](crds/ippolicy.md) — IPPolicy (`ngrok.com`)
- [boundendpoint.md](crds/boundendpoint.md) — BoundEndpoint (`ngrok.com`)

### [controllers/](controllers/) — Controller Behavior

- [common.md](controllers/common.md) — Base controller pattern, error handling, drain awareness
- [service.md](controllers/service.md) — Service LoadBalancer controller
- [ingress.md](controllers/ingress.md) — Ingress controller
- [agentendpoint.md](controllers/agentendpoint.md) — AgentEndpoint controller
- [cloudendpoint.md](controllers/cloudendpoint.md) — CloudEndpoint controller
- [kubernetesoperator.md](controllers/kubernetesoperator.md) — KubernetesOperator controller
- [trafficpolicy.md](controllers/trafficpolicy.md) — TrafficPolicy controller
- [domain.md](controllers/domain.md) — Domain controller
- [ippolicy.md](controllers/ippolicy.md) — IPPolicy controller
- [boundendpoint.md](controllers/boundendpoint.md) — BoundEndpoint controller
- [bindings-forwarder.md](controllers/bindings-forwarder.md) — Bindings Forwarder controller
- **[gateway-api/](controllers/gateway-api/)** — Gateway API controllers
  - [gatewayclass.md](controllers/gateway-api/gatewayclass.md)
  - [gateway.md](controllers/gateway-api/gateway.md)
  - [httproute.md](controllers/gateway-api/httproute.md)
  - [tcproute.md](controllers/gateway-api/tcproute.md)
  - [tlsroute.md](controllers/gateway-api/tlsroute.md)
  - [referencegrant.md](controllers/gateway-api/referencegrant.md)
