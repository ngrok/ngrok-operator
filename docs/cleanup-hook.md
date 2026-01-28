# Cleanup Hook

## Overview

The ngrok-operator includes an optional Helm pre-delete hook that ensures proper cleanup of ngrok resources when
uninstalling the operator. Without this hook, the operator's deployment and pods may be deleted before they can
clean up ngrok API resources, leaving orphaned resources in your ngrok account.

## How It Works

When `helm uninstall` is executed, the cleanup hook:

1. **Runs before deletion**: As a `pre-delete` hook, it executes before any operator resources are removed
2. **Annotates resources**: Adds `k8s.ngrok.com/cleanup=true` to all Kubernetes resources managed by the operator that have the ngrok finalizer
3. **Processes in order**:
   - Gateway API Routes (HTTPRoute, TCPRoute, TLSRoute)
   - Core resources (Ingress, Service)
   - Gateway API Gateways
   - ngrok CRDs (CloudEndpoint, AgentEndpoint, Domain, IPPolicy, etc.)
4. **Waits for cleanup**: Monitors each resource until the operator removes the finalizer, indicating cleanup is complete
5. **Retries on failure**: Automatically retries operations if they fail

This ensures the operator managers stay running long enough to properly clean up all ngrok resources before the operator itself is removed.

## Configuration

The cleanup hook is configured via Helm values:

```yaml
cleanupHook:
  # Enable or disable the cleanup hook
  enabled: true

  # Maximum time (in seconds) to wait for all resources to be cleaned up
  timeout: 300  # 5 minutes

  # Number of times to retry on failure
  retries: 3

  # Time to wait (in seconds) between retries
  retryInterval: 10  # 10 seconds

  # Resource requests/limits for the cleanup job pod
  resources:
    limits:
      cpu: 100m
      memory: 128Mi
    requests:
      cpu: 50m
      memory: 64Mi
```

## When to Disable

You may want to disable the cleanup hook if:

- You want to retain ngrok resources after uninstalling the operator
- You're troubleshooting and need to prevent automatic cleanup
- You have a custom cleanup process

To disable:

```bash
helm install ngrok-operator ngrok/ngrok-operator \
  --set cleanupHook.enabled=false
```

## Troubleshooting

### Timeout Issues

If the cleanup hook times out, you can increase the timeout:

```yaml
cleanupHook:
  timeout: 600s  # 10 minutes
```

### Hook Failures

Check the cleanup job logs:

```bash
kubectl logs -n ngrok-operator job/ngrok-operator-cleanup
```

### Manual Cleanup

If the hook fails, you can manually trigger cleanup by annotating resources:

```bash
# Annotate a specific ingress
kubectl annotate ingress my-ingress k8s.ngrok.com/cleanup=true

# Annotate all services with the ngrok finalizer
kubectl get svc -A -o json | \
  jq -r '.items[] | select(.metadata.finalizers[] | contains("k8s.ngrok.com/finalizer")) | "\(.metadata.namespace)/\(.metadata.name)"' | \
  xargs -I {} kubectl annotate svc {} k8s.ngrok.com/cleanup=true
```

## Resource Processing Order

The hook processes resources in a specific order to handle dependencies:

1. **Routes first**: Gateway API routes are cleaned up before their parent gateways
2. **Core resources**: Ingress and Service resources that create ngrok CRDs
3. **Gateways**: After routes are cleaned up
4. **ngrok CRDs**: Finally, any remaining operator-managed custom resources

This ordering ensures that dependent resources are cleaned up before their dependencies, preventing validation errors.

## Permissions

The cleanup hook requires cluster-wide permissions to:
- List and update all resource types managed by the operator
- Check if optional CRDs (like Gateway API) exist

These permissions are automatically granted via the `ngrok-operator-cleanup` ClusterRole, which is created as part of the hook.

## Implementation Details

The cleanup hook is implemented as:
- A Kubernetes Job with `helm.sh/hook: pre-delete`
- Uses a bash script (stored in ConfigMap) with kubectl to annotate and monitor resources
- Runs in a minimal `bitnami/kubectl` container image
- ServiceAccount with ClusterRole permissions
- Automatic cleanup via `helm.sh/hook-delete-policy: before-hook-creation,hook-succeeded`

The bash script is stored at `helm/ngrok-operator/scripts/cleanup.sh` and can be reviewed or customized before installation.

The hook will be automatically removed after successful completion or before the next hook execution.
