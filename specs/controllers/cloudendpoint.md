# CloudEndpoint Controller

## Summary

The CloudEndpoint controller reconciles `CloudEndpoint` resources by creating and managing ngrok cloud endpoints via the ngrok API. It ensures associated domains exist, resolves traffic policies, and keeps the endpoint synchronized with the desired state.

During the migration window it reconciles both the canonical `ngrok.com/v1` `CloudEndpoint` and the deprecated `ngrok.k8s.ngrok.com/v1alpha1` `CloudEndpoint`, as a pair keyed by namespace/name. See [Migration from `ngrok.k8s.ngrok.com`](#migration-from-ngrokk8sngrokcom).

## Watches

| Resource              | Relation   | Predicate                                    |
|-----------------------|------------|----------------------------------------------|
| `CloudEndpoint` (`ngrok.com/v1`) | Primary    | AnnotationChanged or GenerationChanged       |
| `CloudEndpoint` (`ngrok.k8s.ngrok.com/v1alpha1`, deprecated) | Same namespace/name key as its v1 twin | AnnotationChanged or GenerationChanged |
| `TrafficPolicy`  | Secondary  | Indexed by `spec.trafficPolicy.targetRef`; DELETE events filtered |
| `Domain`              | Owned      | All events                                   |

## Reconciliation Flow

1. Ensure the associated Domain exists via `DomainManager.EnsureDomainExists()`.
2. Fetch the traffic policy (inline or by name).
3. Create or update the cloud endpoint via the ngrok API — **this happens regardless of whether the associated Domain is ready**. A domain that is not ready (e.g., certificate still provisioning) is still usable as a URL target; the endpoint is created so that traffic can begin flowing as soon as the domain becomes ready.
4. Update status with the endpoint ID, domain reference, and conditions.
5. Call `ReconcileStatus()`.

## Created Resources

- ngrok cloud endpoint (via ngrok API)
- `Domain` CR (via DomainManager)

A `Domain` whose reservation was skipped because a wildcard parent already
covers it satisfies `DomainReady` like any other ready Domain, and the
`CoveredByWildcardDomain` reason propagates onto the endpoint's condition. See
[domain.md](domain.md#wildcard-domain-coverage).

## Status

| Field        | Description                            |
|--------------|----------------------------------------|
| `id`         | The ngrok API resource ID              |
| `domainRef`  | Reference to the associated Domain CR  |

## Conditions

| Type    | Description                                    |
|---------|------------------------------------------------|
| `Ready` | Overall readiness of the cloud endpoint        |

A deprecated object whose v1 twin manages the endpoint reports `Ready=True` with reason `SupersededByV1`.

## Migration from `ngrok.k8s.ngrok.com`

A deprecated and a canonical `CloudEndpoint` with the same namespace and name are twins. The key is never the URL: pooling lets distinct objects share one. Each pass loads both:

- **Adoption:** a canonical object without an ID takes over its twin's ID instead of creating a new cloud endpoint. The ID is written to the canonical object's status right after its finalizer is added and before anything else, so before the deprecated object's finalizer can be released. A `CloudEndpointAdopted` event is emitted. The adopted endpoint then gets the canonical spec, so a different `spec.url` moves it.
- **One writer:** while the canonical object exists and isn't being deleted, only it calls the ngrok API. The deprecated object makes no API calls and reports `Ready=True` with reason `SupersededByV1`. A user-created deprecated object gets a `DeprecatedAPIGroup` warning event when it becomes passive or is edited; an operator-generated one (a controller-name label, or a Service controller owner) doesn't.
- **Deletes:** the cloud endpoint is deleted only when no remaining object (one that exists and isn't being deleted) holds its ID. Just before deleting, the twin is re-read without the cache, failing closed: NotFound or an unserved kind means no twin; Forbidden means the twin is treated as present and the delete is skipped; any other error requeues. A deprecated object's delete also waits (one-second requeue) while a canonical twin has no ID yet, since it may still adopt; drain never waits. Deleting the deprecated object while its twin holds the ID releases its finalizer without an ngrok call. Deleting the canonical object hands the endpoint back to its twin in the same pass.
- **Different IDs:** the canonical object is still the only writer. If the deprecated object's endpoint no longer exists, its ID follows the canonical object's. If the canonical object's own endpoint no longer exists (a rollback can leave this), it adopts the deprecated object's live ID instead of creating one. If both endpoints exist, neither is deleted and a `CloudEndpointIDConflict` warning event is emitted on each.
- **Drain:** drain deletes the deprecated kind first, so for a pair sharing an ID the canonical delete removes the endpoint once.

Writers of generated objects:

- **Ingress and Gateway API:** the managerdriver writes v1 objects with the legacy objects' names. A legacy object it generated is left alone while still desired and deleted, before its v1 twin, once it isn't.
- **Service:** names come from `GenerateName`, so the first v1 object takes the name of one of the Service's legacy objects, preferring one that holds an ID, then the lowest name. Other legacy objects the Service owns are deleted. Until the v1 object has a `domainRef`, the Service's status is computed from its legacy twin.

These are the twin rules in [crd-group-migration.md](../crd-group-migration.md#twin-rules-r1). The rule 3 extension and the Service name selection are listed under [Exceptions](../crd-group-migration.md#exceptions).

## Error Handling

| Error                          | Behavior                                       |
|--------------------------------|------------------------------------------------|
| Codes 18016, 18017             | Retryable (endpoint pooling state conflicts)   |
| `ErrDomainNotReady`            | Requeue after 10s                              |
| `ErrInvalidTrafficPolicyConfig`| No requeue                                     |
| Default                        | Via `CtrlResultForErr`                         |
