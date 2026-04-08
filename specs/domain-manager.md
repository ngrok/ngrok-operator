# Domain Manager

> Manages the lifecycle of Domain CRDs on behalf of endpoint controllers.

<!-- Last updated: 2026-04-08 -->

## Overview

The Domain Manager (`internal/domain/manager.go`) is a shared component used by the CloudEndpoint and AgentEndpoint controllers to ensure that a Domain CRD exists for each endpoint's hostname. It handles domain creation, readiness checking, condition propagation, and cleanup of stale domains.

## Key Operations

### EnsureDomainExists

Called by endpoint controllers during reconciliation. Given an `EndpointWithDomain` (the interface implemented by both AgentEndpoint and CloudEndpoint):

1. **Parse and validate the URL** from the endpoint's spec.
2. **Check for skipped domains** — the following do not need a domain reservation:
   - TCP endpoints (`tcp://` scheme) — set `DomainReady` condition to True, return.
   - Internal endpoints (`.internal` TLD) — set condition, return.
   - Kubernetes-bound endpoints (`bindings: ["kubernetes"]`) — delete any stale domain, set condition, return.
   - Internal-bound endpoints (`bindings: ["internal"]`) — set condition, return.
3. **Get or create the Domain CRD**:
   - Look up an existing Domain by the hyphenated domain name in the endpoint's namespace.
   - If found, check its Ready condition and propagate it to the endpoint's `DomainReady` condition.
   - If not found, create a new Domain CRD with the domain name, optional reclaim policy, and controller labels.
4. **Set `domainRef`** on the endpoint status pointing to the Domain CRD.
5. **Return a `DomainResult`** indicating readiness. If not ready, the caller (endpoint controller) should requeue.

### Condition Propagation

The Domain Manager propagates the Domain's `Ready` condition to the endpoint as a `DomainReady` condition:

| Domain State | Endpoint DomainReady | Reason |
|-------------|---------------------|--------|
| Domain exists and Ready | `True` | `DomainReady` |
| Domain exists, not Ready | `False` | Propagated from Domain's Ready condition |
| Domain being created | `False` | `DomainCreating` |
| Domain skipped (TCP/internal/binding) | `True` | `DomainReady` with descriptive message |
| Error checking domain | `False` | `NgrokAPIError` |

### Stale Domain Cleanup

When an endpoint transitions to a `kubernetes` binding, the Domain Manager deletes any previously created Domain CRD for that endpoint (since Kubernetes-bound endpoints don't need domain reservations).

## Configuration Options

| Option | Description |
|--------|-------------|
| `WithDefaultDomainReclaimPolicy(policy)` | Sets the default reclaim policy (`Delete` or `Retain`) for newly created Domains |
| `WithControllerLabels(clv)` | Sets controller labels to apply to created Domain CRDs |

## Source References

| Symbol / Concept | File | Lines |
|-----------------|------|-------|
| Manager struct | `internal/domain/manager.go` | 73–78 |
| EnsureDomainExists | `internal/domain/manager.go` | 99–111 |
| checkSkippedDomains | `internal/domain/manager.go` | 125–179 |
| getOrCreateDomain | `internal/domain/manager.go` | 182–203 |
| setDomainCondition | `internal/domain/manager.go` | 302–317 |
