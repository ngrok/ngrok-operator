//TODO: Fix headers throughout file

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
"After the rollback window" steps in the
0.24 upgrade guide that
this guide lists as required below.

This migration has 4 major breaking changes:

- API Key and Authtoken have been removed and replaced with access tokens
- The Helm Values schema has been refactored, requiring any values you set to likely need to be changed.
- Some minor Spec fields and annotations have been changed (Called out below: TODO: Link to section)
- The CRDs have moved from `V1Alpha1` to `V1` and also moved groups to all be in the `ngrok.com` group
    - Note: Some CRDs have their resources created by the operator, which it will manage migrating after upgrading. The others are managed by you, you will need to change after the upgrade. TODO: Phrasing.

## TL;DR checklist

TODO: Link to sections

1. Prerequisite requirements met
2. Create an access token
3. Migrate Helm values to the new schema
4. Remove deprecated fields dropped by 0.25 
5. Upgrade `ngrok-crds` Helm Chart if installed separately
6. Helm upgrade, verify health
7. Migrate your alpha objects to their v1 versions

## Prerequisite Requirements

### Confirm you are on 0.24

If you are not follow the upgrade guide //TODO: add link

### Back up your 0.24 helm release

```bash
RELEASE=ngrok-operator
NAMESPACE=ngrok-operator

helm get values "$RELEASE" --namespace "$NAMESPACE" --all \
> ngrok-operator-0.24-values.yaml
helm get manifest "$RELEASE" --namespace "$NAMESPACE" \
> ngrok-operator-0.24-manifest.yaml
```

## Create an access token

In 0.25 we move to using access tokens instead of apiKey+authtoken. Provision an access token at https://dashboard.ngrok.com/access-tokens. **NOTE:** Keep all credentials valid until the end of the upgrade process.  TODO: Link to bottom section that explains the needed scopes

Follow the instructions below for your secret integrations strategy:

#### Using Helm Values to Reference Secret by Name (`credentails.secret.name`)

Create a new secret (with a new name) with the access token you created above. Later, you will pass the reference to the new Helm chart.

```markdown
apiVersion: v1
kind: Secret
metadata:
  name: my-ngrok-credentials-v2
  namespace: ngrok-operator
type: Opaque
data:
  AGENT_ACCESS_TOKEN: <base64-encoded-access-token>
  API_MANAGER_ACCESS_TOKEN: <base64-encoded-access-token>
```

#### Set Secret Value Directly in Helm Chart (`credentials.apiKey + credentials.authtoken`)

You will set the value directly when upgrading the Helm chart. No action is needed now. TODO: Link to helm upgrade section

## Migrate Helm values to the new schema

The 0.25 chart reorganizes its values into three groups: the chart's own settings stay at the top level, the operator's configuration moves under `ngrok`, and pod settings move under `components`. There are no aliases for the old keys: a values file that still sets one fails the render and lists each old key with its new location. 

The mapping can be found in the table below:

| 0.24 value | 0.25 value |
| --- | --- |
| `description`, `region`, `rootCAs`, `serverAddr`, `apiURL`, `clusterDomain` | `ngrok.<same>` |
| `ngrokMetadata`, `metaData` | `ngrok.metadata` |
| `credentials.*` | `ngrok.credentials.*` |
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
| `podAnnotations`, `podLabels`, `nodeSelector`, `tolerations`, `affinity`, `podAffinityPreset`, `podAntiAffinityPreset`, `nodeAffinityPreset`, `topologySpreadConstraints`, `priorityClassName`, `extraEnv` | `components.common.<same>` (applies to every component) |
| `lifecycle`, `extraVolumes`, `extraVolumeMounts` | `components.common.<same>` (as in 0.24, these apply to every component) |
| `replicaCount`, `resources`, `terminationGracePeriodSeconds`, `podDisruptionBudget.*`, `serviceAccount.*` | `components.apiManager.<same>` |
| `agent.*` | `components.agent.*` |
| `bindings.forwarder.*` | `components.bindingsForwarder.*` |
| `nameOverride`, `fullnameOverride`, `commonLabels`, `commonAnnotations`, `image.*`, `installCRDs`, `crdAccessRoles.*` | unchanged |

Three behaviors change along with the keys:

- Top-level pod settings such as `podAnnotations` and `tolerations` now apply to
the agent and bindings-forwarder as well as the api-manager. To keep a
setting on one component only, set it on that component instead, for example
`components.apiManager.tolerations`.
- Log settings can be overridden for one component under
`components.<component>.log`, for example `components.agent.log.level: debug`.
- An empty operator setting means "use the operator's built-in default". The
defaults are listed in the chart README.

#### //TODO: include migration script

## Remove deprecated fields dropped by 0.25

Fields that were deprecated in 0.24 are now removed in 0.25. Those fields are:

