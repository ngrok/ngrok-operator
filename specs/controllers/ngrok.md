# ngrok Controllers

> Controllers that reconcile ngrok-specific CRDs: CloudEndpoint, KubernetesOperator, and NgrokTrafficPolicy.

<!-- Last updated: 2026-04-08 -->

## Overview

The ngrok controller group manages the operator's core CRDs that interface directly with the ngrok API. These controllers are registered by the **API Manager** binary.

## Controllers

| Controller | Primary Resource | Secondary Watches | Owned Resources |
|------------|-----------------|-------------------|-----------------|
| `CloudEndpointReconciler` | `CloudEndpoint` | `NgrokTrafficPolicy`, `Domain` | Endpoint (ngrok API), Domain (CRD) |
| `KubernetesOperatorReconciler` | `KubernetesOperator` | — | KubernetesOperator (ngrok API), TLS Secret |
| `NgrokTrafficPolicyReconciler` | `NgrokTrafficPolicy` | — | — (validation only) |

## Reconciliation Logic

### CloudEndpointReconciler

Reconciles `CloudEndpoint` CRDs against the ngrok Endpoints API. Uses the Domain Manager to ensure the endpoint's domain is reserved before creating the endpoint.

**Create:**
1. Calls `DomainManager.EnsureDomainExists()` to reserve the domain. If the domain is not yet ready, requeues after 10 seconds.
2. Resolves the traffic policy — either from `spec.trafficPolicyName` (a reference to a `NgrokTrafficPolicy` CRD) or from `spec.trafficPolicy` (inline definition).
3. Creates the endpoint in the ngrok API with URL, description, metadata, traffic policy, bindings, and pooling configuration.
4. Sets `status.id` and `status.domainRef`.

**Update:**
1. Re-ensures domain exists and resolves traffic policy.
2. Updates the endpoint in the ngrok API.
3. If the endpoint is not found by ID (deleted externally), clears `status.id` and triggers a create on the next reconcile.

**Delete:** Deletes the endpoint from the ngrok API.

**Status conditions:**
- `CloudEndpointCreated` — endpoint exists in ngrok API.
- `Ready` — composite condition including domain readiness.

**Events:**
- `ConfigError` (Warning) — invalid traffic policy configuration.
- `EndpointNotFound` (Warning) — endpoint not found by ID during update.

**Requeue strategy:** Domain not ready → 10s requeue. Traffic policy config errors → no requeue (terminal).

### KubernetesOperatorReconciler

Reconciles the `KubernetesOperator` CRD (typically a singleton) against the ngrok KubernetesOperators API. This controller manages operator registration, TLS certificate provisioning, and the drain workflow.

**Create:**
1. Searches for an existing KubernetesOperator in the ngrok API by matching deployment name/namespace and namespace UID in metadata.
2. If bindings are enabled, generates a P256 ECDSA CSR and creates a TLS Secret.
3. Creates or adopts the KubernetesOperator in the ngrok API with enabled features, region, deployment info, binding configuration, and CSR.
4. Stores the returned TLS certificate in the Secret.

**Update:**
1. Updates the KubernetesOperator in the ngrok API with current configuration.
2. Refreshes the TLS certificate if needed.

**Delete:**
1. Initiates the drain workflow via the `DrainOrchestrator`.
2. Updates `status.drainStatus` (`pending` → `draining` → `completed`/`failed`), `status.drainProgress` (format: `X/Y`), and `status.drainErrors`.
3. Once drain completes, deletes the KubernetesOperator from the ngrok API.
4. Removes the finalizer.

**Status fields:**
- `id`, `uri` — ngrok API identifiers.
- `registrationStatus` — `pending`, `registered`, or `error`.
- `registrationErrorCode` — pattern `ERR_NGROK_XXXX`.
- `enabledFeatures` — string representation of enabled features.
- `drainStatus`, `drainProgress`, `drainMessage`, `drainErrors` — drain workflow state.

**Requeue strategy:** Drain in progress → requeue. Not found errors → clear ID and recreate.

### NgrokTrafficPolicyReconciler

Validates `NgrokTrafficPolicy` CRDs and triggers endpoint re-syncs when policies change.

**Reconciliation:**
1. Parses the JSON traffic policy from `spec.policy`.
2. Validates syntax — checks for unknown top-level keys.
3. Warns about legacy directives (`inbound`/`outbound` vs. `on_tcp_connect`/`on_http_request`/`on_http_response`).
4. Warns about the deprecated `enabled` field.
5. Calls `Driver.SyncEndpoints()` to propagate changes to all endpoints referencing this policy.

**Events:**
- `TrafficPolicyParseFailed` (Warning) — malformed policy JSON.
- `PolicyDeprecation` (Warning) — legacy directives or deprecated fields.

**Requeue strategy:** Sync result handled by `managerdriver.HandleSyncResult()`.

## Source References

| Symbol / Concept | File | Lines |
|-----------------|------|-------|
| CloudEndpoint controller | `internal/controller/ngrok/cloudendpoint_controller.go` | — |
| KubernetesOperator controller | `internal/controller/ngrok/kubernetesoperator_controller.go` | — |
| NgrokTrafficPolicy controller | `internal/controller/ngrok/ngroktrafficpolicy_controller.go` | — |
| Domain Manager | `internal/domain/manager.go` | — |
| Drain orchestrator | `internal/drain/orchestrator.go` | — |
