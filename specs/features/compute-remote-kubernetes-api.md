# Compute Remote Kubernetes API

This document defines the server-side contract for remotely managing Ship/Compute
replicas through a Kubernetes-API-compatible endpoint created by ngrok-operator.

## Transport and authentication

The endpoint is an ngrok internal HTTPS Agent Endpoint created in-process by
the operator's Compute API gateway with the ngrok Go SDK. Compute is the only
intended client.

- The operator generates a cryptographically random 256-bit access key whenever
  the api-manager starts.
- The operator sends the raw access key to Compute once during registration.
- The raw access key is retained in operator memory only for the registration
  request. Kubernetes stores only its SHA-256 verifier.
- Compute authenticates every Kubernetes API request with the access key as an
  HTTP bearer token.
- The gateway validates the bearer token, removes it, and authenticates the
  upstream request to Kubernetes with its projected ServiceAccount token.
- The internal endpoint is served directly from `Agent.Listen`; there is no
  Kubernetes Service or pod port for proxy traffic.
- The endpoint is pooled so multiple gateway replicas can serve the same stable
  URL during scaling and rolling updates.
- Remote access and the app-replica poller are mutually exclusive. When remote
  access is enabled, it takes precedence and the poller does not start.
- The in-cluster gateway authenticates to Kubernetes with its projected
  ServiceAccount token. Kubernetes credentials are never returned to Compute.

The access key is replaced whenever api-manager restarts. The server MUST treat
the most recently registered access key as authoritative. A brief loss of
access during rotation is acceptable; no overlap between old and new keys is
required.

## Registration API

The operator calls this operation with the same ngrok API authentication used
to register and manage the KubernetesOperator:

```http
PUT /v1/runners/{runner_id}/kubernetes-access
Content-Type: application/json
```

The operation is idempotent for a runner. The authenticated account MUST own
`runner_id`.

### Access publication

The payload is replacement-style: empty or omitted fields leave the previously
reported value in place. It is shaped to fold unchanged into a future runner
status/sync exchange.

First publication (mints the key; readiness not yet claimed):

```json
{
  "kubernetes_access_key": "QsdY07Kqk7mNmMTwFBqNlVYHBPm9gnqHrsEW96VoMSg",
  "kubernetes_access_namespace": "my-operator-compute"
}
```

Successful response:

```json
{
  "endpoint": "https://ko-abc123.k8s.compute.internal"
}
```

Steady-state report (periodically, and whenever readiness changes):

```json
{
  "kubernetes_access_namespace": "my-operator-compute",
  "kubernetes_access_ready": true
}
```

Requirements:

- Compute derives `endpoint` by lowercasing the runner ID, replacing `_` with
  `-`, and appending `.k8s.compute.internal`. The endpoint is a pure function
  of the runner ID: it MUST remain stable for the lifetime of the runner and
  is returned by every publication.
- `endpoint` MUST use the `https` scheme and its hostname MUST end in
  `.internal`.
- `kubernetes_access_key` is an opaque bearer credential. The server MUST
  store it using the same protections as other service credentials and MUST
  NOT log it. The first publication for a runner MUST carry it (and a
  namespace); later publications carry it only to rotate. Repeating the
  request with a new key replaces the previous key without changing the
  endpoint.
- `kubernetes_access_namespace` is the dedicated namespace in which Compute
  creates and manages replica resources. The authenticated runner is
  authoritative for it, including moves after the initial publication.
- `kubernetes_access_ready` reports whether the runner's exported API
  endpoint is up. The server MUST NOT connect until it has observed a ready
  publication, and treats ready runners as server-reconciled.
- A successful response means the server has committed the publication.

## Server implementation checklist

The Compute server must make the following changes from the mTLS contract:

1. Replace the provisioning request's `csr` field with `access_key`.
2. Stop issuing and returning `server_certificate` and `client_ca`.
3. Return the stable endpoint as `https://<runner>.k8s.compute.internal`.
4. Replace the runner's stored access key on every provisioning request.
5. Configure the runner's Kubernetes `rest.Config.BearerToken` with that key.
6. Discard the superseded key immediately; retry while the operator gateway
   temporarily returns `401`, is unavailable, or is still provisioning.
7. Never return or log the access key after accepting the provisioning request.

## Kubernetes client

Compute configures a standard Kubernetes REST client with:

- Host: the registered endpoint
- Bearer token: the most recently registered `access_key`
- No client certificate or custom CA configuration

For example:

```go
config := &rest.Config{
    Host:        registration.Endpoint,
    BearerToken: accessKey,
}
clientset, err := kubernetes.NewForConfig(config)
```

The gateway returns `401 Unauthorized` with `WWW-Authenticate: Bearer` when the
header is missing or the access key is invalid. Once authenticated, the gateway
does not perform application-layer path or method filtering. Kubernetes RBAC
is the authorization layer. The gateway runs in the operator's release
namespace, while its Role and RoleBinding exist only in the dedicated replica
namespace. The agent manager also watches the replica namespace and has a
separate Role there so it can reconcile AgentEndpoints created by Compute.

Remote access adds the replica namespace to the agent's watch scope rather than
replacing it, so it composes with Ingress support: the chart passes
`--watch-namespace` once per namespace when `ingress.watchNamespace` is set, and
passes no `--watch-namespace` at all when Ingress is already cluster-wide (which
covers the replica namespace). See
[namespace-watching.md](namespace-watching.md).

The gateway ServiceAccount is currently granted:

| Resource | Access |
|---|---|
| API discovery and version | read |
| Deployments | read, watch, create, update, patch, delete |
| ReplicaSets | read, watch |
| Pods | read, watch |
| Pod logs | read |
| Pod metrics (`metrics.k8s.io`) | read |
| Events | read, watch |
| Services | read, watch, create, update, patch, delete |
| Secrets | read, list, create, update, patch, delete |
| AgentEndpoints | read, watch, create, update, patch, delete |

Exec, attach, port-forward, proxy subresources, ServiceAccount token creation,
RBAC, impersonation, cluster-scoped resources, and other namespaces are denied
by the configured Role. Broadening that Role immediately broadens the remote
API surface without requiring a gateway change.

The poller labels every managed object with `ngrok-id=<replica-id>`. Compute
SHOULD establish list/watch streams using `resourceVersion` and MUST perform a
full relist after watch expiration or reconnection.

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
  remoteAccess:
    enabled: true
    # Defaults to a chart-created <operator namespace>-compute namespace.
    # A custom value must name an existing namespace.
    replicaNamespace: ""
```

Every key under `compute` is forwarded to the ngrok API as KubernetesOperator
metadata, so the gateway Deployment's own settings — `replicaCount`, `resources`,
`nodeSelector`, `tolerations`, `affinity`, `priorityClassName`, `podAnnotations`,
and `topologySpreadConstraints` — live under a separate `computeGateway` block:

```yaml
computeGateway:
  replicaCount: 1
  resources:
    requests:
      cpu: 100m
```

The gateway's Deployment, ServiceAccount, ConfigMap, Role, and RoleBinding are
all named `<release fullname>-gateway`, and the api-manager is pointed at them
with `--compute-gateway-name`.

`computeBaseURL` must address a server implementing this contract. Remote access
and the app-replica poller are mutually exclusive. Poller-mode installations
use `compute.poller.enabled: true`. The deprecated `compute.enabled` field maps
to poller mode only when remote access is disabled.
