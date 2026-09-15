# Upgrade from Helm chart 0.24 to 0.25

This guide covers upgrading an existing ngrok Kubernetes Operator installation
from Helm chart `0.24.x` to `0.25.x`.

> **Status:** in progress. This guide grows as 0.25 changes land; it is not yet
> a complete account of the release.

In the rest of this guide, "0.25" means the `ngrok/ngrok-operator` Helm chart
release.

## Rolling back

**Roll back to `0.24.x` only. Do not roll back from 0.25 to 0.23.**

Upgrade one minor version at a time, and roll back one minor version at a time.
0.24 is the oldest release a 0.25 cluster can return to.

The 0.25 operator writes `KubernetesOperator.status.enabledFeatures` as a JSON
array. 0.24 reads both that and the older comma-separated string, so a rollback
to 0.24 is safe and self-corrects on the next reconcile. 0.23 predates the
compatibility decoder entirely and cannot read the array at all:

```
unable to create KubernetesOperator: json: cannot unmarshal array into Go
struct field KubernetesOperatorStatus.status.enabledFeatures of type string
```

That failure is not limited to the api-manager pod. If any `AgentEndpoint`
exists, the agent-manager's watch on `KubernetesOperator` also fails to sync,
and affected AgentEndpoints stop reconciling with an empty `status: {}`. On a
cluster with no AgentEndpoints and no BoundEndpoints the problem can stay
invisible until a workload is created, so an empty-cluster smoke test is not
enough to confirm a 0.23 rollback succeeded.

### Recovering if you rolled back to 0.23 anyway

Order matters here. Complete the rollback **first**, then repair the stored
status. Patching while a 0.25 pod is still running lets it rewrite the array,
after which the 0.23 manager starts, takes the leader lease, and sits
`Ready=true` with a permanently broken `KubernetesOperator` watch — a healthy
looking pod that never reconciles.

1. Confirm no 0.25 operator pod is still running:

   ```bash
   kubectl get pods --namespace "$NAMESPACE" \
     -o custom-columns=NAME:.metadata.name,IMAGE:.spec.containers[0].image
   ```

2. Rewrite the status to the comma-separated form, using your own values from
   `spec.enabledFeatures`:

   ```bash
   kubectl patch kubernetesoperator "$RELEASE" \
     --namespace "$NAMESPACE" --subresource=status --type=merge \
     -p '{"status":{"enabledFeatures":"ingress,bindings"}}'
   ```

3. Restart the operator so it picks up the repaired object:

   ```bash
   kubectl rollout restart deployment/ngrok-operator-manager \
     --namespace "$NAMESPACE"
   ```

## `kubectl get kubernetesoperators` output format

The `Enabled Features` column now prints a JSON array rather than a bare
comma-separated list:

```
# 0.24
NAME             ID          READY   ENABLED FEATURES
ngrok-operator   k8sop_...   True    ingress,bindings

# 0.25
NAME             ID          READY   ENABLED FEATURES
ngrok-operator   k8sop_...   True    ["ingress","bindings"]
```

This affects anything reading the field programmatically — for example
`kubectl get kubernetesoperator -o jsonpath='{.status.enabledFeatures}'` piped
into a split on commas. Read it as a list instead:

```bash
kubectl get kubernetesoperator "$RELEASE" --namespace "$NAMESPACE" \
  -o jsonpath='{.status.enabledFeatures[*]}'
```

Existing objects keep the old format until the operator's first reconcile after
the upgrade, so during the rollout the format can differ between clusters. No
action is required; the value is operator-written and converts on its own.
Leader election means the first reconcile can lag the pod becoming `Ready` by
roughly 30 seconds.
