# Upgrade from Helm chart 0.24 to 0.25

This guide covers upgrading an existing ngrok Kubernetes Operator installation
from Helm chart `0.24.x` to `0.25.x`.

The 0.25 chart replaces the operator's two ngrok credentials with a single
[personal access token](https://dashboard.ngrok.com/settings/access-tokens).
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

### Create a personal access token

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
and set `credentials.pat` instead:

```yaml
credentials:
  pat: "<your-personal-access-token>"
```

If you pass credentials on the command line, replace both `--set` flags with
one:

```bash
helm upgrade "$RELEASE" ngrok/ngrok-operator \
  --set credentials.pat="$NGROK_PAT"
```

Passing either removed value now fails the render:

```
Error: execution error at (ngrok-operator/templates/credentials-secret.yaml):
credentials.apiKey and credentials.authtoken have been replaced by a single
credentials.pat (an ngrok personal access token).
```

This includes values carried forward implicitly. If you upgrade with
`--reuse-values`, the old keys come along from the previous release and the
upgrade fails until you remove them.

### Update a pre-existing Secret

If you manage the credentials Secret yourself and point at it with
`credentials.secret.name`, change its keys. The chart now reads a single key:

| Before | After |
| --- | --- |
| `API_KEY` | `PAT` |
| `AUTHTOKEN` | `PAT` |

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-ngrok-credentials
  namespace: ngrok-operator
type: Opaque
data:
  PAT: <base64-encoded-personal-access-token>
```

All three operator components read this one key, so a Secret that still carries
only `API_KEY` and `AUTHTOKEN` leaves every pod unable to start. Update the
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
unable to verify ngrok personal access token: HTTP 403: ... [ERR_NGROK_203]
```

A `ngrok API read failed` line names the resource whose permissions the token is
missing. These checks are reads only: a token with read but not write access
logs `read ok` for every resource and still fails when the operator reconciles.

## Rotating the token

Set the new value and upgrade. All three deployments carry a checksum of the
credentials Secret, so every pod that holds the token restarts and picks up the
new value.

```bash
helm upgrade "$RELEASE" ngrok/ngrok-operator \
  --namespace "$NAMESPACE" \
  --reuse-values \
  --set credentials.pat="$NEW_NGROK_PAT"
```

Revoke the old token in the dashboard once the rollout completes.

## Rolling back

Rolling back to 0.24 requires the credentials it understands. `helm rollback`
restores the previous release's values, including `credentials.apiKey` and
`credentials.authtoken`, so keep those credentials valid until the rollback
window has closed. Do not revoke the old API key and authtoken until you are
committed to 0.25.
