# PrivateEndpoint Controller

## Summary

Two components in the api-manager wire private endpoints into the cluster. The **poller** mirrors the ngrok account's private endpoints into `PrivateEndpoint` CRs. The **controller** gives each hostname one `ClusterIP` Service and assigns each endpoint a forwarder port. Both run only when `ngrok.features.privateEndpoints.enabled` is `true`, only on the leader, and only in the operator namespace.

## Poller

A manager runnable that syncs once at start and then every 10 seconds.

### Sync Flow

1. List the account's endpoints from the ngrok API. The client-side rules below select private endpoints; the endpoints list is not filtered server-side.
2. Keep the endpoints that are not `kubernetes`-bound, whose URL parses, and whose hostname is under `.ngrok.direct`, keyed by CR name (see [CRD naming](../crds/privateendpoint.md#naming)). Pooled endpoints share a URL and collapse into one entry.
3. List the PrivateEndpoints in the operator namespace that carry `ngrok.com/managed-by: private-endpoint-poller`.
4. For each listed CR:
   - still desired: restore drift (below);
   - no longer desired: delete it.
5. Create a CR for each desired endpoint that has none.

Errors on individual CRs are collected and the rest of the sync continues.

### Convergence Rules

| Situation | Behavior |
|-----------|----------|
| The ngrok API list fails | Nothing is created, changed or deleted; the error is logged and the next poll retries. An API outage never deletes CRs. |
| A managed CR's spec or poller-owned labels were edited | Spec and the `managed-by` label are restored. Other labels and status are left alone. |
| A CR with the desired name exists without the `managed-by` label | Left alone, never adopted or overwritten. A `NotAdopted` Warning Event is recorded on it each poll; the API server folds repeats into one Event with a count. |
| A managed CR's endpoint is gone, or no longer eligible | The CR is deleted. |

## Controller

### Watches

| Resource          | Relation  | Predicate / Mapping                                                   |
|-------------------|-----------|-----------------------------------------------------------------------|
| `PrivateEndpoint` | Primary   | GenerationChanged; mapped to its normalized `spec.hostname`           |
| `Service`         | Secondary | Only with the `managed-by` label; mapped from its name back to the hostname (`foo-ngrok-direct` → `foo.ngrok.direct`) |

The controller is keyed by **hostname**, not by CR: the request name is the normalized hostname. Every endpoint on a hostname shares the one IP that DNS returns for it. Objects outside the operator namespace, and Services without the `managed-by` label, are ignored. `MaxConcurrentReconciles` is 1, because port allocation assumes a single worker.

### Reconciliation Flow

1. List all PrivateEndpoints in the operator namespace, uncached. CRs being deleted are skipped.
2. Derive the Service name from the hostname. If it cannot be mapped, set `UnsupportedHostname` on every CR of the hostname and stop: such a hostname never has a Service.
3. If no CR has this `spec.hostname`, delete the hostname's Service if it is managed, and stop.
4. Order the hostname's CRs by `spec.port`, then name. The first CR on each port gets a Service port; later ones get `PortConflict`.
5. Keep each CR's existing forwarder port where allowed, and allocate one otherwise (see [Port Allocation](#port-allocation)). A CR that gets no port gets `PortsExhausted`.
6. If no CR got a Service port, delete the hostname's Service and stop.
7. Create or update the Service (see [Created Resources](#created-resources)). If a Service with that name exists without the `managed-by` label, it is not modified and each CR that would have had a port gets `ServiceConflict`.
8. Write `status.clusterIP`, `status.forwarderPort` and `Ready=True` on each served CR, or `ServiceNotReady` if the Service has no ClusterIP yet.

Status is written only when it changes.

### Port Allocation

Forwarder ports come from 20000–20999. Each one is a container port on every forwarder pod and routes to exactly one endpoint.

- A port is **in use** when another hostname's CR holds it in status, or when another hostname's managed Service targets it. The Service check covers a status write that failed after the Service write, and a CR deleted before its Service.
- A CR **keeps** its current port when it is in range and no CR on another hostname with a lower name also holds it. When two hostnames claim one port, the lowest CR name keeps it and the other is reallocated.
- New ports are handed out by a cursor: the next free port after the last one allocated, wrapping to 20000. A freed port is reused only after the rest of the range has been cycled through, which gives forwarder replicas time to drop the old mapping before the port is reassigned.
- The cursor is in memory. After a restart or leader change it is reseeded from the highest port in use, so a port freed above every remaining one can be reused at once.

The allocator reads PrivateEndpoints and Services uncached because the informer may not yet have seen the forwarder ports the previous reconcile just wrote. Reusing one of those would send one endpoint's traffic to another.

### Requeue Behavior

| Condition                         | Requeue                                           |
|-----------------------------------|---------------------------------------------------|
| `PortsExhausted`                  | After 30s                                         |
| `ServiceConflict`                 | After 30s. The Service watch maps only labeled Services, so nothing else would enqueue the hostname when the foreign Service goes away. |
| `ServiceNotReady` (no ClusterIP)  | After 2s                                          |
| API error                         | controller-runtime backoff                        |
| `Ready`, `PortConflict`, `UnsupportedHostname` | None; the next change to the hostname's CRs or Services enqueues it |

### Created Resources

One `Service` per hostname, in the operator namespace:

| Field       | Value                                                                    |
|-------------|--------------------------------------------------------------------------|
| Name        | `<label>-ngrok-direct` for `<label>.ngrok.direct`                         |
| Labels      | `ngrok.com/managed-by: private-endpoint-poller`                           |
| Type        | `ClusterIP`                                                              |
| Selector    | `app.kubernetes.io/component: private-endpoint-forwarder`               |
| Ports       | One per served endpoint: name `port-<port>`, protocol TCP, `port: spec.port`, `targetPort: status.forwarderPort` |

The Service is deleted by name when its hostname has no CRs left, or none that can be served.

Neither CRs nor Services carry finalizers or owner references, and neither controller takes part in the [drain](../features/draining.md). See [Uninstall](../features/private-endpoints.md#uninstall).

## Notes

- The controller acts on any PrivateEndpoint in the operator namespace, not only those the poller created. A CR only ever serves its own `spec.hostname`.
- If a hostname's Service is replaced by a foreign one, the foreign Service is never modified or deleted.
- See [features/private-endpoints.md](../features/private-endpoints.md) for the full feature overview.
