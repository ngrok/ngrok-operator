# ReferenceGrant Controller

## Executive Summary

The ReferenceGrant controller watches `ReferenceGrant` resources and passes them to the Driver to enable cross-namespace references in Gateway API routes.

## Watches

| Resource          | Relation | Predicate |
|-------------------|----------|-----------|
| `ReferenceGrant`  | Primary  | None      |

## Reconciliation Flow

1. If deleted: call `Driver.DeleteReferenceGrant()`.
2. Otherwise: call `Driver.UpdateReferenceGrant()`.

This is a simple pass-through controller — the Driver uses ReferenceGrant information to determine whether cross-namespace references in routes are authorized.

## Created Resources

None. This controller only updates Driver state.

## Configuration

ReferenceGrants can be disabled via `gateway.disableReferenceGrants: true`. When disabled:
- This controller is not registered.
- Cross-namespace references are allowed without explicit grants.
