# Agent Endpoint Controller

> Controller that reconciles AgentEndpoint CRDs by establishing ngrok agent tunnels to upstream services.

<!-- Last updated: 2026-04-08 -->

## Overview

The AgentEndpoint controller runs in the **Agent Manager** binary. It watches `AgentEndpoint` CRDs and establishes ngrok agent tunnels that forward traffic from the ngrok edge network to upstream Kubernetes Services.

## Controllers

| Controller | Primary Resource | Secondary Watches | Owned Resources |
|------------|-----------------|-------------------|-----------------|
| `AgentEndpointReconciler` | `AgentEndpoint` | `NgrokTrafficPolicy`, `Secret`, `Domain` | Agent tunnels (via AgentDriver), Domain (CRD) |

## Reconciliation Logic

**Create/Update:**
1. Calls `DomainManager.EnsureDomainExists()` to reserve the domain. If not ready, requeues after 10 seconds.
2. Resolves the traffic policy — from `spec.trafficPolicy.targetRef` (reference to `NgrokTrafficPolicy`) or from `spec.trafficPolicy.inline` (inline JSON).
3. Resolves client certificates from referenced Secrets (`spec.clientCertificateRefs`). Each Secret must contain `tls.crt` and `tls.key` keys.
4. Calls `AgentDriver.CreateAgentEndpoint()` with a tunnel name (`namespace/name`), the AgentEndpoint spec, resolved traffic policy, and client certificates.
5. Sets `status.assignedURL`, `status.trafficPolicy`, and `status.domainRef`.

**Delete:** Calls `AgentDriver.DeleteAgentEndpoint()` to tear down the tunnel.

**Field Indexing:** The controller creates field indexes on:
- `spec.trafficPolicy.targetRef.name` — to find AgentEndpoints referencing a specific traffic policy.
- `spec.clientCertificateRefs` — to find AgentEndpoints referencing a specific Secret.

**Domain Matching:** When a Domain resource changes, the controller finds related AgentEndpoints by:
1. Checking `status.domainRef` for a direct match.
2. Parsing the AgentEndpoint's URL and comparing the hyphenated domain name to the Domain's name.

### Status Conditions

| Condition | Meaning |
|-----------|---------|
| `EndpointCreated` | Agent tunnel has been established successfully |
| `TrafficPolicyApplied` | Traffic policy was resolved and applied |
| `TrafficPolicyError` | Traffic policy reference not found or invalid |
| `Ready` | Composite: all conditions are healthy |

### Events

| Event Type | Reason | When |
|------------|--------|------|
| `Warning` | `ConfigError` | Invalid traffic policy configuration |
| `Warning` | `SecretNotFound` | Client certificate Secret not found |
| `Warning` | `TrafficPolicyNotFound` | Referenced NgrokTrafficPolicy does not exist |

### Requeue Strategy

| Scenario | Behavior |
|----------|----------|
| Domain not ready | Requeue after 10 seconds |
| Traffic policy config error | No requeue (terminal) |
| Secret not found | Default backoff |
| Agent driver error | Default backoff |

## Agent Driver

The `AgentDriver` (`pkg/agent/driver.go`) manages the lifecycle of ngrok agent tunnels:

- **CreateAgentEndpoint**: Establishes a new tunnel or updates an existing one. The tunnel connects to the upstream URL from the AgentEndpoint spec and forwards traffic from the ngrok edge.
- **DeleteAgentEndpoint**: Tears down the tunnel and releases resources.

The Agent Manager maintains an `EndpointForwarderMap` that tracks active tunnel forwarders keyed by endpoint name (`namespace/name`).

Each instance of the Agent Manager pod establishes its own set of tunnels, enabling horizontal scaling. At `N` replicas, each AgentEndpoint results in `N` agent endpoints in the ngrok API, with traffic load-balanced across them.

## Source References

| Symbol / Concept | File | Lines |
|-----------------|------|-------|
| AgentEndpoint controller | `internal/controller/agent/agent_endpoint_controller.go` | — |
| AgentEndpoint conditions | `internal/controller/agent/agent_endpoint_conditions.go` | — |
| Agent driver | `pkg/agent/driver.go` | — |
| Endpoint forwarder map | `pkg/agent/endpoint_forwarder_map.go` | — |
