# IPPolicy Controller

## Summary

The IPPolicy controller reconciles `IPPolicy` resources by creating and managing IP policies and their rules in the ngrok API.

During the migration window it reconciles both the canonical `ngrok.com/v1` `IPPolicy` and the deprecated `ingress.k8s.ngrok.com/v1alpha1` `IPPolicy`, as a pair keyed by namespace/name. See [Migration from `ingress.k8s.ngrok.com`](#migration-from-ingressk8sngrokcom).

## Watches

| Resource   | Relation | Predicate                              |
|------------|----------|----------------------------------------|
| `IPPolicy` (`ngrok.com/v1`) | Primary  | AnnotationChanged or GenerationChanged |
| `IPPolicy` (`ingress.k8s.ngrok.com/v1alpha1`, deprecated) | Same namespace/name key as its v1 twin | AnnotationChanged or GenerationChanged |

## Reconciliation Flow

1. Add finalizer.
2. Create or update the IP Policy remote resource via `IPPoliciesClient`.
3. Reconcile IP Policy Rules via `IPPolicyRulesClient`:
   - Create new rules
   - Update existing rules
   - Delete rules that are no longer in the spec
4. Update status with ID, rule statuses, and conditions.
5. Call `ReconcileStatus()`.

## Created Resources

- IP Policy (via ngrok API)
- IP Policy Rules (via ngrok API)

## Status

| Field    | Description                              |
|----------|------------------------------------------|
| `id`     | ngrok IP policy ID                       |
| `rules`  | Status of each rule (id, cidr, action)   |

## Conditions

| Type                      | Description                               |
|---------------------------|-------------------------------------------|
| `IPPolicyCreated`         | Whether the ngrok IP policy was created   |
| `IPPolicyRulesConfigured` | Whether all rules were configured         |
| `Ready`                   | Overall readiness                         |

A deprecated object whose v1 twin manages the IP policy reports `Ready=True` with reason `SupersededByV1`.

## Migration from `ingress.k8s.ngrok.com`

A deprecated and a canonical `IPPolicy` with the same namespace and name are twins. Each pass loads both:

- **Adoption:** a canonical object without an ID takes over its twin's ID instead of creating a new ngrok IP policy, so references to the `ipp_...` ID keep working. The ID is written to the canonical object's status before the deprecated object's finalizer can be released.
- **One writer:** when both hold the same ID, or the deprecated object has none, only the canonical object's spec is pushed. The deprecated object makes no ngrok API calls.
- **Deletes:** the ngrok IP policy is deleted only when no remaining object (one that exists and isn't being deleted) holds its ID. Deleting the canonical object hands the IP policy back to its twin in the same pass.
- **Different IDs:** twins that hold different IDs are both left as they are, and an `IPPolicyIDConflict` warning event is emitted on each.
- **Drain:** only deletes run; there is no adoption, so drain never waits on it.
