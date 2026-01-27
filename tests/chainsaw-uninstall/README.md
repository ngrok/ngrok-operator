# Uninstall E2E Tests

This directory contains end-to-end tests for the operator's behavior during `helm uninstall`.

## Overview

When `helm uninstall` is executed, a pre-delete hook triggers the drain process which:
1. Sets the KubernetesOperator to draining state
2. Processes all managed resources based on the `drainPolicy`:
   - **Retain** (default): Removes finalizers only (ngrok API resources preserved)
   - **Delete**: Deletes CRs (controllers clean up ngrok API resources)
3. Removes the KubernetesOperator finalizer and completes uninstall

**Note**: We test the Delete policy first as it makes manual cleanup easier during development.

## Test Scenarios

### Single Operator Scenarios

| Scenario | Drain Policy | CRDs | Description |
|----------|--------------|------|-------------|
| `delete-policy-bundled-crds` | Delete | Bundled | Resources deleted from ngrok API, CRDs removed with operator |
| `retain-policy-bundled-crds` | Retain | Bundled | Resources preserved in ngrok API, CRDs removed with operator |
| `delete-policy-separate-crds` | Delete | Separate | Resources deleted from ngrok API, CRDs persist, CRs deleted |
| `retain-policy-separate-crds` | Retain | Separate | CloudEndpoint preserved in ngrok API, CRDs persist, CRs exist without finalizers |

### Separate CRD Scenarios

When CRDs are installed separately via `helm/ngrok-crds`:
1. CRDs persist after operator uninstall
2. With **Delete policy**: CRs are deleted (triggering ngrok API cleanup via finalizers)
3. With **Retain policy**: CRs exist but **without finalizers** (drain removes them)
4. Uninstalling the CRD chart removes CRDs and cascades to delete any remaining CRs

### Multi-Operator Scenarios

All multi-operator scenarios use **separate CRDs** to avoid complications when one operator is uninstalled.

#### Namespace-Scoped (watchNamespace)

Two operators in separate namespaces, each watching only its own namespace:

| Scenario | Drain Policy | Description |
|----------|--------------|-------------|
| `multi-ns-delete-policy` | Delete | Uninstall operator-a, verify operator-b continues working, resources cleaned from ngrok API |
| `multi-ns-retain-policy` | Retain | Uninstall operator-a, verify operator-b continues working, CloudEndpoint preserved in ngrok API |

#### IngressClass-Scoped

Two operators watching all namespaces, scoped by different ingress class names:

| Scenario | Drain Policy | Description |
|----------|--------------|-------------|
| `multi-ingressclass-delete-policy` | Delete | Uninstall operator-a (ngrok-a class), verify operator-b (ngrok-b class) unaffected |
| `multi-ingressclass-retain-policy` | Retain | Uninstall operator-a, verify operator-b unaffected |

**Key differences from namespace-scoped**:
- Only tests **Ingress resources** (which have `ingressClassName`)
- Does NOT test CloudEndpoints/AgentEndpoints - these CRDs have no ingress class concept
- If you need CRD isolation with multiple operators, use namespace-scoped (`watchNamespace`)

**Why no CloudEndpoints in ingress-class tests?**
When two operators watch all namespaces and are only differentiated by ingress class, both would try to reconcile the same CloudEndpoints (since CloudEndpoints have no ingress class field). This causes conflicts and undefined behavior.

**Known bug: Shared backend services**
When multiple Ingresses (with different ingress classes) point to the same backend service, the operator creates a single AgentEndpoint based on the backend service name, not the Ingress name. This causes only one Ingress to get an endpoint in the ngrok API. **Workaround**: Use separate backend services for each Ingress when using multiple ingress classes.

## Running Tests

### Run a specific scenario
```bash
make e2e-uninstall SCENARIO=delete-policy-bundled-crds
make e2e-uninstall SCENARIO=multi-ns-delete-policy
```

### Run with debug mode (pause on failure, keep resources)
```bash
make e2e-uninstall SCENARIO=delete-policy-bundled-crds DEBUG=1
```

### Run all scenarios
```bash
make e2e-uninstall-all
```

### Cleanup after failed test
```bash
make e2e-clean-uninstall
```

## Prerequisites

- `NGROK_API_KEY` and `NGROK_AUTHTOKEN` environment variables set
- Kind cluster running (`kind create cluster`)
- Docker available for image building

## Directory Structure

