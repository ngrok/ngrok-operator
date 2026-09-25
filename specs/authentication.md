# Authentication

## Overview

The ngrok-operator authenticates with ngrok using a single credential:

- **Access Token** (`NGROK_ACCESS_TOKEN`): Used both for ngrok API access to manage resources (domains, endpoints, IP policies, etc.) and for ngrok agent authentication to establish tunnels.

Earlier versions used a separate API key and auth token. Those are no longer supported.

## Credential Storage

The credential is stored in a Kubernetes Secret in the operator's namespace. Each component reads its own key, so that one component's token can be rotated without touching the other:

| Secret Key | Read by | Description |
|---|---|---|
| `AGENT_ACCESS_TOKEN` | agent-manager | ngrok access token for the tunnel session |
| `API_MANAGER_ACCESS_TOKEN` | api-manager | ngrok access token for the ngrok API |

Both keys hold the same value unless a per-component token is configured.

## Providing Credentials

### Via Helm Values (recommended for initial setup)

When installing via Helm, the token can be provided directly:

```yaml
credentials:
  accessToken: "<your-access-token>"
```

One token for every component is the simple path. A token can also be set per
component, each falling back to `credentials.accessToken` when empty:

```yaml
credentials:
  agent:
    accessToken: "<agent-manager token>"
  apiManager:
    accessToken: "<api-manager token>"
```

The split exists so that each component can be given a token carrying only the
permissions it needs — the agent-manager only establishes tunnel sessions, and
the api-manager only calls the ngrok API. That is not yet possible: an access
token carries the full permissions of the account membership that created it,
so today both tokens must be equally privileged and the split buys nothing.
Wire it up now and narrowing becomes a values change later.

Setting a token for one component but not the other fails the render, rather
than leaving the other pod unable to start.

When the value is provided, the Helm chart creates a Secret with the generated name `<release-name>-ngrok-operator-credentials` (or the name specified in `credentials.secret.name`).

### Via Pre-existing Secret

If the token is managed externally (e.g., by a secrets manager), create the Secret before installing the operator:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-ngrok-credentials
  namespace: <operator-namespace>
type: Opaque
data:
  AGENT_ACCESS_TOKEN: <base64-encoded-access-token>
  API_MANAGER_ACCESS_TOKEN: <base64-encoded-access-token>
```

Both keys are required, and may hold the same token.

Then reference it in Helm values:

```yaml
credentials:
  secret:
    name: my-ngrok-credentials
```

When `credentials.accessToken` is empty, the Helm chart does not create a Secret and expects the named Secret to already exist.

## Credential Consumption

The two pods that need the credential mount their own key as `NGROK_ACCESS_TOKEN`:

- The **api-manager** (main controller) uses it for all ngrok API operations.
- The **agent-manager** uses it for establishing agent tunnels.

The **bindings-forwarder** does not receive it. Its data path authenticates with the mTLS client certificate described below. It becomes a credential consumer when its egress moves to private dial, which is access token-only; the credential gets plumbed in as part of that work.


## mTLS for Bindings

When the bindings feature is enabled, the operator generates a self-signed TLS certificate and creates a Certificate Signing Request (CSR) with the ngrok API. This certificate is stored in a Secret (default name: `default-tls`) in the operator's namespace and is used for mTLS communication between the bindings forwarder and ngrok's ingress endpoint.

## One-Click Demo Mode

When `oneClickDemoMode: true` is set, the operator does not connect to the ngrok API or reconcile resources; it reports as Ready and waits. The api-manager pod does not mount the credential in this mode, so no Secret is required for it to start.
