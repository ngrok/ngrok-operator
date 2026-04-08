# NgrokTrafficPolicy

> Reusable traffic policy configuration that can be referenced by CloudEndpoint and AgentEndpoint resources.

<!-- Last updated: 2026-04-08 -->

## Overview

An `NgrokTrafficPolicy` stores a raw JSON traffic policy document that defines rules for handling traffic at ngrok endpoints. It serves as a shared, reusable policy that multiple endpoints can reference by name, avoiding inline duplication.

**API Group:** `ngrok.k8s.ngrok.com`
**Version:** `v1alpha1`
**Kind:** `NgrokTrafficPolicy`
**Scope:** Namespaced

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `policy` | `json.RawMessage` | No | — | Raw JSON-encoded traffic policy |

## Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `policy` | `json.RawMessage` | The policy as applied to the ngrok API |

## Traffic Policy Structure

The JSON policy document supports three phase hooks:

| Phase | Key | Description |
|-------|-----|-------------|
| HTTP Request | `on_http_request` | Rules applied when an HTTP request is received |
| HTTP Response | `on_http_response` | Rules applied when an HTTP response is returned |
| TCP Connect | `on_tcp_connect` | Rules applied when a TCP connection is established |

Each phase contains an array of `Rule` objects with:
- `name` — optional rule identifier.
- `expressions` — CEL expressions that must all match for the rule to apply.
- `actions` — ordered list of actions to execute when expressions match.

### Supported Action Types

| Action | Phase(s) | Description |
|--------|----------|-------------|
| `add-headers` | Request, Response | Add headers |
| `remove-headers` | Request, Response | Remove headers |
| `basic-auth` | Request | HTTP Basic Authentication |
| `circuit-breaker` | Request | Circuit breaker pattern |
| `compress-response` | Response | Response compression |
| `custom-response` | Request | Return a hard-coded response |
| `deny` | Request | Deny the request |
| `forward-internal` | Request | Forward to an internal endpoint |
| `jwt-validation` | Request | JWT token validation |
| `log` | Request, Response, TCP | Logging |
| `set-vars` | Request, Response, TCP | Set variables |
| `oauth` | Request | OAuth provider authentication |
| `openid-connect` | Request | OIDC provider authentication |
| `rate-limit` | Request | Rate limiting |
| `redirect` | Request | URL redirect |
| `restrict-ips` | Request, Response, TCP | IP-based access control |
| `terminate-tls` | TCP | TLS termination configuration |
| `url-rewrite` | Request | URL path/host rewriting |
| `verify-webhook` | Request | Webhook signature verification |

## Validation

The controller validates the policy JSON on each reconcile:
- Unknown top-level keys (anything other than `on_http_request`, `on_http_response`, `on_tcp_connect`) cause a warning event.
- Legacy directives (`inbound`, `outbound`) trigger a deprecation warning.
- The deprecated `enabled` field triggers a deprecation warning.

## Relationships

| Related Resource | Relationship | Description |
|-----------------|--------------|-------------|
| `AgentEndpoint` | Referenced by | Via `spec.trafficPolicy.targetRef.name` |
| `CloudEndpoint` | Referenced by | Via `spec.trafficPolicyName` |

When an `NgrokTrafficPolicy` is updated, the controller triggers a re-sync of all endpoints referencing it.

## Source References

| Symbol / Concept | File | Lines |
|-----------------|------|-------|
| NgrokTrafficPolicy types | `api/ngrok/v1alpha1/ngroktrafficpolicy_types.go` | — |
| NgrokTrafficPolicy controller | `internal/controller/ngrok/ngroktrafficpolicy_controller.go` | — |
| TrafficPolicy DSL | `internal/trafficpolicy/policy.go` | 1–73 |
| Action types | `internal/trafficpolicy/policy.go` | 17–37 |
