# Authentication

## Overview

The ngrok-operator authenticates with the ngrok API using two credentials:

- **API Key** (`NGROK_API_KEY`): Used for ngrok API access to manage resources (domains, endpoints, IP policies, etc.)
- **Auth Token** (`NGROK_AUTHTOKEN`): Used for ngrok agent authentication to establish tunnels

Both credentials are required for the operator to function.

## Credential Storage

Credentials are stored in a Kubernetes Secret in the operator's namespace. The Secret contains two keys:

| Secret Key   | Description                        |
|--------------|------------------------------------|
| `API_KEY`    | ngrok API key                      |
| `AUTHTOKEN`  | ngrok auth token                   |

## Providing Credentials

### Via Helm Values (recommended for initial setup)

When installing via Helm, credentials can be provided directly:

```yaml
credentials:
  apiKey: "<your-api-key>"
  authtoken: "<your-authtoken>"
```

When both values are provided, the Helm chart creates a Secret with the generated name `<release-name>-ngrok-operator-credentials` (or the name specified in `credentials.secret.name`).

### Via Pre-existing Secret

If credentials are managed externally (e.g., by a secrets manager), create the Secret before installing the operator:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-ngrok-credentials
  namespace: <operator-namespace>
type: Opaque
data:
  API_KEY: <base64-encoded-api-key>
  AUTHTOKEN: <base64-encoded-authtoken>
```

Then reference it in Helm values:

```yaml
credentials:
  secret:
    name: my-ngrok-credentials
```

When `credentials.apiKey` and `credentials.authtoken` are both empty, the Helm chart does not create a Secret and expects the named Secret to already exist.

## Credential Consumption

The operator pods mount the Secret as environment variables:

- The **api-manager** (main controller) uses `NGROK_API_KEY` for all ngrok API operations.
- The **agent-manager** uses `NGROK_AUTHTOKEN` for establishing agent tunnels.
- The **bindings-forwarder** uses `NGROK_AUTHTOKEN` for its connections.

## mTLS for Bindings

When the bindings feature is enabled, the operator generates a self-signed TLS certificate and creates a Certificate Signing Request (CSR) with the ngrok API. This certificate is stored in a Secret (default name: `default-tls`) in the operator's namespace and is used for mTLS communication between the bindings forwarder and ngrok's ingress endpoint.

## One-Click Demo Mode

When `oneClickDemoMode: true` is set, the operator starts without requiring credentials. It will report as Ready but will not actually connect to the ngrok API. This mode is intended for demonstration purposes only.
