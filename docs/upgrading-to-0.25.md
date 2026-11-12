# Upgrading from Helm chart 0.24 to 0.25

This guide covers upgrading an existing ngrok Kubernetes Operator installation
from Helm chart `0.24.x` to `0.25.x`.

The project publishes the Helm chart, operator image, and CRDs with independent
version numbers:

| Component | Before | After |
| --- | --- | --- |
| `ngrok/ngrok-operator` Helm chart | `0.24.x` | `0.25.x` |
| Operator image | `0.22.x` | `0.23.x` |
| `ngrok/ngrok-crds` subchart | `0.4.x` | `0.5.x` |

In the rest of this guide, "0.25" means the `ngrok/ngrok-operator` Helm chart
release.

**Upgrade from 0.24 only.** Do not upgrade directly from 0.23 or earlier. 0.25
removes compatibility code that 0.24 used to migrate existing objects, so a
0.23 installation must first upgrade to 0.24, run healthily, and complete the
["After the rollback window"](upgrading-to-0.24.md#after-the-rollback-window)
steps in the 0.24 upgrade guide that this guide lists as required below.

This migration has 4 major breaking changes:

- API Key and Authtoken have been removed and replaced with a single [access token](#create-an-access-token)
- The [Helm Values schema](#migrate-helm-values-to-the-new-schema) has been refactored, requiring any values you set to likely need to be changed.
- Spec fields and annotations deprecated in 0.24 [have been removed](#remove-deprecated-fields-dropped-by-025)
- The CRDs have moved from `v1alpha1` to `v1` and also moved groups to all be in the `ngrok.com` group
    - The operator migrates the objects it created for you. You [migrate the objects you created](#migrate-your-alpha-objects-to-their-v1-versions) after the upgrade.

## TL;DR checklist

1. [Prerequisite requirements met](#prerequisite-requirements)
2. [Create an access token](#create-an-access-token)
3. [Migrate Helm values to the new schema](#migrate-helm-values-to-the-new-schema)
4. [Remove deprecated fields dropped by 0.25](#remove-deprecated-fields-dropped-by-025)
5. [Upgrade `ngrok-crds` Helm Chart if installed separately](#upgrade-ngrok-crds-helm-chart-if-installed-separately)
6. [Helm upgrade, verify health](#helm-upgrade-verify-health)
7. [Migrate your alpha objects to their v1 versions](#migrate-your-alpha-objects-to-their-v1-versions)

## Prerequisite Requirements

### Confirm you are on 0.24

If you are not, follow the [0.24 upgrade guide](upgrading-to-0.24.md) first.

### Back up your 0.24 helm release

```bash
RELEASE=ngrok-operator
NAMESPACE=ngrok-operator

helm get values "$RELEASE" --namespace "$NAMESPACE" \
> ngrok-operator-0.24-values.yaml
helm get manifest "$RELEASE" --namespace "$NAMESPACE" \
> ngrok-operator-0.24-manifest.yaml
```

Both files contain your current credentials, so store them somewhere only you can read.

## Create an access token

In 0.25 we move to using a single access token instead of apiKey+authtoken. Provision an access token at https://dashboard.ngrok.com/access-tokens. Under **Scopes**, choose either:

- **Full access**: the simplest option. The token gets everything its owner's role can access today.
- **Custom**: open **Quick select** and choose **Kubernetes Operator** to select only the scopes the operator needs. See [Access token scopes](#access-token-scopes) for the list.

**NOTE:** Do not revoke your API Key and Authtoken yet. Rolling back to 0.24 still uses them.

Follow the instructions below for your secret integrations strategy:

### Using Helm Values to Reference Secret by Name (`credentials.secret.name`)

Create a new secret (with a new name) with the access token you created above. Keep the old secret so you can still roll back.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-ngrok-credentials-v2
  namespace: ngrok-operator
type: Opaque
data:
  AGENT_ACCESS_TOKEN: <base64-encoded-access-token>
  API_MANAGER_ACCESS_TOKEN: <base64-encoded-access-token>
  # Only if you enable ngrok.features.privateEndpoints
  PRIVATE_ENDPOINTS_ACCESS_TOKEN: <base64-encoded-access-token>
```

Then point your 0.25 values at it:

```yaml
ngrok:
  credentials:
    secret:
      name: my-ngrok-credentials-v2
```

### Set Secret Value Directly in Helm Chart (`credentials.apiKey + credentials.authtoken`)

Replace `credentials.apiKey` and `credentials.authtoken` with the access token in your 0.25 values:

```yaml
ngrok:
  credentials:
    accessToken: "<your-access-token>"
```

Or, if you pass it on the command line, use `--set ngrok.credentials.accessToken="$NGROK_ACCESS_TOKEN"` in the [Helm upgrade](#helm-upgrade-to-025) command.

## Migrate Helm values to the new schema

The 0.25 chart reorganizes its values into three groups: the chart's own settings stay at the top level, the operator's configuration moves under `ngrok`, and pod settings move under `components`. There are no aliases for the old keys: a values file that still sets one fails the render and lists each old key with its new location. 

### Migration script

The [migration script](https://github.com/ngrok/ngrok-operator/blob/main/scripts/migrate-helm-values-to-0.25.sh) rewrites your 0.24 values into the 0.25 layout. It requires only `helm`:

```bash
./migrate-helm-values-to-0.25.sh ngrok-operator-0.24-values.yaml > ngrok-operator-0.25-values.yaml
```

It prints a note for anything you need to fix by hand, such as credentials. Review the result before upgrading.

### Values mapping

The mapping can be found in the table below:

| 0.24 value | 0.25 value |
| --- | --- |
| `description`, `region`, `rootCAs`, `serverAddr`, `apiURL`, `clusterDomain` | `ngrok.<same>` |
| `ngrokMetadata`, `metaData` | `ngrok.metadata` |
| `credentials.secret.name` | `ngrok.credentials.secret.name` |
| `credentials.apiKey`, `credentials.authtoken` | removed: set `ngrok.credentials.accessToken` instead (see [Create an access token](#create-an-access-token)) |
| `log.*` | `ngrok.log.*` |
| `ingress.*` | `ngrok.features.ingress.*` |
| `watchNamespace`, `controllerName`, `ingressClass.*` | `ngrok.features.ingress.watchNamespace`, `ngrok.features.ingress.controllerName`, `ngrok.features.ingress.ingressClass.*` |
| `gateway.*` | `ngrok.features.gateway.*` |
| `bindings.enabled`, `bindings.endpointSelectors`, `bindings.serviceAnnotations`, `bindings.serviceLabels`, `bindings.ingressEndpoint` | `ngrok.features.bindings.<same>` |
| `drainPolicy` | `ngrok.features.cleanup.drainPolicy` |
| `defaultDomainReclaimPolicy` | `ngrok.features.domains.defaultReclaimPolicy` |
| `oneClickDemoMode` | `ngrok.features.oneClickDemoMode.enabled` |
| `cleanupHook.enabled`, `cleanupHook.timeout` | `ngrok.features.cleanup.enabled`, `ngrok.features.cleanup.timeout` |
| `cleanupHook.image`, `cleanupHook.resources` | `components.cleanupHook.<same>` |
| `podAnnotations`, `podLabels`, `affinity`, `podAffinityPreset`, `podAntiAffinityPreset`, `nodeAffinityPreset`, `extraEnv`, `extraVolumes`, `extraVolumeMounts`, `lifecycle` | `components.common.<same>` (as in 0.24, these apply to every component) |
| `nodeSelector`, `tolerations`, `topologySpreadConstraints`, `priorityClassName`, `replicaCount`, `resources`, `terminationGracePeriodSeconds`, `podDisruptionBudget.*`, `serviceAccount.*` | `components.apiManager.<same>` |
| `agent.*` | `components.agent.*` |
| `bindings.forwarder.*` | `components.bindingsForwarder.*` |
| `nameOverride`, `fullnameOverride`, `commonLabels`, `commonAnnotations`, `image.*`, `installCRDs`, `crdAccessRoles.*` | unchanged |

In 0.24 the `components.apiManager` settings above only applied to the api-manager. To apply one to every pod, set it under `components.common` instead.

Three behaviors change along with the keys:

- In 0.24, a non-empty `agent.podAnnotations` replaced the top-level
`podAnnotations` on the agent pod. In 0.25, `components.agent.podAnnotations`
merges over `components.common.podAnnotations`, and setting it to `{}` clears
the common value. If you set both in 0.24 and don't want the top-level
annotations on the agent, put them on `components.apiManager` and
`components.bindingsForwarder` instead of `components.common`. The
[migration script](#migration-script) does this for you.
- Log settings can be overridden for one component under
`components.<component>.log`, for example `components.agent.log.level: debug`.
- An empty operator setting means "use the operator's built-in default". The
defaults are listed in the chart README.

## Remove deprecated fields dropped by 0.25

Fields that were deprecated in 0.24 are now removed in 0.25. If you completed the 0.24 guide, these are already done. If any of these still exist, follow the linked 0.24 guide section to update them:

- [`Domain` `spec.resolves_to` -> `spec.resolvesTo`](upgrading-to-0.24.md#rename-domainspecresolves_to)
- [`CloudEndpoint` `spec.trafficPolicyName` / `spec.trafficPolicy.policy` -> `spec.trafficPolicy.targetRef` / `spec.trafficPolicy.inline`](upgrading-to-0.24.md#migrate-cloudendpoint-traffic-policies)
- [Selectors on `bindings.k8s.ngrok.com/*` labels and the `endpoint-url` annotation -> `ngrok.com/*`](upgrading-to-0.24.md#update-selectors-for-operator-written-bindings-labels)
- Tooling that reads the operator-written `k8s.ngrok.com/controller-name` and `k8s.ngrok.com/controller-namespace` labels, `k8s.ngrok.com/computed-url` annotation, or `k8s.ngrok.com/finalizer` finalizer -> the same keys under `ngrok.com/`. The operator swaps these on its own.

0.25 still reads the `k8s.ngrok.com/*` annotations you set, but [rename them to `ngrok.com/*`](upgrading-to-0.24.md#rename-user-set-annotations) before upgrading to 1.0.

## Upgrade `ngrok-crds` Helm Chart if installed separately

By default, the Helm chart comes packaged with the CRDs, if you installed them separately, you need to upgrade the CRDs independently. Helm Chart 0.5.0 introduces `v1` versions of CRDs in new group locations that are canonical and deprecating the `v1alpha1` versions of those CRDs.

### Upgrade Instructions

Use the release name and namespace you installed the CRDs chart under, not the operator's. Find them with `helm list -A | grep ngrok-crds`.

```bash
CRD_RELEASE=ngrok-crds
CRD_NAMESPACE=ngrok-operator
TARGET_VERSION=0.5.0

helm repo update ngrok

helm upgrade "$CRD_RELEASE" ngrok/ngrok-crds \
  --namespace "$CRD_NAMESPACE" \
  --version "$TARGET_VERSION"
```

### Verification Instructions

Verify the new CRDs are available

```bash
kubectl get crds | grep ngrok.com | grep -v k8s.ngrok.com
```

```
agentendpoints.ngrok.com                    2026-11-11T19:45:16Z
cloudendpoints.ngrok.com                    2026-11-11T19:45:16Z
domains.ngrok.com                           2026-11-11T19:45:16Z
ippolicies.ngrok.com                        2026-11-11T19:45:16Z
kubernetesoperators.ngrok.com               2026-11-11T19:45:16Z
privateendpoints.ngrok.com                  2026-11-11T19:45:16Z
trafficpolicies.ngrok.com                   2026-11-11T19:45:16Z
```

Note: Both the old and new versions of the CRDs exist at the same time in the cluster (see the `grep -v k8s.ngrok.com` filtering out the old ones). You should query by the fully qualified CRD name such as `domains.ngrok.com`, not just `domains` as it could resolve to the `v1alpha1` versions and not be obvious.

## Helm upgrade, verify health

### Helm Upgrade to 0.25

```bash
TARGET_VERSION=0.25.0

helm repo update ngrok

helm upgrade "$RELEASE" ngrok/ngrok-operator \
  --namespace "$NAMESPACE" \
  --version "$TARGET_VERSION" \
  --values ngrok-operator-0.25-values.yaml \
  --wait
```

If any of your values still use their 0.24 names, you will see an error like this:

```
Error: UPGRADE FAILED: execution error at (ngrok-operator/templates/api-manager/deployment.yaml:1:4):

These Helm values moved in 0.25 and are no longer read at their old location:

  credentials -> ngrok.credentials (apiKey and authtoken are replaced by accessToken)

See the 0.25 upgrade guide for the full mapping.
```

### Verify Health

```bash
kubectl get deployments,pods --namespace "$NAMESPACE"
kubectl get kubernetesoperators.ngrok.com -A
```

### What changes on its own

The CRDs fall into 3 categories:

1. Operator managed: `KubernetesOperator`
2. User managed: `TrafficPolicy`, `IPPolicy`
3. Operator and User managed: `Domains`, `CloudEndpoints`, `AgentEndpoints`

CRDs managed by the Operator will automatically create new `v1` versions of the existing `v1alpha1` Custom Resource Objects, without creating new resources in ngrok. CRDs managed by the User are left alone.

### Rolling back

Rolling back at this point is safe and easy via a `helm rollback` command. The `v1` resources created by the `0.25.0` release survive a rollback due to the `helm.sh/resource-policy: keep`  annotation. The previous `0.24.0` release will ignore them and allow you to re-upgrade again later safely. **NOTE:** When rolled back to `0.24.0`, the `v1alpha1` resources are now the primary resources, reverting changes made to the `v1` objects while in `0.25.0` . Any changes done after updating to `0.25.0` would need to be mirrored on the `v1alpha1` objects to persist.

## Migrate your alpha objects to their v1 versions

Once the Operator is running, you can create `v1` versions of them. Both the `v1alpha1` version and the `v1` versions can live side by side without conflicting or resulting in duplicate resources being created. Instead, the operator will see that there is a pair of resources of different `Kinds` but with the same `metadata.name` and namespace and prefer the `v1` as the source of truth, adopting the `v1alpha1` resources `status` fields like `ID` and adds the status condition of `Ready` and reason `SupersededByV1` to the `v1alpha1` indicating that the `v1` version is the primary object.

Trying to modify the `v1alpha1` in this state will result in a `DeprecatedAPIGroup` warning event, and changes will be ignored, indicating that the changes should be made on the `v1` version. **NOTE:** Changes to `v1` are not persisted to `v1alpha1` objects.

**NOTE**: If you use gitops tools like ArgoCD, do not just edit the versions and kinds in place as it may prune the old `v1alpha1` before making the new `v1` versions, which may result in intermittent downtime

### Spec.Metadata

As a part of the migration to V1, the previous `spec.metadata` fields of CRDs, where they existed, moved from a `JSON`  String to a Map. If you had set this previously on your `v1alpha1` CRs, you should change their format in the new `v1` Version.

Example:

```yaml
apiVersion: <group>/v1alpha1
kind: <kind>
metadata:
  name: my-cr
spec:
  ...
  metadata: '{"team":"edge"}'
```

Becomes:

```yaml
apiVersion: ngrok.com/v1
kind: <kind>
metadata:
  name: my-cr
spec:
  ...
  metadata:
    team: edge
```

### TrafficPolicy

`ngrok.k8s.ngrok.com/v1alpha1` `NgrokTrafficPolicy` is deprecated. Its replacement is `ngrok.com/v1` `TrafficPolicy`. The spec is unchanged. References to `TrafficPolicy` objects resolve by name, so new replacement `v1` `TrafficPolicy` objects must match the `name` and `namespace` of their `v1alpha1` counterparts.

All `TrafficPolicy` objects are user created, so the user is responsible for all migrations.

### Domain

`ingress.k8s.ngrok.com/v1alpha1` `Domain` is deprecated. Its replacement is `ngrok.com/v1` `Domain`. The spec is unchanged except `spec.metadata`, which must be a map as described above.

K8sOperator will automatically update CR objects that are managed by the operator, but the user is responsible for upgrading the CRs they manage. If a user changed the metadata on an operator created `Domain` that is unable to be converted into a map, a `DomainMigrationBlocked` event will be emitted on the endpoint that uses the domain.

### IPPolicy

`ingress.k8s.ngrok.com/v1alpha1` `IPPolicy` is deprecated. Its replacement is `ngrok.com/v1` `IPPolicy`. The spec is unchanged except `spec.metadata` and `spec.rules[].metadata`, which must be maps as described above.

All `IPPolicy` objects are user created, so the user is responsible for all migrations.

### Cloud Endpoints

`ngrok.k8s.ngrok.com/v1alpha1` `CloudEndpoint` is deprecated. Its replacement is `ngrok.com/v1` `CloudEndpoint`. The spec is unchanged except `spec.metadata`, which must be a map:

```yaml
apiVersion: ngrok.k8s.ngrok.com/v1alpha1
kind: CloudEndpoint
metadata:
  name: my-endpoint
spec:
  url: https://example.ngrok.app
  trafficPolicy:
    inline:
      on_http_request:
        - actions:
            - type: custom-response
              config:
                status_code: 200
  metadata: '{"team":"edge"}'
```

becomes:

```yaml
apiVersion: ngrok.com/v1
kind: CloudEndpoint
metadata:
  name: my-endpoint
spec:
  url: https://example.ngrok.app
  trafficPolicy:
    inline:
      on_http_request:
        - actions:
            - type: custom-response
              config:
                status_code: 200
  metadata:
    team: edge
```

While both a `v1alpha1` version and `v1` version exist, as long as they have the same name and namespace, the operator dedupes them and uses the `v1` as the source of truth so you only have 1 `CloudEndpoint` online to avoid billing for multiple endpoints.

### Agent Endpoints

`ngrok.k8s.ngrok.com/v1alpha1` `AgentEndpoint` is deprecated. Its replacement is `ngrok.com/v1` `AgentEndpoint`. The spec is unchanged except `spec.metadata`, which must be a map:

```yaml
apiVersion: ngrok.k8s.ngrok.com/v1alpha1
kind: AgentEndpoint
metadata:
  name: my-endpoint 
spec:
  url: https://example.ngrok.app
  upstream:
    url: http://my-service.my-namespace:80
  metadata: '{"team":"edge"}'
```

becomes:

```yaml
apiVersion: ngrok.com/v1
kind: AgentEndpoint
metadata:
  name: my-endpoint
spec:
  url: https://example.ngrok.app
  upstream:
    url: http://my-service.my-namespace:80
  metadata:
    team: edge
```

While both a `v1alpha1` version and `v1` version exist, as long as they have the same name and namespace, the operator dedupes them and uses the `v1` as the source of truth so you only have 1 `AgentEndpoint` online to avoid billing for multiple endpoints.

## Follow ups

**NOTE:** Only do the following if you do not want to roll back anymore at all!

After upgrading your resources to `v1` you have satisfied the requirements for this release. All `v1alpha1` resources are deprecated in this release and are safe to remove. However, to support dual read, the `v1alpha1` CRDs **are required** in the cluster while `0.25.0` is running and should not be removed in this release. 

At this point, you can also delete the `API Key` and `Authtoken` from your account in the Dashboard. If you created a new credentials `Secret` for 0.25, you can delete your old 0.24 `Secret`. Do not delete the `Secret` your 0.25 release uses.

## What's next: 1.0 and 1.1

In `1.0`, the `v1alpha1` CRDs will be ignored by the controllers, and in `1.1`, they will be removed from Helm. This makes `0.25.0` a required stop for `1.0` .

## Appendix

### Access token scopes

The **Kubernetes Operator** quick select in the dashboard picks the api-manager and agent-manager scopes below. If you enable private endpoints, also add `gateway:endpoints:connect`.

The operator needs these scopes:

| Component | Scopes |
| --- | --- |
| api-manager | `domains:read`, `domains:write`, `endpoints:read`, `endpoints:write`, `tcp-addrs:read`, `tcp-addrs:write`, `ip-policies:read`, `ip-policies:write`, `kubernetes-operators:read`, `kubernetes-operators:write` |
| agent-manager | `tunnel:start` |
| private-endpoint-forwarder (only with `ngrok.features.privateEndpoints.enabled`) | `gateway:endpoints:connect` |

### One token per component

By default, 1 access token is used by every component. If you want extra security and minimal access, you can create a separate token for each component with only the scopes in the [table above](#access-token-scopes):

```yaml
ngrok:
  credentials:
    agent:
      accessToken: "<agent-manager token>"
    apiManager:
      accessToken: "<api-manager token>"
    # Only if you enable ngrok.features.privateEndpoints
    privateEndpointForwarder:
      accessToken: "<private-endpoint-forwarder token>"
```

If you reference a `Secret` instead, put each token under its own key: `AGENT_ACCESS_TOKEN`, `API_MANAGER_ACCESS_TOKEN`, and `PRIVATE_ENDPOINTS_ACCESS_TOKEN`.

### Uninstalling after a rollback

If you roll back via Helm, the `v1` CRs will still have finalizers. To fully uninstall the operator you must remove the finalizers.