- Domain spec.resolves_to -> resolvesTo
- CloudEndpoint trafficPolicyName / trafficPolicy.policy -> targetRef / inline
- Selectors on bindings.k8s.ngrok.com/* labels + endpoint-url annotation -> ngrok.com/*
- Tooling on k8s.ngrok.com/* controller labels, computed-url, finalizer

If any of those exist, follow the 0.24 guide to update them //TODO: Link to 0.24 guide section

https://github.com/ngrok/ngrok-operator/blob/main/docs/upgrading-to-0.24.md#remove-domainspecregion
https://github.com/ngrok/ngrok-operator/blob/main/docs/upgrading-to-0.24.md#migrate-cloudendpoint-traffic-policies
https://github.com/ngrok/ngrok-operator/blob/main/docs/upgrading-to-0.24.md#update-selectors-for-operator-written-bindings-labels
https://github.com/ngrok/ngrok-operator/blob/main/docs/upgrading-to-0.24.md#rename-user-set-annotations

- <!-- old: 206-336. Should already be done if 0.24 guide followed. Short intro, then:All safe to do on 0.24 and survive rollback. -->

## Upgrade `ngrok-crds` Helm Chart if installed separately

By default, the Helm chart comes packaged with the CRDs, if you installed them separately, you need to upgrade the CRDs independently. Helm Chart 0.5.0 introduces `v1` versions of CRDs in new group locations that are canonical and deprecating the `v1alpha1` versions of those CRDs.

### Upgrade Instructions

```bash
TARGET_VERSION=0.5.0

helm repo update ngrok

helm upgrade "$RELEASE" ngrok/ngrok-crds \
  --version "$TARGET_VERSION" \
  --values path/to/your-values.yaml
```

### Verification Instructions

Verify the new CRDs are available

```bash
kubectl get crds | grep ngrok | grep -v k8s
agentendpoints.ngrok.com                   Namespaced   v1(storage)         2026-11-11T19:45:16Z
cloudendpoints.ngrok.com                   Namespaced   v1(storage)         2026-11-11T19:45:16Z
domains.ngrok.com                          Namespaced   v1(storage)         2026-11-11T19:45:16Z
ippolicies.ngrok.com                       Namespaced   v1(storage)         2026-11-11T19:45:16Z
kubernetesoperators.ngrok.com              Namespaced   v1(storage)         2026-11-11T19:45:16Z
privateendpoints.ngrok.com                 Namespaced   v1(storage)         2026-11-11T19:45:16Z
trafficpolicies.ngrok.com                  Namespaced   v1(storage)         2026-11-11T19:45:16Z
```

Note: Both the old and new versions of the CRDs exist at the same time in the cluster (see the `grep -v k8s` filtering out the old ones). You should query by the fully qualified CRD name such as `agentendpoints.ngrok.com,` not just `agentendpoints` as it could resolve to the `v1alpha1` versions and not be obvious.

## Helm upgrade, verify health

### Helm Upgrade to 0.25

```bash
TARGET_VERSION=0.25.0

helm repo update ngrok

helm upgrade "$RELEASE" ngrok/ngrok-operator \
  --namespace "$NAMESPACE" \
  --version "$TARGET_VERSION" \
  --values path/to/your-values.yaml \
  --wait
```

If any of the new Helm values are named incorrectly, you may see an error like this:

```bash
Error: UPGRADE FAILED: execution error at (ngrok-operator/templates/api-manager/deployment.yaml:1:4):

These Helm values moved in 0.25 and are no longer read at their old location:

  credentials -> ngrok.credentials (apiKey and authtoken are replaced by accessToken)
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

CRDs managed by the Operator will automatically create new `v1` versions of the existing `v1alpha1` Custom Resource Objects, without making changes in the ngrok API. CRDs managed by the User are left alone.

### Rolling back

Rolling back at this point is safe and easy via a `helm rollback` command. The `v1` resources created by the `0.25.0` release survive a rollback due to the `helm.sh/resource-policy: keep`  annotation. The previous `0.24.0` release will ignore them and allow you to re-upgrade again later safely. **NOTE:** When rolled back to `0.24.0`, the `v1alpha1` resources are now the primary resources, reverting changes made to the `v1` objects while in `0.25.0` . Any changes done after updating to `0.25.0` would need to be mirrored on the `v1alpha1` objects to persist.

## Migrate your alpha objects to their v1 versions

Once the Operator is running, you can create `v1` versions of them. Both the `v1alpha1` version and the `v1` versions can live side by side without conflicting or resulting in duplicate resources being created. Instead, the operator will see that there is a pair of resources of different `Kinds` but with the same `spec.name` and prefer the `v1` as the source of truth, adopting the `v1alpha1` resources `status` fields like `ID` and adds the status condition of `Ready` and reason `SupersededByV1` to the `v1alpha1` indicating that the `v1` version is the primary object.

Trying to modify the `v1alpha1` in this state will result in a `SupersededByV1` warning event, and changes will be ignored, indicating that the changes should be made on the `v1` version. **NOTE:** Changes to `v1` are not persisted to `v1alpha1` objects.

**NOTE**: If you use gitops tools like ArgoCD, do not just edit the versions and kinds in place as it may prune the old `v1alpha1` before making the new `v1` versions, which may result in intermittent downtime

### Spec.Metadata

As a part of the migration to V1, the previous `spec.metadata` fields of CRDs, where they existed, moved from a `JSON`  String to a Map. If you had set this previously on your `v1alpha1` CRs, you should change their format in the new `v1` Version.

Example:

```bash
apiVersion: <group>/v1alpha1
kind: <kind>
metadata:
  name: my-cr
spec:
  ...
  metadata: '{"team":"edge"}'
```

Becomes:

```bash
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

K8sOperator will automatically update CR objects that are managed by the operator, but the user is responsible for upgrading the CRs they manage. If a user changed the metadata on an operator created `Domain` that is unable to be converted into a map, a `DomainMigrationBlocked` event will be emitted.

If you delete a `v1alpha1` Domain without creating an `ngrok.com/v1` one, and an
Ingress, Gateway or endpoint still uses its hostname, the operator creates a
default `ngrok.com/v1` Domain for it, which takes over the same reservation.
Re-stamp instead if you set a `description`, `resolvesTo` or `reclaimPolicy`
you want to keep.

### IPPolicy

`ingress.k8s.ngrok.com/v1alpha1` `IPPolicy` is deprecated. Its replacement is `ngrok.com/v1` `IPPolicy`. The spec is unchanged except `spec.metadata`, which must be a map as described above.

All `IPPolicy` objects are user created, so the user is responsible for all migrations.

### Cloud Endpoints

`ngrok.k8s.ngrok.com/v1alpha1` `CloudEndpoint` is deprecated. Its replacement is `ngrok.com/v1` `CloudEndpoint`. The spec is unchanged except `spec.metadata`, which must be a map:

```yaml
apiVersion: ngrok.k8s.ngrok.com/v1alpha1
kind: CloudEndpoint
metadata:
  name: my-endpoint
spec:
  url: <https://example.ngrok.app>
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
  url: <https://example.ngrok.app>
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
  url: <https://example.ngrok.app>
  upstream:
    url: <http://my-service.my-namespace:80>
  metadata: '{"team":"edge"}'
```

becomes:

```yaml
apiVersion: ngrok.com/v1
kind: AgentEndpoint
metadata:
  name: my-endpoint
spec:
  url: <https://example.ngrok.app>
  upstream:
    url: <http://my-service.my-namespace:80>
  metadata:
    team: edge
```

While both a `v1alpha1` version and `v1` version exist, as long as they have the same name and namespace, the operator dedupes them and uses the `v1` as the source of truth so you only have 1 `AgentEndpoint` online to avoid billing for multiple endpoints.

## Follow ups

**NOTE:** Only do the following if you do not want to roll back anymore at all!

After upgrading your resources to `v1` you have satisfied the requirements for this release. All `v1alpha1` resources are deprecated in this release and are safe to remove. However, to support dual read, the `v1alpha1` CRDs **are required** in the cluster while `0.25.0` is running and should not be removed in this release. 

At this point, you can also delete your Kubernetes ngrok credential `Secret` and you can delete the `API Key` and `Authtoken` from your account in the Dashboard. 

## What's next: 1.0 and 1.1

In `1.0`, the `v1alpha1` CRDs will be ignored by the controllers, and in `1.1`, they will be removed from Helm. This makes `0.25.0` a required stop for `1.0` .

## Appendix

### One token per component

By default, you can create 1 `Access Token` with all the permissions the operator needs which gets used by both the `API Manager` pod, the `Agent Manager Pod` , and the `Private Dial` pod if you have it enabled. If you want extra security and minimal access, you can create separate tokens for each with their scoped permissions:

### Agent Manager Pod

The Agent Manager Pod only needs a token with the scope `tunnels:start-tunnels`

### Private Dial

TODO: uhhh… no scope for private dial yet?? need full access? asking brian…

### API Manager Pod

The API Manager Pod needs a token with the following scopes:

- `domains:read`, `domains:write`
- `endpoints:read`, `endpoints:write`
- `kubernetesOperator:read` , `kubernetesOperator:write`
- `ipPolicies:read`, `ipPolicies:write` : If you use IP Policies.

### Uninstalling after a rollback

If you roll back via Helm, the `v1` CRs will still have finalizers. To fully uninstall the operator you must remove the finalizers.