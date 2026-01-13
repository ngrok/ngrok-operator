# Uninstalling the ngrok Operator

This guide explains how to safely uninstall the ngrok-operator while ensuring:
- All ngrok API resources (endpoints, domains, etc.) are properly cleaned up
- Kubernetes resources don't get stuck in `Terminating` state
- No orphaned resources remain in your cluster

## Overview

The ngrok-operator uses Kubernetes finalizers to ensure that ngrok API resources are deleted before the corresponding Kubernetes resources are removed. This creates a dependency: the operator must be running to process finalizer removal.

## Quick Start

### Helm (Recommended)

If you installed via Helm with `cleanupHook.enabled: true` (the default), simply run:

```bash
helm uninstall ngrok-operator -n ngrok-operator
```

The pre-delete hook will:
1. Delete the `KubernetesOperator` CR
2. Wait for the operator to drain all managed resources
3. Complete the uninstall

### Manual / Non-Helm

For non-Helm installations, follow these steps:

```bash
# Step 1: Trigger drain by deleting the KubernetesOperator CR
kubectl delete kubernetesoperator <name> -n <namespace>

# Step 2: Wait for drain to complete
kubectl wait --for=delete kubernetesoperator/<name> -n <namespace> --timeout=300s

# Step 3: Delete the operator deployment and other resources
kubectl delete -f operator-manifests.yaml
```

## Drain Mode

The operator supports a "drain mode" that cleans up all resources it manages. Drain mode is triggered automatically when:

1. The `KubernetesOperator` CR is deleted
2. The `spec.drainMode` field is set to `true`

### What Happens During Drain

During drain, the operator:

1. **Lists all managed resources** - Uses controller labels (`k8s.ngrok.com/controller-namespace`, `k8s.ngrok.com/controller-name`) to find resources owned by this operator instance

2. **Handles user-owned resources** (Ingress, Service, Gateway, HTTPRoute, TCPRoute, TLSRoute):
   - Removes the `k8s.ngrok.com/finalizer`
   - Does NOT delete the Kubernetes resource (preserves user's intent)

3. **Handles operator-managed resources** (Domain, IPPolicy, CloudEndpoint, AgentEndpoint, BoundEndpoint):
   - Deletes the corresponding ngrok API resource
   - Removes the finalizer
   - Deletes the Kubernetes resource

### Monitoring Drain Progress

You can monitor drain progress via the `KubernetesOperator` status:

```bash
kubectl get kubernetesoperator <name> -n <namespace> -o yaml
```

Status fields:
- `status.drainStatus`: `pending`, `draining`, `completed`, or `failed`
- `status.drainMessage`: Human-readable status message
- `status.drainProgress`: Progress indicator (e.g., `5/10`)

### Manual Drain Trigger

To trigger drain without deleting the CR:

```bash
kubectl patch kubernetesoperator <name> -n <namespace> \
  --type=merge -p '{"spec":{"drainMode":true}}'
```

Watch the status:

```bash
kubectl get kubernetesoperator <name> -n <namespace> -w
```

## Multi-Instance Installations

If you have multiple ngrok-operator instances (e.g., in different namespaces), drain only affects resources managed by that specific instance. Resources are identified by the controller labels:

- `k8s.ngrok.com/controller-namespace`: The namespace where the operator is deployed
- `k8s.ngrok.com/controller-name`: The name of the operator deployment

This ensures that uninstalling one operator instance doesn't affect resources managed by another.

## Troubleshooting

### Resources Stuck in Terminating State

If resources are stuck in `Terminating` after uninstall:

1. **Check if operator is running**:
   ```bash
   kubectl get pods -n ngrok-operator
   ```
   If not running, the finalizer cannot be removed automatically.

2. **Manual finalizer removal** (use with caution - may orphan ngrok resources):
   ```bash
   kubectl patch ingress <name> -n <namespace> \
     --type=json -p='[{"op":"remove","path":"/metadata/finalizers"}]'
   ```

3. **Re-install the operator temporarily** to let it clean up properly:
   ```bash
   helm install ngrok-operator ngrok/ngrok-operator -n ngrok-operator
   # Wait for cleanup
   helm uninstall ngrok-operator -n ngrok-operator
   ```

### Drain Timeout

If drain takes too long, the Helm hook may time out. Increase the timeout:

```yaml
# values.yaml
cleanupHook:
  timeout: 600  # 10 minutes
```

### Orphaned ngrok Resources

If Kubernetes resources were deleted before the operator could clean up ngrok API resources, those resources may be orphaned in the ngrok dashboard. You can:

1. Manually delete them from the [ngrok Dashboard](https://dashboard.ngrok.com)
2. Use the ngrok API or CLI to list and delete them

## Cleanup Hook Configuration

The Helm chart includes a pre-delete hook that automates the drain process.

```yaml
# values.yaml
cleanupHook:
  enabled: true      # Enable the cleanup hook (default: true)
  timeout: 300       # Timeout in seconds (default: 300)
  image:
    repository: bitnami/kubectl
    tag: latest
  resources:
    limits:
      cpu: 100m
      memory: 128Mi
    requests:
      cpu: 50m
      memory: 64Mi
```

### Disabling the Cleanup Hook

If you prefer manual cleanup or have a custom uninstall process:

```yaml
cleanupHook:
  enabled: false
```

## Best Practices

1. **Always drain before uninstall** - Ensure the `KubernetesOperator` CR is deleted and drain completes before removing the operator deployment

2. **Monitor drain status** - Check the CR status to confirm all resources are cleaned up

3. **Don't delete the operator deployment first** - If the deployment is deleted before drain completes, resources may be stuck with finalizers

4. **Use Helm hooks** - The default cleanup hook automates the correct uninstall order

5. **Plan for timeout** - For large installations with many resources, increase the `cleanupHook.timeout`

## Related Documentation

- [Finalizer Cleanup Design](finalizer-plan-1.md) - Technical design document
- [Developer Guide](developer-guide/) - For contributors
