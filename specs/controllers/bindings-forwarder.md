# Bindings Forwarder Controller

## Executive Summary

The Bindings Forwarder controller manages TCP listeners for `BoundEndpoint` resources. It bridges incoming connections from the upstream service through mTLS to the ngrok ingress endpoint.

## Watches

| Resource         | Relation | Predicate |
|------------------|----------|-----------|
| `BoundEndpoint`  | Primary  | Manual setup (unmanaged controller) |

## Reconciliation Flow

1. Fetch the KubernetesOperator CR for binding configuration and the ingress endpoint address.
2. Fetch the TLS Secret for mTLS authentication.
3. Create a TLS dialer with the client certificate.
4. Listen on the allocated port for the BoundEndpoint.
5. For each incoming connection:
   - Look up the source Pod by client IP (via field indexer on `status.podIP`).
   - Upgrade the connection to a binding connection via mux protocol.
   - Join the client connection with the ngrok ingress endpoint connection.
6. Close the listener when the BoundEndpoint is deleted.

## Created Resources

- TCP listeners (in-process, not Kubernetes resources)

## Notes

- This controller runs in the bindings-forwarder deployment, not the api-manager.
- Leader election is disabled for the bindings-forwarder.
- The controller uses `statusID()` that returns namespace/name (always non-empty), so the Update handler is always used.
- See [features/bindings.md](../features/bindings.md) for the full feature overview.
