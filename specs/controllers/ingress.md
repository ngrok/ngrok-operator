# Ingress Controllers

> Controllers that reconcile Kubernetes Ingress resources and ngrok ingress-group CRDs (Domain, IPPolicy).

<!-- Last updated: 2026-04-08 -->

## Overview

The Ingress controller group handles standard Kubernetes Ingress resources and the operator's ingress-group CRDs (`Domain`, `IPPolicy`). The Ingress controller translates Ingress rules into ngrok endpoints via the Manager Driver, while the Domain and IPPolicy controllers directly reconcile their CRDs against the ngrok API.

All Ingress controllers are registered by the **API Manager** binary. The Ingress controller is gated behind `--enable-feature-ingress` (enabled by default).

## Controllers

| Controller | Primary Resource | Secondary Watches | Owned Resources |
|------------|-----------------|-------------------|-----------------|
| `IngressReconciler` | `Ingress` | `IngressClass`, `Service`, `Domain`, `NgrokTrafficPolicy` | Ingress (status/finalizers) |
| `DomainReconciler` | `Domain` | — | ReservedDomain (ngrok API) |
| `IPPolicyReconciler` | `IPPolicy` | — | IPPolicy + IPPolicyRules (ngrok API) |

## Reconciliation Logic

### IngressReconciler

Validates that the Ingress belongs to the ngrok ingress class before processing. Ingress class matching checks:
1. If `spec.ingressClassName` is set, it must match one of the operator's IngressClass resources (where `spec.controller` matches the configured controller name).
2. If `spec.ingressClassName` is unset, the operator handles the Ingress only if one of its IngressClass resources has the `ingressclass.kubernetes.io/is-default-class: "true"` annotation.

Ingress validation requires:
- At least one rule.
- Each rule must have a non-empty `host`.
- Each rule must have an `http` section.

On success, the controller updates the Driver store and calls `Driver.Sync()` to translate Ingress rules into CloudEndpoint/AgentEndpoint CRDs.

**Events:**
- `NoDefaultIngressClassFound` (Warning) — no ngrok IngressClass exists.
- `InvalidIngressSpec` (Warning) — Ingress fails validation.

**Requeue strategy:** Driven by `managerdriver.HandleSyncResult()`.

### DomainReconciler

Reconciles `Domain` CRDs against the ngrok ReservedDomain API using the `BaseController` pattern.

**Create:** Reserves a domain via the ngrok API with domain name, region, description, and metadata. Sets status fields including `id`, `domain`, `cnameTarget`, certificate info, and management policy.

**Update:** Checks if `description`, `metadata`, or `resolvesTo` have changed and updates the ngrok API resource only if needed.

**Delete:** Respects the `spec.reclaimPolicy`:
- `Delete` (default): Deletes the reserved domain from the ngrok API.
- `Retain`: Removes the finalizer without deleting the ngrok API resource.

Internal domains (`.internal` TLD) are skipped entirely — the controller removes the finalizer and returns without calling the ngrok API.

**Status conditions:** Domain readiness based on ngrok API state. Certificate provisioning status is tracked via `status.certificateManagementStatus`.

**Requeue strategy:** Custom exponential backoff rate limiter (30s base, 10m max) for certificate provisioning waits. Retryable errors (codes 446, 511) are propagated.

### IPPolicyReconciler

Reconciles `IPPolicy` CRDs against the ngrok IPPolicy and IPPolicyRule APIs using the `BaseController` pattern.

**Create:** Creates the IP policy, then creates all rules (CIDR + action pairs).

**Update:** Performs a diff-based reconciliation of rules:
1. Groups existing and desired rules by action (allow/deny) and CIDR.
2. Creates new deny rules first, then processes replacements, then creates new allow rules, then deletes stale rules, then updates modified rules.
3. This ordering ensures deny rules are always in place before allow rules are removed.

**Delete:** Deletes the IP policy from the ngrok API (rules are cascade-deleted).

**Status conditions:**
- `IPPolicyCreated` — policy exists in ngrok API.
- `IPPolicyRulesConfigured` — all rules match desired state.
- `Ready` — composite of the above.

**Requeue strategy:** Invalid CIDR errors are terminal (no requeue). Other errors use default backoff.

## Source References

| Symbol / Concept | File | Lines |
|-----------------|------|-------|
| Ingress controller | `internal/controller/ingress/ingress_controller.go` | — |
| Domain controller | `internal/controller/ingress/domain_controller.go` | — |
| IPPolicy controller | `internal/controller/ingress/ippolicy_controller.go` | — |
| Store ingress filtering | `internal/store/store.go` | 350–413 |
| Ingress translation | `pkg/managerdriver/translate_ingresses.go` | — |
