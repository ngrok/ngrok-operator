# TrafficPolicy Controller

## Summary

The TrafficPolicy controller is a validation-only controller. It validates the JSON syntax of traffic policies, reports the result via `Ready`/`Valid` status conditions, emits warnings for deprecated features, and triggers re-sync of dependent endpoints.

## Watches

| Resource              | Relation   | Predicate                                    |
|-----------------------|------------|----------------------------------------------|
| `TrafficPolicy`  | Primary    | AnnotationChanged or GenerationChanged       |

## Reconciliation Flow

1. Parse the `spec.policy` field as JSON.
2. Set the `Ready` and `Valid` conditions from the parse result and update status. Reasons: `TrafficPolicyParseFailed` (parse error), `LegacyPolicyFormat` (legacy directions), `EnabledFieldDeprecated` (`enabled` set), `TrafficPolicyValid` otherwise.
3. If parsing fails, emit a `TrafficPolicyParseFailed` event and return success (a malformed policy will not fix itself; the generation-changed predicate re-triggers on spec edits).
4. Check for deprecated features (legacy `directions` field, `enabled` field on rules).
5. If deprecations found, emit `PolicyDeprecation` events.
6. Call `Driver.SyncEndpoints()` to trigger re-reconciliation of dependent endpoints.

## Created Resources

None. This controller does not create any remote resources.

## Events

| Event                     | Description                         |
|---------------------------|-------------------------------------|
| `TrafficPolicyParseFailed`| Emitted when JSON parsing fails     |
| `PolicyDeprecation`       | Emitted when deprecated features are used |

## Notes

- The controller always succeeds reconciliation, even if the policy is malformed or contains deprecated features — failures are reported via conditions and Events, not requeues.
- Policy schema validation is performed by the ngrok API when the policy is applied to an endpoint, not by this controller.
- Changes to a TrafficPolicy trigger re-reconciliation of all endpoints that reference it (via secondary watches on endpoint controllers).
- See [features/traffic-policy.md](../features/traffic-policy.md) for the cross-cutting traffic policy feature.
