# Multi-Namespace E2E Tests

This directory contains Chainsaw tests for validating multi-namespace operator deployments.

## Prerequisites

These tests assume the cluster has been set up using `make deploy_multi_namespace`, which deploys:

1. **CRDs** installed once in `kube-system` via `ngrok-operator-crds` helm chart
2. **Operator A** (`ngrok-operator-a`) in `namespace-a`:
   - Watches only `namespace-a`
   - Uses ingress class `ngrok-a`
   - Controller name: `k8s.ngrok.com/ingress-controller-a`
3. **Operator B** (`ngrok-operator-b`) in `namespace-b`:
   - Watches only `namespace-b`
   - Uses ingress class `ngrok-b`
   - Controller name: `k8s.ngrok.com/ingress-controller-b`

## Running Locally

```bash
# Set up the multi-namespace deployment
make deploy_multi_namespace

# Run the tests
make e2e-tests-multi-ns
```

## Test Structure

- **sanity-checks/**: Validates both operators are running and ingress classes exist
- **operator-registration/**: Tests that the operators correctly register with ngrok and receive an ID.

## Writing New Tests

When adding tests to this directory:

- Use explicit `metadata.namespace` values (`namespace-a` or `namespace-b`)
- Use the correct `ingressClassName` (`ngrok-a` or `ngrok-b`)
- Focus on multi-operator isolation behaviors that can't be tested with a single operator
