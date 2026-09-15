# Authentication

## Overview

The ngrok-operator authenticates with ngrok using a single credential:

- **Personal Access Token** (`NGROK_PAT`): Used both for ngrok API access to manage resources (domains, endpoints, IP policies, etc.) and for ngrok agent authentication to establish tunnels.

Earlier versions used a separate API key and auth token. Those are no longer supported.

## Credential Storage

The credential is stored in a Kubernetes Secret in the operator's namespace. The Secret contains one key:

| Secret Key | Description                     |
|------------|---------------------------------|
| `PAT`      | ngrok personal access token     |

## Providing Credentials

### Via Helm Values (recommended for initial setup)

When installing via Helm, the token can be provided directly:

```yaml
credentials:
  pat: "<your-personal-access-token>"
```

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
  PAT: <base64-encoded-personal-access-token>
```

Then reference it in Helm values:

```yaml
credentials:
  secret:
    name: my-ngrok-credentials
```

When `credentials.pat` is empty, the Helm chart does not create a Secret and expects the named Secret to already exist.

## Credential Consumption

The two pods that need the credential mount it as `NGROK_PAT`:

- The **api-manager** (main controller) uses it for all ngrok API operations.
- The **agent-manager** uses it for establishing agent tunnels.

The **bindings-forwarder** does not receive it. Its data path authenticates with the mTLS client certificate described below. It becomes a credential consumer when its egress moves to private dial, which is PAT-only; the credential gets plumbed in as part of that work.

A single token with the permissions all three need is the simple path. Per-component tokens with narrower permissions are not wired up yet.

## mTLS for Bindings

When the bindings feature is enabled, the operator generates a self-signed TLS certificate and creates a Certificate Signing Request (CSR) with the ngrok API. This certificate is stored in a Secret (default name: `default-tls`) in the operator's namespace and is used for mTLS communication between the bindings forwarder and ngrok's ingress endpoint.

## One-Click Demo Mode

When `oneClickDemoMode: true` is set, the operator does not connect to the ngrok API or reconcile resources; it reports as Ready and waits. The api-manager pod still mounts `NGROK_PAT` from the Secret unconditionally, so the Secret and its `PAT` key must exist for the pod to start even in this mode.
