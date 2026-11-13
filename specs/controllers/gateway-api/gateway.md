# Gateway Controller

## Summary

The Gateway controller reconciles `Gateway` resources that reference an ngrok-managed GatewayClass. It validates the Gateway, updates the Driver store, and triggers synchronization to materialize endpoints.

## Watches

| Resource  | Relation | Predicate |
|-----------|----------|-----------|
| `Gateway` | Primary  | None      |

## Reconciliation Flow

1. If deleted: remove from Driver store, call `Driver.Sync()`, remove finalizer.
2. Verify the referenced GatewayClass exists and is managed by ngrok.
3. If draining: skip.
4. Add finalizer.
5. Update the Gateway in the Driver store.
6. Call `Driver.Sync()` to materialize endpoints and write status.

## Created Resources

- `AgentEndpoint` and/or `CloudEndpoint` CRs (via Driver.Sync)
- `Domain` CRs (via Driver.Sync)

## Status

Status is written by the Driver at the end of every `Sync()`, from the whole store (`pkg/managerdriver/gatewayapi_status.go`), so it accounts for the routes attached to each listener. Every condition carries the Gateway's current `metadata.generation`. Conditions of other types, written by other controllers or users, are kept as they are; listeners removed from spec are removed from status.

### Addresses

`status.addresses` lists, with type `Hostname`, the Domain of each listener's hostname (or the Gateway's generated domain, for a listener without one): its `status.cnameTarget` for a custom domain, otherwise the domain itself, with any `*.` prefix trimmed.

### Gateway conditions

| Type | Status | Reason | When |
|------|--------|--------|------|
| `Accepted` | `True` | `Accepted` | Every listener is valid |
| `Accepted` | `True` | `ListenersNotValid` | At least one listener is valid and at least one is not |
| `Accepted` | `False` | `ListenersNotValid` | No listener is valid |
| `Programmed` | `True` | `Programmed` | Accepted and at least one address |
| `Programmed` | `False` | `AddressNotAssigned` | Accepted, waiting for the Domains to be reserved |
| `Programmed` | `False` | `Invalid` | Not accepted |

### Listener status

`supportedKinds` lists the route kinds the listener serves: `HTTPRoute` for HTTP and HTTPS, `TLSRoute` and `TCPRoute` for TLS and TCP when those CRDs are installed, narrowed to `allowedRoutes.kinds` when set. `attachedRoutes` counts routes accepted on the listener, including routes whose backends don't resolve.

| Type | Status | Reason | When |
|------|--------|--------|------|
| `Accepted` | `False` | `UnsupportedProtocol` | The protocol isn't HTTP, HTTPS, TLS or TCP (`supportedKinds` is empty) |
| `Accepted` | `False` | `PortUnavailable` | HTTP not on port 80, or HTTPS not on 443 |
| `Accepted` | `False` | `HostnameRequired` | No hostname, no `spec.addresses`, and no `generatedDomainSuffix` |
| `Accepted` | `True` | `Accepted` | Otherwise |
| `ResolvedRefs` | `False` | `InvalidRouteKinds` | `allowedRoutes.kinds` names a kind the listener can't serve |
| `ResolvedRefs` | `False` | `RefNotPermitted` | A certificateRef in another namespace without a ReferenceGrant (checked before existence) |
| `ResolvedRefs` | `False` | `InvalidCertificateRef` | A certificateRef that isn't a core Secret, doesn't exist, or doesn't hold a valid key pair |
| `ResolvedRefs` | `True` | `ResolvedRefs` | Otherwise |
| `Programmed` | `True` | `Programmed` | Accepted and references resolved |
| `Programmed` | `False` | `Invalid` | Otherwise |

## Error Handling

| Error               | Behavior                                                    |
|---------------------|-------------------------------------------------------------|
| Status update conflict | Retried via `retry.RetryOnConflict` with concurrency limit of 4 |
| Gateway not found   | Silently skipped (Gateway was deleted during sync)          |

## Notes

- The Gateway controller works in concert with the route controllers (HTTPRoute, TCPRoute, TLSRoute). The Driver considers both Gateway listeners and route rules when generating endpoints.
- HTTPRoute status is written by the Driver alongside Gateway status; see [httproute.md](httproute.md#status).
