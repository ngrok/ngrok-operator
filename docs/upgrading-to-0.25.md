# Upgrade from Helm chart 0.24 to 0.25

This guide covers upgrading an existing ngrok Kubernetes Operator installation
from Helm chart `0.24.x` to `0.25.x`.

The 0.25 chart replaces the operator's two ngrok credentials with a single
[access token](https://dashboard.ngrok.com/access-tokens).
This is a breaking change: `credentials.apiKey` and `credentials.authtoken` are
removed, and an upgrade that still passes them fails with an explicit error
rather than starting a release with no credentials.

One token can serve both components, or you can set a separate token for each.
Setting one per component prepares for scoped access tokens, which will let each
token carry only the permissions its component needs. See
[one token per component](#optional-one-token-per-component).

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
[dashboard.ngrok.com/access-tokens](https://dashboard.ngrok.com/access-tokens).
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
Error: UPGRADE FAILED: execution error at (ngrok-operator/templates/api-manager/deployment.yaml:30:28):
credentials.apiKey and credentials.authtoken have been replaced by a single
credentials.accessToken (an ngrok access token).
```

The error names the api-manager deployment because it is the first template to
pull in the credentials Secret, for its checksum annotation. The removed values
are what it is complaining about.

This includes values carried forward implicitly. If you upgrade with
`--reuse-values`, the old keys come along from the previous release and the
upgrade fails until you remove them.

### Update a pre-existing Secret

If you manage the credentials Secret yourself and point at it with
`credentials.secret.name`, change its keys. The chart now reads one key per
component:

| Before | After |
| --- | --- |
| `API_KEY` | `API_MANAGER_ACCESS_TOKEN` |
| `AUTHTOKEN` | `AGENT_ACCESS_TOKEN` |

Both keys hold the same token unless you set one per component, below.

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

At startup the api-manager verifies the token by listing endpoints. A token it
cannot use stops startup with an error:

```
Unable to verify access token: HTTP 403: ... [ERR_NGROK_203]
```

The agent-manager has no equivalent check, so confirm that it established its
tunnel session:

```bash
kubectl logs --namespace "$NAMESPACE" deploy/"$RELEASE"-agent | grep heartbeat
```

```
drivers.agent  ngrok agent heartbeat received  {"latency": "19.225797ms"}
```

## Optional: one token per component

The operator runs its credentials in two pods split by capability. The
agent-manager establishes tunnel sessions and makes no API calls; the
api-manager reconciles resources against the ngrok API and never starts
tunnels.

The chart can take a token per component so that each one can eventually carry
only the permissions it needs. That narrowing is not available yet: an access
token carries the full permissions of the account membership that created it,
so two tokens set this way are just as privileged as one. Setting them now
means narrowing later is a values change rather than a migration.

To set them, use these instead of `credentials.accessToken`:

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

Both deployments annotate a checksum over the whole rendered Secret, so
changing either token restarts both pods.

## The old Secret keys

If the chart manages the Secret, Helm removes `API_KEY` and `AUTHTOKEN` as part
of the upgrade — they are gone from the live Secret once it completes, with no
cleanup step needed. Confirm:

```bash
kubectl get secret "$RELEASE-ngrok-operator-credentials" --namespace "$NAMESPACE" \
  -o go-template='{{range $k, $v := .data}}{{$k}}{{"\n"}}{{end}}'
```

```
AGENT_ACCESS_TOKEN
API_MANAGER_ACCESS_TOKEN
```

If you manage the Secret yourself, remove the two old keys when you add the new
ones.

Revoking the old API key and authtoken in the dashboard is a separate step, and
one to take only after you are committed to 0.25 — see rolling back, below.

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

This works because the checksum is taken over the Secret the chart renders. If
you manage the Secret yourself with `credentials.secret.name`, the chart renders
nothing, the checksum never changes, and neither pod restarts — the token is
read from the environment once at startup, so both keep using the old value.
Restart them yourself after rotating, before revoking the old token:

```bash
kubectl rollout restart --namespace "$NAMESPACE" \
  deploy/"$RELEASE" deploy/"$RELEASE"-agent
```

## Rolling back

Rolling back to 0.24 requires the credentials it understands. `helm rollback`
restores the previous release's values, including `credentials.apiKey` and
`credentials.authtoken`, so keep those credentials valid until the rollback
window has closed. Do not revoke the old API key and authtoken until you are
committed to 0.25.
