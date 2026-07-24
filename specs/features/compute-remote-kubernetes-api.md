# Compute Remote Kubernetes API

This document defines the server-side contract for remotely managing Ship/Compute
replicas through a Kubernetes-API-compatible endpoint created by ngrok-operator.

## Transport and trust

The endpoint is an ngrok internal TLS AgentEndpoint. Compute is the only intended
client.

- The operator generates an ECDSA P-256 private key and CSR. The key remains in
  the cluster.
- Compute signs the CSR with a server CA trusted by the Compute Kubernetes
  client. The certificate SAN MUST contain the hostname returned in `endpoint`.
- Compute returns the CA bundle used to validate Compute client certificates.
- The AgentEndpoint requires a valid Compute client certificate.
- The in-cluster gateway authenticates to Kubernetes with its projected
  ServiceAccount token. Kubernetes credentials are never returned to Compute.

Client certificates SHOULD be short lived and identify the Compute service.
The initial implementation validates the issuing CA. A later protocol version
may add runner-specific URI SAN enforcement.

## Registration API

The operator calls this operation with the same ngrok API authentication used
for the runner-replicas API:

```http
PUT /v1/runners/{runner_id}/kubernetes-access
Content-Type: application/json
```

The operation is idempotent for a runner. The authenticated account MUST own
`runner_id`.

### Provisioning request

```json
{
  "state": "provisioning",
  "csr": "-----BEGIN CERTIFICATE REQUEST-----\n...\n"
}
```

Successful response:

```json
{
  "endpoint": "tls://ko_abc123.k8s.compute.internal",
  "server_certificate": "-----BEGIN CERTIFICATE-----\n...\n",
  "client_ca": "-----BEGIN CERTIFICATE-----\n...\n"
}
```

Requirements:

- Compute derives `endpoint` by lowercasing the runner ID, replacing `_` with
  `-`, and appending `.k8s.compute.internal`.
- Compute adds the derived endpoint hostname as a DNS SAN when signing the CSR.
- `endpoint` MUST remain stable for the lifetime of the runner.
- `server_certificate` MUST match the submitted public key and endpoint
  hostname.
- `client_ca` contains public certificates only.
- Repeating the request with a new CSR rotates the server certificate without
  changing the endpoint.

### Lifecycle update

After creating the AgentEndpoint, and periodically thereafter, the operator
sends:

```json
{
  "state": "ready",
  "endpoint": "tls://ko_abc123.k8s.compute.internal",
  "assigned_url": "tls://ko_abc123.k8s.compute.internal"
}
```

Before the AgentEndpoint Ready condition is true, `state` is `provisioning`
and `assigned_url` may be empty. The server MUST accept an empty
`assigned_url` while provisioning and MUST NOT connect until it has observed
`ready`. When `state` is `ready`, `assigned_url` MUST equal the registered
endpoint. Lifecycle updates return `200` with either an empty body or the
current registration object.

Recommended future states are `unavailable` and `deleting`; they are not emitted
by the first operator implementation.

## Kubernetes client

Compute configures a standard Kubernetes REST client with:

- Host: the registered endpoint
- TLS server name: endpoint hostname
- Root CAs: the server-signing CA
- Client certificate/key: Compute mTLS identity
- No Kubernetes bearer token

The gateway does not perform application-layer path or method filtering.
Kubernetes RBAC is the sole authorization layer. The gateway ServiceAccount is
currently granted:

| Resource | Access |
|---|---|
| API discovery and version | read |
| Deployments | read, watch, create, update, patch, delete |
| ReplicaSets | read, watch |
| Pods | read, watch |
| Pod metrics (`metrics.k8s.io`) | read |
| Events | read, watch |
| Services | read, watch, create, update, patch, delete |
| `ngrok-container-registry-*` Secrets | read by name, create, update, patch, delete |
| AgentEndpoints | read, watch, create, update, patch, delete |

Exec, attach, port-forward, logs, proxy subresources, ServiceAccount token
creation, RBAC, impersonation, cluster-scoped resources, and other namespaces
are denied by the configured Role. Broadening that Role immediately broadens
the remote API surface without requiring a gateway change.

Compute SHOULD use server-side apply with field manager `ngrok-compute`, label
every managed object with `ngrok-id=<replica-id>`, and establish list/watch
streams using `resourceVersion`. It MUST perform a full relist after watch
expiration or reconnection.

## Replica readiness

Scheduling is not readiness. Compute reports a replica Ready only when:

1. Deployment `observedGeneration` matches `metadata.generation`.
2. Desired, updated, and available replica counts have converged.
3. The selected Pod has `Ready=True` and all application containers are ready.
4. Every AgentEndpoint for the replica has `Ready=True`.

Container waiting/termination reasons and relevant warning Events should be
retained for user-visible status. A lost remote connection yields `Unknown`,
not `Failed`, and existing workloads are left running.

## Rollout

The Helm opt-in is:

```yaml
compute:
  enabled: false
  remoteAccess:
    enabled: true
    replicaCount: 1
```

`computeBaseURL` must address a server implementing this contract. Remote mode
and the app-replica poller are mutually exclusive.
