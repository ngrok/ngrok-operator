# IPPolicy Controller

## Executive Summary

The IPPolicy controller reconciles `IPPolicy` resources by creating and managing IP policies and their rules in the ngrok API.

## Watches

| Resource   | Relation | Predicate                              |
|------------|----------|----------------------------------------|
| `IPPolicy` | Primary  | AnnotationChanged or GenerationChanged |

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
