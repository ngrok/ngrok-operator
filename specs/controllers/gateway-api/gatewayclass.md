# GatewayClass Controller

## Executive Summary

The GatewayClass controller accepts GatewayClass resources that match the operator's controller name. It sets the `Accepted` condition and manages a finalizer that prevents deletion while Gateways reference the class.

## Watches

| Resource       | Relation   | Predicate                                        |
|----------------|------------|--------------------------------------------------|
| `GatewayClass` | Primary    | `ShouldHandleGatewayClass` (checks controller name) + GenerationChanged |
| `Gateway`      | Secondary  | Map function to find parent GatewayClass         |

## Reconciliation Flow

1. Check if the GatewayClass matches the operator's controller name.
2. Set the `Accepted` condition to `True`.
3. If Gateways reference this class: add `GatewayClassGatewayExistsFinalizer`.
4. If no Gateways reference this class: remove the finalizer.

## Created Resources

None. This is a status-only controller.

## Conditions

| Type       | Reason     | Description                                           |
|------------|------------|-------------------------------------------------------|
| `Accepted` | `Accepted` | `"gatewayclass accepted by the ngrok controller"`     |

## Finalizer

The `GatewayClassGatewayExistsFinalizer` prevents the GatewayClass from being deleted while Gateways still reference it. This ensures Gateways are cleaned up before their class is removed.
