# Domain

> Represents a reserved domain in the ngrok platform, providing DNS and TLS certificate management.

<!-- Last updated: 2026-04-08 -->

## Overview

A `Domain` CRD reserves a domain name in the ngrok platform. Domains are typically created automatically by the Domain Manager when CloudEndpoint or AgentEndpoint resources reference a hostname that requires a domain reservation. Users can also create Domain resources directly.

**API Group:** `ingress.k8s.ngrok.com`
**Version:** `v1alpha1`
**Kind:** `Domain`
**Scope:** Namespaced

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `domain` | `string` | Yes | — | The domain name to reserve |
| `description` | `string` | No | `"Created by kubernetes-ingress-controller"` | Human-readable description |
| `metadata` | `string` | No | `"{"owned-by":"kubernetes-ingress-controller"}"` | JSON string of arbitrary data |
| `region` | `string` | No | — | Region to reserve domain in |
| `resolves_to` | `*[]DomainResolvesToEntry` | No | — | List of resolving targets |
| `reclaimPolicy` | `DomainReclaimPolicy` | No | `Delete` | What happens to the ngrok API resource on CRD deletion: `Delete` or `Retain` |

## Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | `string` | ngrok API domain identifier |
| `domain` | `string` | The reserved domain name |
| `region` | `string` | Region where the domain was created |
| `resolves_to` | `*[]DomainResolvesToEntry` | Current resolving targets |
| `cnameTarget` | `*string` | CNAME target for custom domains (users must create a CNAME DNS record pointing to this value) |
| `acmeChallengeCnameTarget` | `*string` | CNAME target for ACME challenge (wildcard domains) |
| `certificate.id` | `string` | Certificate identifier |
| `certificateManagementPolicy.authority` | `string` | Certificate authority (e.g., `letsencrypt`) |
| `certificateManagementPolicy.privateKeyType` | `string` | Private key type (e.g., `ecdsa`) |
| `certificateManagementStatus.renewsAt` | `*metav1.Time` | When the certificate renews |
| `certificateManagementStatus.provisioningJob` | `*DomainStatusProvisioningJob` | Current provisioning info |
| `conditions` | `[]metav1.Condition` | Standard Kubernetes conditions |

## Naming Convention

Domain CRD names use a hyphenated form of the domain name. For example:
- `example.com` → `example-com`
- `app.example.com` → `app-example-com`
- `*.example.com` → `star-example-com`

This is computed by `ingressv1alpha1.HyphenatedDomainNameFromURL()`.

## Lifecycle

Internal domains (`.internal` TLD) skip the ngrok API entirely — the controller removes the finalizer without creating a reserved domain.

For public domains:
1. **Create**: Reserves the domain via the ngrok API. Certificate provisioning begins automatically.
2. **Ready**: Domain is reserved and certificate is provisioned.
3. **Update**: Only sends API calls when `description`, `metadata`, or `resolvesTo` change.
4. **Delete**:
   - `reclaimPolicy: Delete` → Deletes the reserved domain from the ngrok API.
   - `reclaimPolicy: Retain` → Removes the finalizer without API deletion, preserving the domain.

The Domain controller uses a custom rate limiter with exponential backoff (30s base, 10m max) to handle certificate provisioning delays.

## Relationships

| Related Resource | Relationship | Description |
|-----------------|--------------|-------------|
| `AgentEndpoint` | Referenced by | Via `status.domainRef` |
| `CloudEndpoint` | Referenced by | Via `status.domainRef` |

## Source References

| Symbol / Concept | File | Lines |
|-----------------|------|-------|
| Domain types | `api/ingress/v1alpha1/domain_types.go` | — |
| Domain controller | `internal/controller/ingress/domain_controller.go` | — |
| Domain Manager | `internal/domain/manager.go` | — |
