# Uninstall E2E Tests

End-to-end tests for `helm uninstall` drain behavior.

## Key Concept: Endpoint Types

| Type | Persists in ngrok API? | Notes |
|------|------------------------|-------|
| **CloudEndpoint** | ✅ Yes | Retained with `Retain` policy |
| **AgentEndpoint** | ❌ No | Session-based; dies when agent stops |

This distinction drives all test assertions.

### Mapping Strategy: endpoints-verbose

All Ingress and Gateway fixtures use the `k8s.ngrok.com/mapping-strategy: "endpoints-verbose"` annotation.
This ensures **both** a CloudEndpoint (public URL) and an internal AgentEndpoint are created:

- **CloudEndpoint**: e.g., `https://uninstall-test-ingress.internal` - respects Retain/Delete policy
- **Internal AgentEndpoint**: e.g., `https://abc12-svc-ns-80.internal` - always ephemeral

This provides consistent, predictable test behavior regardless of route complexity.

## Running Tests

```bash
# Run specific scenario
make e2e-uninstall SCENARIO=delete-policy-bundled-crds

# Run with debug mode
make e2e-uninstall SCENARIO=delete-policy-bundled-crds DEBUG=1

# Run all scenarios
make e2e-uninstall-all

# Cleanup after failed test
make e2e-clean-uninstall
```

## Test Scenarios

### Single Operator

| Scenario | Policy | CRDs | ngrok API After Uninstall |
|----------|--------|------|---------------------------|
| `delete-policy-bundled-crds` | Delete | Bundled | Empty |
| `retain-policy-bundled-crds` | Retain | Bundled | CloudEndpoints only |
| `delete-policy-separate-crds` | Delete | Separate | Empty |
| `retain-policy-separate-crds` | Retain | Separate | CloudEndpoints only |

### BoundEndpoint (kubernetes bindings)

| Scenario | Policy | ngrok API After Uninstall |
|----------|--------|---------------------------|
| `bindings-delete-policy` | Delete | Empty |
| `bindings-retain-policy` | Retain | CloudEndpoint with binding |

### Multi-Operator

All use separate CRDs and namespace scoping (`watchNamespace`).

| Scenario | Policy | Description |
|----------|--------|-------------|
| `multi-ns-delete-policy` | Delete | Operator-a uninstalled; operator-b unaffected |
| `multi-ns-retain-policy` | Retain | Same |
| `multi-ingressclass-delete-policy` | Delete | Same + different ingress classes |
| `multi-ingressclass-retain-policy` | Retain | Same |

## Directory Structure

```
tests/chainsaw-uninstall/
├── _fixtures/                        # Shared test resources
│   ├── cloudendpoint.yaml            # → CloudEndpoint (retained)
│   ├── agentendpoint.yaml            # → AgentEndpoint (never retained)
│   ├── ingress.yaml                  # → CloudEndpoint + internal AgentEndpoint (endpoints-verbose)
│   ├── gateway.yaml + httproute.yaml # → CloudEndpoint + internal AgentEndpoint (endpoints-verbose)
│   ├── ngrok-api-helper.sh           # API assertions
│   └── cleanup-cluster.sh            # Pre-test cleanup
│
├── delete-policy-bundled-crds/
├── retain-policy-bundled-crds/
├── ...
```

## Prerequisites

- `NGROK_API_KEY` and `NGROK_AUTHTOKEN` environment variables
- Kind cluster running
- Docker available

## Debugging

```bash
# List all endpoints in ngrok API
./_fixtures/ngrok-api-helper.sh endpoint list

# Check what the operator created
kubectl get cloudendpoint,agentendpoint -A

# Verify finalizers
kubectl get ingress,cloudendpoint,agentendpoint -A \
  -o jsonpath='{range .items[*]}{.kind}/{.metadata.name}: {.metadata.finalizers}{"\n"}{end}'
```
