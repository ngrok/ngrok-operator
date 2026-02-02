# Uninstall E2E Tests

End-to-end tests for `helm uninstall` drain behavior.

## Key Concepts

### Drain Process

When `helm uninstall` runs, a pre-delete hook triggers the drain:
1. Sets `KubernetesOperator` to draining state
2. Processes resources based on `drainPolicy`:
   - **Delete**: Deletes CRs → controllers clean up ngrok API resources
   - **Retain**: Removes finalizers only → ngrok API resources preserved
3. Deregisters `KubernetesOperator` from ngrok API
4. Helm completes uninstall

### Endpoint Types (CRITICAL!)

| Endpoint Type | Persists in ngrok API? | Created By |
|--------------|------------------------|------------|
| **CloudEndpoint** | ✅ Yes (API resource) | Explicit CloudEndpoint CR, or advanced Ingress/Gateway configs |
| **AgentEndpoint** | ❌ No (session-based) | Agent pod; dies when pod stops |

**This is the most important concept for test assertions:**
- CloudEndpoints are retained in ngrok API with `Retain` policy
- AgentEndpoints ALWAYS disappear when the operator stops (regardless of policy)

## Fixture → Endpoint Mapping

| Fixture | K8s Resource | Creates in Cluster | ngrok API Endpoint | Persists After Uninstall? |
|---------|--------------|-------------------|-------------------|--------------------------|
| `cloudendpoint.yaml` | CloudEndpoint CR | CloudEndpoint | CloudEndpoint | ✅ Yes (with Retain) |
| `agentendpoint.yaml` | AgentEndpoint CR | AgentEndpoint | AgentEndpoint | ❌ No (session-based) |
| `ingress.yaml` | Ingress | AgentEndpoint (driver-generated) | AgentEndpoint | ❌ No (session-based) |
| `gateway.yaml` + `httproute.yaml` | Gateway + HTTPRoute | AgentEndpoint (driver-generated) | AgentEndpoint | ❌ No (session-based) |
| `cloudendpoint-k8s-binding.yaml` | CloudEndpoint w/ kubernetes binding | CloudEndpoint + BoundEndpoint + Service | CloudEndpoint | ✅ Yes (with Retain) |

### Endpoint URLs by Fixture

**Single-operator tests (`uninstall-test` namespace):**
| Fixture | Endpoint URL | Type | Retained? |
|---------|--------------|------|-----------|
| `cloudendpoint.yaml` | `uninstall-test-cloud-ep.internal` | Cloud | ✅ |
| `agentendpoint.yaml` | `uninstall-test-agent-ep.internal` | Agent | ❌ |
| `ingress.yaml` | `uninstall-test-ingress.internal` | Agent | ❌ |
| `gateway.yaml` | `uninstall-test-gateway.internal` | Agent | ❌ |
| `cloudendpoint-k8s-binding.yaml` | `bound-test-svc.bound-target-ns` | Cloud | ✅ |

**Multi-operator tests:**
| Fixture | Endpoint URL | Type | Retained? |
|---------|--------------|------|-----------|
| `cloudendpoint-a.yaml` | `uninstall-test-cloud-ep-a.internal` | Cloud | ✅ |
| `cloudendpoint-b.yaml` | `uninstall-test-cloud-ep-b.internal` | Cloud | ✅ |
| `ingress-a.yaml` | `uninstall-test-ingress-a.internal` | Agent | ❌ |
| `ingress-b.yaml` | `uninstall-test-ingress-b.internal` | Agent | ❌ |

## Test Skeleton

Every test follows this structure:

```
PHASE 0: Cleanup
  └── cleanup-cluster.sh - Remove leftover CRDs, helm releases, namespaces

PHASE 1: Setup
  ├── Install Gateway API CRDs (if needed)
  ├── Install ngrok CRDs (if separate)
  ├── Build and load operator image
  ├── helm install operator
  ├── Assert KubernetesOperator registered (status.registrationStatus=registered)
  └── Capture KubernetesOperator ID to temp file

PHASE 2: Create Resources
  ├── Create fixtures (CloudEndpoint, AgentEndpoint, Ingress, Gateway, etc.)
  ├── Assert resources have finalizers (k8s.ngrok.com/finalizer)
  └── Assert endpoints exist in ngrok API

PHASE 3: Uninstall
  └── helm uninstall (triggers drain process)

PHASE 4: Assert Drain Results
  ├── Verify finalizers removed from all resources
  ├── Verify CRDs removed (bundled) or persist (separate)
  ├── Verify CRs deleted (Delete policy) or exist without finalizers (Retain policy)
  ├── Verify ngrok API state:
  │   ├── DELETE policy: ALL endpoints absent
  │   └── RETAIN policy: CloudEndpoints exist, AgentEndpoints absent
  └── Verify KubernetesOperator deregistered from ngrok API

PHASE 5: Cleanup Retained (Retain policy only)
  └── Delete retained CloudEndpoints from ngrok API

PHASE 6: Final Cleanup
  ├── Uninstall CRD chart (if separate)
  └── Delete namespaces
```

## Test Scenarios

### Single Operator

