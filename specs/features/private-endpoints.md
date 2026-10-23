# Private Endpoints Feature

## Overview

The private endpoints feature makes the ngrok account's private endpoints reachable from any pod in the cluster by their real URL, over HTTP, HTTPS, TLS and raw TCP. A pod runs `curl https://foo.ngrok.direct` or `redis-cli -h bar.ngrok.direct -p 6379` exactly as it would outside the cluster. No per-endpoint Kubernetes configuration, `service.namespace` URL convention or `kubernetes` binding is involved.

Only endpoints under `*.ngrok.direct` are mirrored.

Private endpoints is opt-in (`ngrok.features.privateEndpoints.enabled: false` by default).

## Configuration

| Helm Value                                                | Description                                                      | Default |
|-----------------------------------------------------------|------------------------------------------------------------------|---------|
| `ngrok.features.privateEndpoints.enabled`                 | Enable the private endpoints feature                             | `false` |
| `ngrok.credentials.privateEndpointForwarder.accessToken`  | Access token for the forwarder; falls back to `ngrok.credentials.accessToken` | `""` |

There are no selectors: every eligible private endpoint in the account is mirrored. The poll interval (10s) and the forwarder port range (20000–20999) are fixed.

## Components

| Component                    | Runs in          | Description                                                        |
|------------------------------|------------------|--------------------------------------------------------------------|
| PrivateEndpoint CRD          |                  | One per private endpoint URL, in the operator namespace            |
| PrivateEndpoint poller       | api-manager      | Mirrors the account's private endpoints into PrivateEndpoint CRs   |
| PrivateEndpoint controller   | api-manager      | Creates one Service per hostname and assigns each endpoint a forwarder port |
| Private Endpoint Forwarder   | own Deployment   | Listens on each forwarder port and dials the endpoint over private dial |

## Flow