```
tests/chainsaw-uninstall/
├── README.md
├── _fixtures/                          # Shared test resources
│   ├── values-base.yaml                # Base Helm values (image, logging, etc.)
│   ├── backend-service.yaml            # Single-operator backend service
│   ├── backend-service-a.yaml          # Multi-operator backend (namespace-a)
│   ├── backend-service-b.yaml          # Multi-operator backend (namespace-b)
│   ├── cloudendpoint.yaml              # Single-operator CloudEndpoint
│   ├── cloudendpoint-a.yaml            # Multi-operator CloudEndpoint A
│   ├── cloudendpoint-b.yaml            # Multi-operator CloudEndpoint B
│   ├── agentendpoint.yaml
│   ├── ingress.yaml                    # Single-operator Ingress
│   ├── ingress-a.yaml                  # Multi-operator Ingress A (ngrok-a class)
│   ├── ingress-b.yaml                  # Multi-operator Ingress B (ngrok-b class)
│   └── ngrok-api-helper.sh             # API assertion helper script
│
├── delete-policy-bundled-crds/         # Single operator: Delete + bundled CRDs
├── retain-policy-bundled-crds/         # Single operator: Retain + bundled CRDs
├── delete-policy-separate-crds/        # Single operator: Delete + separate CRDs
├── retain-policy-separate-crds/        # Single operator: Retain + separate CRDs
│
├── multi-ns-delete-policy/             # Multi-operator: Namespace-scoped + Delete
│   ├── chainsaw-test.yaml
│   ├── values-operator-a.yaml          # Operator A config (watches namespace-a)
│   └── values-operator-b.yaml          # Operator B config (watches namespace-b)
├── multi-ns-retain-policy/             # Multi-operator: Namespace-scoped + Retain
│   ├── chainsaw-test.yaml
│   ├── values-operator-a.yaml
│   └── values-operator-b.yaml
│
├── multi-ingressclass-delete-policy/   # Multi-operator: IngressClass-scoped + Delete
│   ├── chainsaw-test.yaml
│   ├── values-operator-a.yaml          # Operator A config (ingress class ngrok-a)
│   └── values-operator-b.yaml          # Operator B config (ingress class ngrok-b)
└── multi-ingressclass-retain-policy/   # Multi-operator: IngressClass-scoped + Retain
    ├── chainsaw-test.yaml
    ├── values-operator-a.yaml
    └── values-operator-b.yaml
```

## Helm Values Cascade

Tests use cascading Helm values files:
```bash
# Single operator
helm upgrade ... \
  --values ./tests/chainsaw-uninstall/_fixtures/values-base.yaml \
  --values ./tests/chainsaw-uninstall/<scenario>/values.yaml \
  --set credentials.apiKey=... \
  --set credentials.authtoken=...

# Multi-operator
helm upgrade ngrok-operator-a ... \
  --values ./tests/chainsaw-uninstall/_fixtures/values-base.yaml \
  --values ./tests/chainsaw-uninstall/<scenario>/values-operator-a.yaml \
  ...
```

**values-base.yaml** contains common settings:
- Image repository, tag, pullPolicy
- Log format and level

**Scenario values files** only override what's different:
- `drainPolicy: Delete` or `Retain`
- `installCRDs: false` for separate CRD scenarios
- `watchNamespace` for namespace-scoped operators
- `ingress.ingressClass.name`, `ingress.controllerName` for multi-operator scenarios

## Adding New Scenarios

1. Create a new directory under `tests/chainsaw-uninstall/`
2. Create values file(s) with scenario-specific helm configuration
3. Create `chainsaw-test.yaml` using shared fixtures from `../_fixtures/`
4. The scenario will automatically be picked up by `make e2e-uninstall-all`

## Helper Scripts

- `_fixtures/ngrok-api-helper.sh` - Assert ngrok API state for endpoints and KubernetesOperators
  ```bash
  # Endpoint commands
  ./_fixtures/ngrok-api-helper.sh endpoint exists "my-endpoint.internal"
  ./_fixtures/ngrok-api-helper.sh endpoint absent "my-endpoint.internal"
  ./_fixtures/ngrok-api-helper.sh endpoint list
  ./_fixtures/ngrok-api-helper.sh endpoint delete-matching "uninstall-test"
  
  # KubernetesOperator commands
  ./_fixtures/ngrok-api-helper.sh k8sop exists "k8sop_abc123"
  ./_fixtures/ngrok-api-helper.sh k8sop absent "k8sop_abc123"
  ```

## Relationship to tests/chainsaw-multi-ns

The `tests/chainsaw-multi-ns` directory contains functional tests for multi-namespace deployments:
- Tests that operators only reconcile resources in their watched namespace
- Tests that unwatched namespaces are ignored
- **Does NOT test uninstall behavior**

This uninstall test suite (`tests/chainsaw-uninstall`) complements those tests by:
- Testing what happens when one of multiple operators is uninstalled
- Verifying drain policy behavior (Delete vs Retain)
- Verifying the other operator continues working after one is removed

**Overlap**: Both test suites deploy two operators with namespace/ingress-class scoping. The deployment patterns are similar but serve different purposes.

**Future consideration**: Could potentially share deployment setup code, but keeping them separate maintains test isolation and makes each suite self-contained.

## CI Integration

These tests run serially in CI (due to shared ngrok API) after the standard e2e tests.

## Design Decisions

### Why separate CRDs for multi-operator scenarios?

When CRDs are bundled with the operator:
- Uninstalling the first operator removes CRDs
- This breaks the second operator (can't reconcile resources without CRDs)
- Users should be instructed not to bundle CRDs in multi-operator deployments

We test only the supported configuration (separate CRDs) and document the limitation.

### Why test both namespace-scoped and ingress-class-scoped?

These are the two primary ways users run multiple operators:
1. **Namespace-scoped**: Each operator watches a specific namespace (`watchNamespace`)
2. **IngressClass-scoped**: Operators watch all namespaces but only reconcile Ingresses with their class

Both need testing because:
- They have different resource ownership patterns
- CloudEndpoints in the ingress-class scenario need special handling (placed in operator namespace)
- Drain behavior may interact differently with scope boundaries
