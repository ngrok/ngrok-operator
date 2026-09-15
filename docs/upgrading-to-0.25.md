# Upgrade from Helm chart 0.24 to 0.25

This guide covers upgrading an existing ngrok Kubernetes Operator installation
from Helm chart `0.24.x` to `0.25.x`.

The 0.25 chart replaces the operator's two ngrok credentials with a single
[access token](https://dashboard.ngrok.com/settings/access-tokens).
This is a breaking change: `credentials.apiKey` and `credentials.authtoken` are
removed, and an upgrade that still passes them fails with an explicit error
rather than starting a release with no credentials.

## Before upgrading

### Back up the release configuration

Set these to match your installation:

```bash
RELEASE=ngrok-operator
NAMESPACE=ngrok-operator

helm get values "$RELEASE" --namespace "$NAMESPACE" --all \
  > ngrok-operator-0.24-values.yaml
helm get manifest "$RELEASE" --namespace "$NAMESPACE" \
  > ngrok-operator-0.24-manifest.yaml
```

Keep these backups until the upgrade and its rollback window have closed. Do
not use the output of `helm get values --all` as a new long-term values file;
it contains defaults from the old chart.

### Create an access token

One token replaces both of today's credentials. It authenticates the agent
connection and the ngrok API, so it needs the permissions the operator uses for
both: starting tunnels, and reading and writing endpoints, domains, reserved TCP
addresses, IP policies, and the Kubernetes operator registration.

Create the token at
[dashboard.ngrok.com/settings/access-tokens](https://dashboard.ngrok.com/settings/access-tokens).
A token inherits the permissions of the account membership that created it, so
create it from a membership that can perform the operations above.

The token is shown once. Store it the way you store the credentials it replaces.

### Replace the credential values

Remove `credentials.apiKey` and `credentials.authtoken` from your values file
and set `credentials.accessToken` instead:

```yaml
credentials:
  accessToken: "<your-access-token>"
```

If you pass credentials on the command line, replace both `--set` flags with
one:

```bash
helm upgrade "$RELEASE" ngrok/ngrok-operator \
  --set credentials.accessToken="$NGROK_ACCESS_TOKEN"
```

Passing either removed value now fails the render:

```
Error: execution error at (ngrok-operator/templates/credentials-secret.yaml):
credentials.apiKey and credentials.authtoken have been replaced by a single
credentials.accessToken (an ngrok access token).
```

This includes values carried forward implicitly. If you upgrade with
`--reuse-values`, the old keys come along from the previous release and the
upgrade fails until you remove them.

### Update a pre-existing Secret

If you manage the credentials Secret yourself and point at it with
`credentials.secret.name`, change its keys. The chart now reads a single key:

| Before | After |
| --- | --- |
| `API_KEY` | `API_MANAGER_ACCESS_TOKEN` |
| `AUTHTOKEN` | `AGENT_ACCESS_TOKEN` |

Both keys may hold the same token. They are separate so that a deployment
giving each component its own narrowly-permissioned token can rotate one
without touching the other.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-ngrok-credentials
  namespace: ngrok-operator
type: Opaque
data:
  AGENT_ACCESS_TOKEN: <base64-encoded-access-token>
  API_MANAGER_ACCESS_TOKEN: <base64-encoded-access-token>
```

A Secret that still carries only `API_KEY` and `AUTHTOKEN` leaves both pods
unable to start. Update the
Secret before or together with the chart upgrade.

## Upgrade

Use the values file you normally manage for this release:

```bash
TARGET_VERSION=0.25.0

helm repo update ngrok

helm upgrade "$RELEASE" ngrok/ngrok-operator \
  --namespace "$NAMESPACE" \
  --version "$TARGET_VERSION" \
  --values path/to/your-values.yaml \
  --wait
```

## Verify the upgrade

Confirm that the workloads rolled out:

```bash
kubectl get deployments,pods --namespace "$NAMESPACE"
```

The api-manager logs one line per ngrok API resource it can read at startup:

```
ngrok API read ok {"resource": "endpoints"}
ngrok API read ok {"resource": "domains"}
ngrok API read ok {"resource": "tcp-addrs"}
ngrok API read ok {"resource": "ip-policies"}
```

A token the operator cannot use at all stops startup with a single clear error:

```
unable to verify ngrok access token: HTTP 403: ... [ERR_NGROK_203]
```

A `ngrok API read failed` line names the resource whose permissions the token is
missing. These checks are reads only: a token with read but not write access
logs `read ok` for every resource and still fails when the operator reconciles.

## Optional: one token per component

The operator runs its credentials in two pods split by capability. The
agent-manager establishes tunnel sessions and makes no API calls; the
api-manager reconciles resources against the ngrok API and never starts
tunnels. A single token collapses that boundary — the agent-manager pod ends up
holding a credential that can also reach the management API.

To keep the split, create two tokens and set them per component instead of
setting `credentials.accessToken`:

```yaml
credentials:
  agent:
    accessToken: "<token that can start tunnels>"
  apiManager:
    accessToken: "<token with ngrok API permissions>"
```

Either may be set on its own alongside `credentials.accessToken`, which the
other falls back to. Setting one without a fallback for the other fails the
render rather than leaving a pod unable to start.

## Clean up the old Secret keys

Helm does not remove keys from a Secret it already manages, so after upgrading,
the Secret still carries the `API_KEY` and `AUTHTOKEN` values from 0.24. Nothing
reads them, but they remain readable in the cluster. Confirm what is there:

```bash
kubectl get secret "$RELEASE-credentials" --namespace "$NAMESPACE" \
  -o go-template='{{range $k, $v := .data}}{{$k}}{{"\n"}}{{end}}'
```

Once the rollout is healthy, remove the stale keys and revoke the credentials
themselves in the dashboard:

```bash
kubectl patch secret "$RELEASE-credentials" --namespace "$NAMESPACE" \
  --type=json \
  -p='[{"op":"remove","path":"/data/API_KEY"},{"op":"remove","path":"/data/AUTHTOKEN"}]'
```

Do this only after you are committed to 0.25 — see rolling back, below.

## Rotating the token

Set the new value and upgrade. Both deployments that hold the token carry a checksum of the credentials
Secret, so each restarts and picks up the new value.

```bash
helm upgrade "$RELEASE" ngrok/ngrok-operator \
  --namespace "$NAMESPACE" \
  --reuse-values \
  --set credentials.accessToken="$NEW_NGROK_ACCESS_TOKEN"
```

Revoke the old token in the dashboard once the rollout completes.

## Rolling back

Rolling back to 0.24 requires the credentials it understands. `helm rollback`
restores the previous release's values, including `credentials.apiKey` and
`credentials.authtoken`, so keep those credentials valid until the rollback
window has closed. Do not revoke the old API key and authtoken until you are
committed to 0.25.
