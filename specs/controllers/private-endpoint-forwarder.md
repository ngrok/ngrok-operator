# Private Endpoint Forwarder Controller

## Summary

The Private Endpoint Forwarder controller keeps one TCP listener open per forwarder port in `PrivateEndpoint` status and forwards each accepted connection to that endpoint's `hostname:port` over private dial. It copies bytes only: it reads no Host header or SNI and does no TLS.

## Watches

| Resource          | Relation | Predicate |
|-------------------|----------|-----------|
| `PrivateEndpoint` | Primary  | None; every change triggers a full sync |

The cache is scoped to the forwarder's own namespace.

## Reconciliation Flow

Every reconcile runs the same full sync, whatever the request. Duplicate detection and closing a listener depend on every endpoint that claims a port, not just the one that changed, so each sync recomputes from the cached list (no API call) instead of applying a single change. Deletes need no finalizer: listeners are in-process state.

1. List all PrivateEndpoints in the namespace.
2. Build the routing table from those with both `status.clusterIP` and `status.forwarderPort` set: `forwarderPort → hostname:port`.
3. Swap the table in atomically.
4. Open a listener on `0.0.0.0:<forwarderPort>` for each port in the table that has none.
5. Close the listener of each port no longer in the table.
6. Return any bind failures, so controller-runtime retries them with backoff.

A first sync runs once the cache has synced, so the table is marked synced even when there are no PrivateEndpoints.

## Routing Table

| Rule | Behavior |
|------|----------|
| Accept-time lookup | The target is looked up when a connection is accepted, not when the listener opens. A port reassigned to another endpoint routes to the new one without reopening the listener. |
| Unknown port | The connection is closed. |
| Ambiguous port | A port claimed by more than one PrivateEndpoint resolves to nothing, so its connections are closed rather than sent to a guessed target. An error naming the ports is logged when the set of ambiguous ports changes. |

## Connection Handling

Each accepted connection is dialed to the endpoint as `hostname:port` and joined with the client:

| Event | Behavior |
|-------|----------|
| Dial fails | The client is reset, rather than given a clean EOF that looks like an empty response. Dial time is bounded by private dial (5s per dial). |
| One direction reaches EOF | Its destination is half-closed (`CloseWrite`, or `Close` when unsupported). |
| Client half-closes first | No timer starts; the endpoint may take as long as it needs to respond. |
| Endpoint finishes first | The client gets a 5s drain to finish sending, then both sides are closed. A client that never closes cannot pin the stream. |
| Copy error (not EOF) | Both sides are aborted and the error is logged, so a truncated stream is never reported as an orderly close. A TCP client is reset; the private dial stream is cancelled. |

There is no idle timeout: a silent but live peer can hold a connection open.

## Private Dial Session

One private dial client per process, connecting over QUIC to the private dial server (`ngrok.features.privateEndpoints.privateDialServer`, default `quic.connect-endpoint.ngrok.com:443`) with the access token from `NGROK_ACCESS_TOKEN`.

| Phase | Behavior |
|-------|----------|
| Startup | Connects with a 30s timeout per attempt. Failures, including a rejected token, are retried with backoff from 1s up to 30s; the pod stays running but not Ready. |
| Connected | Transient outages are absorbed by the client without affecting readiness. |
| Permanently done | When the session ends for good (for example, a token revoked and rejected on reconnect), the process exits non-zero so the kubelet restarts the container and re-reads the token. |

## Health Checks

| Endpoint   | Check          | Passes when |
|------------|----------------|-------------|
| `/readyz`  | `routing-table` | The table has synced at least once and the last sync bound every listener it wanted |
| `/readyz`  | `private-dial`  | The private dial session has connected and has not failed |
| `/healthz` | `healthz`       | Always; liveness never depends on ngrok being reachable |

A listener that fails to bind holds readiness and is retried; it does not stop the process.

## Shutdown

On SIGTERM the manager stops and every listener is closed. Connections in flight are dropped; there is no connection draining or preStop hook.

## Created Resources

- TCP listeners (in-process, not Kubernetes resources)

## Notes

- This controller runs in the private-endpoint-forwarder Deployment, not the api-manager.
- Leader election is disabled: every replica serves every port, and the Service spreads connections across them.
- The forwarder opens a listener for any forwarder port in status, without checking it against 20000–20999.
- See [features/private-endpoints.md](../features/private-endpoints.md) for the full feature overview.
