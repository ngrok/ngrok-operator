# Uninstalling the ngrok Operator

This guide covers safe uninstallation of the ngrok-operator, ensuring proper cleanup of ngrok API resources and preventing stuck Kubernetes resources.

## Quick Start

### Helm (Recommended)

```bash
helm uninstall ngrok-operator -n ngrok-operator
```

The pre-delete hook automatically triggers drain mode, waits for cleanup, then completes uninstall.

### Manual / Non-Helm

```bash
# 1. Trigger drain
kubectl delete kubernetesoperator <name> -n <namespace>

# 2. Wait for completion
kubectl wait --for=delete kubernetesoperator/<name> -n <namespace> --timeout=300s

# 3. Delete operator resources
kubectl delete -f operator-manifests.yaml
```

## Drain Policies

Configure via the `drainPolicy` Helm value:

| Policy | ngrok API Resources | Best For |
|--------|---------------------|----------|
| **Retain** (default) | Preserved in your account | Production - keep your configuration |
| **Delete** | Removed from your account | Dev/testing - clean slate |

Both policies remove finalizers from all managed Kubernetes resources.

## Monitoring Drain Progress

```bash
kubectl get kubernetesoperator <name> -n <namespace> -o yaml
```

Status fields:
- `drainStatus`: `pending` → `draining` → `completed`/`failed`
- `drainProgress`: `X/Y` (processed/total)
- `drainErrors`: Error messages if any

## Multi-Instance Installations

When multiple operator instances exist, drain only affects resources managed by that instance:

- **Ingress**: Filtered by `IngressClass`
- **Gateway/Routes**: Filtered by `GatewayClass`
- **Other resources**: Filtered by namespace (if `watchNamespace` is set)

## Troubleshooting

### Resources Stuck in Terminating

1. Check if operator is running: `kubectl get pods -n ngrok-operator`
2. Manual finalizer removal (may orphan ngrok resources):
   ```bash
   kubectl patch ingress <name> -n <namespace> \
     --type=json -p='[{"op":"remove","path":"/metadata/finalizers"}]'
   ```
3. Or re-install temporarily to let it clean up properly

### Drain Timeout

Increase the hook timeout:
```yaml
cleanupHook:
  timeout: 600  # 10 minutes
```

### Orphaned ngrok Resources

Delete manually from [ngrok Dashboard](https://dashboard.ngrok.com) or via the ngrok CLI.

## Helm Configuration

```yaml
drainPolicy: "Retain"  # or "Delete"

cleanupHook:
  enabled: true        # default
  timeout: 300         # seconds
```

## Architecture

See [internal/drain/](../internal/drain/) for implementation:

| Component | Role |
|-----------|------|
| `Orchestrator` | Coordinates the drain workflow |
| `StateChecker` | Detects drain mode (caches once triggered) |
| `Drainer` | Processes resources based on policy |

Controllers check `drain.IsDraining()` before adding finalizers or syncing new resources.
