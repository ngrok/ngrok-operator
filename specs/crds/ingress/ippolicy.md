# IPPolicy

> Defines IP-based access control rules (allow/deny by CIDR) managed through the ngrok API.

<!-- Last updated: 2026-04-08 -->

## Overview

An `IPPolicy` CRD creates an IP policy in the ngrok platform with a set of CIDR-based allow/deny rules. IP policies can be referenced from traffic policy actions (`restrict-ips`) to control access to endpoints.

**API Group:** `ingress.k8s.ngrok.com`
**Version:** `v1alpha1`
**Kind:** `IPPolicy`
**Scope:** Namespaced

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `description` | `string` | No | `"Created by kubernetes-ingress-controller"` | Human-readable description |
| `metadata` | `string` | No | `"{"owned-by":"kubernetes-ingress-controller"}"` | JSON string of arbitrary data |
| `rules` | `[]IPPolicyRule` | Yes | — | List of CIDR rules |

### IPPolicyRule

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `description` | `string` | No | Rule description |
| `metadata` | `string` | No | JSON arbitrary data |
| `cidr` | `string` | Yes | CIDR range (e.g., `10.0.0.0/8`, `0.0.0.0/0`) |
| `action` | `string` | Yes | `allow` or `deny` |

## Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | `string` | ngrok API policy identifier |
| `conditions` | `[]metav1.Condition` | Standard Kubernetes conditions |
| `rules` | `[]IPPolicyRuleStatus` | Status of each rule with `id`, `cidr`, `action` |

## Rule Reconciliation

The controller performs diff-based rule management to minimize API calls and maintain security invariants:

1. Groups rules by action (allow/deny) and CIDR.
2. **Creates** new deny rules first — ensures restrictive rules are in place before permissive ones are removed.
3. **Replaces** modified deny rules, then modified allow rules.
4. **Creates** new allow rules.
5. **Deletes** stale rules.
6. **Updates** rules with changed descriptions/metadata.

This ordering prevents a window where traffic could bypass IP restrictions during reconciliation.

## Source References

| Symbol / Concept | File | Lines |
|-----------------|------|-------|
| IPPolicy types | `api/ingress/v1alpha1/ippolicy_types.go` | — |
| IPPolicy controller | `internal/controller/ingress/ippolicy_controller.go` | — |