| Scenario | Drain Policy | CRDs | What's Retained in ngrok API? |
|----------|--------------|------|------------------------------|
| `delete-policy-bundled-crds` | Delete | Bundled | Nothing |
| `retain-policy-bundled-crds` | Retain | Bundled | CloudEndpoint only |
| `delete-policy-separate-crds` | Delete | Separate | Nothing |
| `retain-policy-separate-crds` | Retain | Separate | CloudEndpoint only |

### BoundEndpoint (kubernetes bindings)

| Scenario | Drain Policy | What's Retained? |
|----------|--------------|------------------|
| `bindings-delete-policy` | Delete | Nothing |
| `bindings-retain-policy` | Retain | CloudEndpoint with kubernetes binding |

### Multi-Operator

All multi-operator tests use **separate CRDs** and **namespace scoping**.

| Scenario | Drain Policy | Description |
|----------|--------------|-------------|
| `multi-ns-delete-policy` | Delete | Uninstall operator-a; operator-b unaffected |
| `multi-ns-retain-policy` | Retain | Uninstall operator-a; operator-b unaffected |
| `multi-ingressclass-delete-policy` | Delete | Same + different ingress classes |
| `multi-ingressclass-retain-policy` | Retain | Same + different ingress classes |

> **⚠️ Multi-operator requires `watchNamespace`!** Without it, operators fight over each other's resources.

## Expected Assertions by Policy

### Delete Policy
```bash
# ALL endpoints should be absent
ngrok-api-helper.sh endpoint absent "uninstall-test-cloud-ep.internal"
ngrok-api-helper.sh endpoint absent "uninstall-test-agent-ep.internal"
ngrok-api-helper.sh endpoint absent "uninstall-test-ingress.internal"
ngrok-api-helper.sh endpoint absent "uninstall-test-gateway.internal"
```

### Retain Policy
```bash
# CloudEndpoints should EXIST (retained)
ngrok-api-helper.sh endpoint exists "uninstall-test-cloud-ep.internal"

# AgentEndpoints should be ABSENT (always disappear when agent stops)
ngrok-api-helper.sh endpoint absent "uninstall-test-agent-ep.internal"
ngrok-api-helper.sh endpoint absent "uninstall-test-ingress.internal"
ngrok-api-helper.sh endpoint absent "uninstall-test-gateway.internal"
```

## Running Tests

```bash
# Run specific scenario
make e2e-uninstall SCENARIO=delete-policy-bundled-crds

# Run with debug mode (pause on failure)
make e2e-uninstall SCENARIO=delete-policy-bundled-crds DEBUG=1

# Run all scenarios
make e2e-uninstall-all

# Cleanup after failed test
make e2e-clean-uninstall
```

## Directory Structure

```
tests/chainsaw-uninstall/
├── _fixtures/                      # Shared resources
│   ├── values-base.yaml            # Common Helm values
│   ├── cloudendpoint.yaml          # → CloudEndpoint (retained w/ Retain)
│   ├── agentendpoint.yaml          # → AgentEndpoint (never retained)
│   ├── ingress.yaml                # → AgentEndpoint (never retained)
│   ├── gateway.yaml + httproute.yaml # → AgentEndpoint (never retained)
│   ├── cloudendpoint-k8s-binding.yaml # → CloudEndpoint + BoundEndpoint
│   ├── ngrok-api-helper.sh         # API assertions
│   └── cleanup-cluster.sh          # Pre-test cleanup
│
├── delete-policy-bundled-crds/     # Single: Delete + bundled CRDs
├── retain-policy-bundled-crds/     # Single: Retain + bundled CRDs
├── delete-policy-separate-crds/    # Single: Delete + separate CRDs
├── retain-policy-separate-crds/    # Single: Retain + separate CRDs
├── bindings-delete-policy/         # BoundEndpoint + Delete
├── bindings-retain-policy/         # BoundEndpoint + Retain
├── multi-ns-delete-policy/         # Multi-op: namespace-scoped + Delete
├── multi-ns-retain-policy/         # Multi-op: namespace-scoped + Retain
├── multi-ingressclass-delete-policy/ # Multi-op: ingress-class + Delete
└── multi-ingressclass-retain-policy/ # Multi-op: ingress-class + Retain
```

## Helm Values Cascade

```bash
helm upgrade ... \
  --values _fixtures/values-base.yaml \  # Common: image, logging
  --values ./values.yaml \               # Scenario-specific: drainPolicy, installCRDs
  --set credentials.apiKey=...
```

## Prerequisites

- `NGROK_API_KEY` and `NGROK_AUTHTOKEN` environment variables
- Kind cluster running
- Docker available

## Debugging Tips

1. **Check endpoint type**: If a Retain test expects an endpoint to exist but it's absent, it's probably an AgentEndpoint (which always disappears)

2. **List all endpoints in ngrok API**:
   ```bash
   ./_fixtures/ngrok-api-helper.sh endpoint list
   ```

3. **Check what the operator created**:
   ```bash
   kubectl get cloudendpoint,agentendpoint -A
   ```

4. **Verify finalizers**:
   ```bash
   kubectl get ingress,gateway,httproute,cloudendpoint,agentendpoint -A -o jsonpath='{range .items[*]}{.kind}/{.metadata.name}: {.metadata.finalizers}{"\n"}{end}'
   ```
