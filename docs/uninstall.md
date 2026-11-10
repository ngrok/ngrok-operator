# Uninstalling the ngrok Operator

This guide covers safe uninstallation of the ngrok-operator, ensuring proper cleanup of ngrok API resources and preventing stuck Kubernetes resources.

## Quick Start

### Helm (Recommended)

```bash
helm uninstall ngrok-operator -n ngrok-operator
```

The pre-delete hook automatically triggers drain mode, waits for cleanup, then completes uninstall.

For version 0.25, `helm uninstall` leaves some of the new `ngrok.com/v1` CRDs
in place. They carry `helm.sh/resource-policy: keep`, so that a `helm rollback`
to a release without them can't delete every object of those kinds. Once the
uninstall has drained, delete them yourself if you don't plan to reinstall:

```bash
kubectl delete crd <CRD>
```

### Manual / Non-Helm

```bash
# 1. Remove the pre-0.25 KubernetesOperator, if one exists. While it exists,
#    deleting the ngrok.com one skips the drain. The operator releases its
#    finalizer; if the delete hangs because the operator is not running, clear
#    the finalizer by hand.
kubectl delete kubernetesoperators.ngrok.k8s.ngrok.com <name> -n <namespace> --ignore-not-found --timeout=60s ||
  kubectl patch kubernetesoperators.ngrok.k8s.ngrok.com <name> -n <namespace> \
    --type=merge -p '{"metadata":{"finalizers":null}}'

# 2. Trigger drain
kubectl delete kubernetesoperators.ngrok.com <name> -n <namespace>

# 3. Wait for completion
kubectl wait --for=delete kubernetesoperators.ngrok.com/<name> -n <namespace> --timeout=300s

# 4. Delete operator resources
kubectl delete -f operator-manifests.yaml
```

Name the group in every `kubectl` command: the pre-0.25
`ngrok.k8s.ngrok.com` KubernetesOperator shares the `kubernetesoperators`
plural.

## Drain Policies

Configure via the `ngrok.features.cleanup.drainPolicy` Helm value:

| Policy | ngrok API Resources | Best For |
|--------|---------------------|----------|
| **Retain** (default) | Preserved in your account | Production - keep your configuration |
| **Delete** | Removed from your account | Dev/testing - clean slate |

Both policies remove finalizers from all managed Kubernetes resources.

## Monitoring Drain Progress

```bash
kubectl get kubernetesoperators.ngrok.com <name> -n <namespace> -o yaml
```

Status fields:
- `Draining` condition: `True` while the drain is running or retrying (reason `DrainInProgress`/`DrainFailed`), `False` with reason `DrainCompleted` when finished
- `drain.drainedResources` / `drain.failedResources` / `drain.totalResources`: latest-attempt counters
- `drain.errors`: Error messages if any

## Multi-Instance Installations

When multiple operator instances exist, drain only affects resources managed by that instance:

- **Ingress**: Filtered by `IngressClass`
- **Gateway/Routes**: Filtered by `GatewayClass`
- **Other resources**: Filtered by namespace (if `ngrok.features.ingress.watchNamespace` is set)

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
ngrok:
  features:
    cleanup:
      timeout: 600  # 10 minutes
```

### Orphaned ngrok Resources

Delete manually from [ngrok Dashboard](https://dashboard.ngrok.com) or via the ngrok CLI.

## Helm Configuration

```yaml
ngrok:
  features:
    cleanup:
      enabled: true          # default
      timeout: 300           # seconds
      drainPolicy: "Retain"  # or "Delete"
```

## Architecture

See [internal/drain/](../internal/drain/) for implementation:

| Component | Role |
|-----------|------|
| `Orchestrator` | Coordinates the drain workflow |
| `StateChecker` | Detects drain mode (caches once triggered) |
| `Drainer` | Processes resources based on policy |

Controllers check `drain.IsDraining()` before adding finalizers or syncing new resources.