1. Every 10s the poller lists the account's endpoints from the ngrok API and creates, restores or deletes `PrivateEndpoint` CRs in the operator namespace to match (see [Endpoint Selection](#endpoint-selection)).
2. The PrivateEndpoint controller gives each hostname one `ClusterIP` Service in the operator namespace, named after the hostname (`foo.ngrok.direct` → `foo-ngrok-direct`). The Service has one port per endpoint on that hostname, mapping the endpoint's port to a forwarder port unique within the cluster.
3. The controller writes the Service's ClusterIP and the endpoint's forwarder port to the CR's status and sets `Ready`.
4. Every forwarder replica opens a listener on each forwarder port in status.
5. Cluster DNS rewrites `foo.ngrok.direct` to `foo-ngrok-direct.<operator-namespace>.svc.cluster.local` (see [DNS](#dns)).

A pod reaching an endpoint:

```
pod ─DNS foo.ngrok.direct─▶ cluster DNS rewrite ─▶ foo-ngrok-direct.<ns>.svc ─▶ ClusterIP
pod ─TCP ClusterIP:443─▶ Service ─▶ forwarder :<forwarderPort> ─▶ private dial foo.ngrok.direct:443 ─▶ endpoint
```

The forwarder never inspects or terminates traffic. It reads no Host header or SNI, and does no TLS. Private dial matches `(hostname, port)` to the endpoint, so one IP per hostname plus the destination port is enough.

| Scheme  | Who terminates TLS                                     | Client sees                          |
|---------|--------------------------------------------------------|--------------------------------------|
| `http`  | No TLS on the client leg                               | Plain HTTP                           |
| `https` | ngrok, with its publicly trusted `*.ngrok.direct` cert | A certificate that verifies          |
| `tls`   | The endpoint's origin, end to end                      | The origin's own certificate         |
| `tcp`   | n/a                                                    | Raw bytes                            |

Within about one poll interval of an endpoint starting, its CR is `Ready` and the forwarder listens on its port. When an endpoint goes away, its CR and its port on the Service are removed on the next poll; when it was the hostname's last endpoint, the Service is deleted too.

## Endpoint Selection

The poller lists every endpoint in the account. Server-side filtering is not used: the ngrok API supports a `filter` parameter on the endpoints list, but the ngrok-api-go client the operator uses does not expose it. Selection is done client-side by the rules below.

An endpoint is mirrored when all of the following hold:

- It is not bound as `kubernetes` (those belong to [bindings](bindings.md)).
- Its URL parses: scheme `http`, `https`, `tls` or `tcp`, a hostname, and a port. `http` defaults to 80, `https` and `tls` to 443; `tcp` must give one.
- Its hostname is under `.ngrok.direct`.

Pooled endpoints share one URL and are mirrored as a single CR. Endpoints that fail to parse are skipped and logged at debug level.

## DNS

The operator never edits cluster DNS, and the chart does not configure it. The cluster administrator must add a rule to cluster DNS that maps each single-label `<name>.ngrok.direct` to `<name>-ngrok-direct.<operator-namespace>.svc.<cluster-domain>`, answering with the queried name. For CoreDNS:

```
rewrite stop name regex ^([a-z][a-z0-9-]*)\.ngrok\.direct\.$ {1}-ngrok-direct.<ns>.svc.cluster.local. answer auto
```

- Only single-label names match, so other names keep resolving as before.
- The rule matches by syntax, not by known endpoints: an `*.ngrok.direct` name with no Service gets `NXDOMAIN`.

For development on kind, `make kind-private-endpoints-dns` (`scripts/kind-private-endpoints-dns.sh`) adds this rule to the main `.:53` server block of the kind cluster's CoreDNS and restarts it. Re-running it replaces the rule. It is not meant for other clusters.

## Coexistence with Bindings

Private endpoints and [bindings](bindings.md) use separate CRDs, controllers, Deployments and listeners, and both can be enabled at once. The poller skips `kubernetes`-bound endpoints, so the two features never select the same endpoint. Forwarder ports are container ports on the private-endpoint-forwarder pods, so they cannot collide with the ports the bindings forwarder allocates.

## Uninstall

Nothing the feature creates carries a finalizer, and it creates nothing on the ngrok side, so uninstalling never waits on it and the [drain](draining.md) does not touch it under either policy. What remains depends on how the operator is removed:

| Removal | PrivateEndpoint CRs | Per-hostname Services |
|---------|---------------------|-----------------------|
| `helm uninstall` with `installCRDs: true` | Deleted with the CRD | Left in the release namespace |
| `helm uninstall` with CRDs installed separately | Left | Left |
| `ngrok.features.privateEndpoints.enabled: false` | Left; adopted again if re-enabled | Left; adopted again if re-enabled |
| Deleting the release namespace | Deleted | Deleted |

Leftover Services select no pods, so their names still resolve through the CoreDNS rewrite but connections to them fail. Remove them with `kubectl delete svc -n <namespace> -l ngrok.com/managed-by=private-endpoint-poller`, and remove the cluster's CoreDNS rewrite rule.

## Known Limitations

| Limitation | Detail |
|------------|--------|
| Account-wide exposure | Every mirrored private endpoint is reachable from every pod in the cluster. There are no selectors, and DNS configuration is not authorization. |
| No pod identity | The dial carries no identity of the calling pod, so Traffic Policy on the endpoint cannot match the caller. |
| Single-label hostnames | Only `<label>.ngrok.direct` is supported, where `<label>` is a DNS-1035 label of at most 50 characters. Other hostnames are mirrored but not wired (`UnsupportedHostname`). |
| UDP/443 egress | The forwarder dials over QUIC only, so the cluster needs outbound UDP/443. |
| Capacity | Forwarder ports come from 20000–20999, so a cluster serves at most 1000 endpoint-ports. Forwarder replicas add throughput, not capacity. |
| `Ready` means configured | `Ready=True` means the Service and forwarder port are in place, not that a forwarder has bound the port, that DNS is configured, or that the endpoint is reachable. |
| IPv4 | Forwarder listeners bind `0.0.0.0` and status carries one ClusterIP. |
| One install per cluster | The DNS rule sends `*.ngrok.direct` to one namespace, so only one installation per cluster can serve private endpoints. |
| No idle timeout | A connection stays open until one side closes it. |
| Trusted namespace | The controller and forwarder act on any PrivateEndpoint in the operator namespace. Write access to PrivateEndpoints or their status there amounts to routing control. |

## Related Specs

- [PrivateEndpoint CRD](../crds/privateendpoint.md)
- [PrivateEndpoint controller](../controllers/privateendpoint.md)
- [Private Endpoint Forwarder controller](../controllers/private-endpoint-forwarder.md)
- [Private Endpoint Forwarder Helm values](../helm/private-endpoint-forwarder.md)
- [Private Endpoint Forwarder RBAC](../rbac/private-endpoint-forwarder.md)

## When Disabled

When `ngrok.features.privateEndpoints.enabled: false`:
- The private-endpoint-forwarder Deployment, ServiceAccount and Roles are not created
- The api-manager does not run the poller or the PrivateEndpoint controller
- The `PrivateEndpoint` CRD is still installed with the other CRDs

Existing PrivateEndpoint CRs and Services are not cleaned up when the feature is turned off; see [Uninstall](#uninstall).
