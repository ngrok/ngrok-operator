# 0.24 → 0.25 upgrade walkthrough kit

Throwaway kit for walking through `docs/upgrading-to-0.25.md` by hand on a
kind cluster. Install chart 0.24, apply the manifests here, then follow the
guide step by step and check that each claim holds.

Not a CI test. Delete this directory once 0.25 ships.

## What's here

| Path | What |
| --- | --- |
| `values-0.24.yaml` | 0.24 values layout, with a spread of keys that move in 0.25 |
| `values-0.25.yaml` | Answer key: the same values in the 0.25 layout. Translate them yourself from the guide first, then diff |
| `manifests/00-50` | User-created resources of every legacy kind, in each metadata shape |
| `manifests/60-80` | Ingress, LoadBalancer Service, and Gateway objects whose generated resources the operator migrates |
| `manifests/90-leftovers.yaml` | Fields 0.25 removes. The guide's "required before upgrading" queries should find these |
| `snapshot.sh` | Prints IDs, URLs, and Ready reasons for both API groups (`./snapshot.sh`), or the legacy keys still on objects (`./snapshot.sh legacy`). Diff it between steps |

## What each guide section exercises

| Guide section | Objects | Check |
| --- | --- | --- |
| Back up / credentials / Secret | Release installed with `credentials.apiKey` + `authtoken` | Render error text matches the guide when the old keys are left in; `--reuse-values` fails as described |
| Move values to new layout | `values-0.24.yaml` | Error lists each old key; your translation matches `values-0.25.yaml`; `podAnnotations` now lands on the agent pod too |
| `resolves_to` → `resolvesTo` | `Domain/leftover-resolves-to` | jq query finds it; after upgrade without fixing, field pruned |
| CloudEndpoint `trafficPolicyName` / `.policy` | `CloudEndpoint/leftover-tp-name`, `leftover-tp-policy` | jq query finds both; unfixed → `Ready=False`, last policy keeps serving |
| Legacy operator-written keys | Everything operator-managed, LB Services | `./snapshot.sh legacy` before (present) and after (gone after reconcile) |
| Upgrade CRDs first | — | Operator starts; new `*.ngrok.com` CRDs present |
| KubernetesOperator moves | Operator's own object | Same ID on both; legacy `SupersededByV1`; no drain |
| AgentEndpoints / CloudEndpoints move (generated) | `ing-default`, `ing-verbose`, `lb-tcp`, `lb-tls-verbose`, `route` | v1 twins with same names/IDs; legacy `SupersededByV1`; traffic never drops |
| Generated metadata must be strings | `ing-default` (`replicas: 3`) | Generated v1 AgentEndpoint has no metadata at all |
| Domain moves (operator-created) | Hostnames from Ingress/Gateway/Service/endpoints | v1 Domains with same ID/CNAME; legacy `SupersededByV1` |
| `spec.domain` immutable | any Domain | Edit is rejected with the guide's message |
| Rollback | All of the above | Everything keeps serving on 0.24; IDs unchanged; re-upgrade picks v1 objects back up |
| Migrate NgrokTrafficPolicy | `tp-add-header`, `tp-respond` | References from annotation, resource backend, targetRef, ExtensionRef all keep resolving mid-migration |
| Migrate Domain (user) | `user-domain-*` | jq works on map/string; fails on free text as described; `Retain` reservation survives |
| Migrate IPPolicy | `ipp-*` | Same ID after re-stamp; rule-level string metadata converted; free text fails |
| Migrate AgentEndpoint | `aep-public`, `aep-internal` | Endpoint keeps serving; `clep-forward` → `aep-internal` keeps working |
| Migrate CloudEndpoint | `clep-*` | Same ID; `replicas` dropped by jq on `clep-forward` |

## Setup

You need `kind`, `helm`, `kubectl`, `jq`, `envsubst` (from gettext), and an
ngrok account that can reserve domains under your suffix.

```bash
cd tests/upgrade-0.25

# 0.24 credentials
export NGROK_API_KEY=...
export NGROK_AUTHTOKEN=...
# 0.25 credential
export NGROK_ACCESS_TOKEN=...

# Every hostname is ${PREFIX}-<thing>${DOMAIN_SUFFIX}. Pick a prefix nobody
# else is using.
export PREFIX=$USER-upg
export DOMAIN_SUFFIX=.ngrok.app

export RELEASE=ngrok-operator NAMESPACE=ngrok-operator FULLNAME=ngrok-operator
```

### 1. Cluster and Gateway API CRDs

```bash
kind create cluster --name upgrade-test
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.5.1/standard-install.yaml
```

### 2. Install chart 0.24

```bash
helm repo add ngrok https://charts.ngrok.com 2>/dev/null; helm repo update ngrok

helm install "$RELEASE" ngrok/ngrok-operator \
  --namespace "$NAMESPACE" --create-namespace \
  --version 0.24.2 \
  --values values-0.24.yaml \
  --set credentials.apiKey="$NGROK_API_KEY" \
  --set credentials.authtoken="$NGROK_AUTHTOKEN" \
  --wait
```

### 3. Apply the manifests

`envsubst` only substitutes the two variables named, so `$` in traffic
policies is left alone.

```bash
for f in manifests/*.yaml; do
  envsubst '$PREFIX $DOMAIN_SUFFIX' < "$f" | kubectl apply -f -
done
```

Skip `80-gateway.yaml` if you don't care about Gateway. Skip
`90-leftovers.yaml` if you only want the happy path.

Give it a minute, then check everything is ready and record the baseline:

```bash
./snapshot.sh > 0-before.txt
./snapshot.sh legacy > 0-before-legacy.txt
cat 0-before.txt
```

Every legacy object should be `Ready=True` with an ID or URL. Fix anything
that isn't before you start: a broken baseline makes later diffs useless.

Quick traffic check:

```bash
curl -s https://$PREFIX-ing-default$DOMAIN_SUFFIX/ -D - -o /dev/null | grep -i x-upgrade-test
curl -s https://$PREFIX-ing-default$DOMAIN_SUFFIX/respond
curl -s https://$PREFIX-clep-inline$DOMAIN_SUFFIX/
curl -s https://$PREFIX-clep-targetref$DOMAIN_SUFFIX/
curl -s https://$PREFIX-clep-forward$DOMAIN_SUFFIX/ | head -3
curl -s https://$PREFIX-aep-public$DOMAIN_SUFFIX/ | head -3
curl -s https://$PREFIX-gw$DOMAIN_SUFFIX/ -D - -o /dev/null | grep -i x-upgrade-test
```

Optionally, run a loop in another terminal through the whole walkthrough
to catch traffic gaps:

```bash
while true; do
  printf '%s ' "$(date +%T)"
  for h in ing-default ing-verbose clep-forward aep-public gw; do
    printf '%s=%s ' "$h" "$(curl -s -o /dev/null -w '%{http_code}' --max-time 3 https://$PREFIX-$h$DOMAIN_SUFFIX/)"
  done
  echo
  sleep 1
done
```

## Walk the guide

Now follow `docs/upgrading-to-0.25.md` top to bottom, using the table above
to know which objects to look at. Snapshot after each phase and diff:

```bash
./snapshot.sh > 1-after-upgrade.txt && diff 0-before.txt 1-after-upgrade.txt
```

### Pointing the upgrade at a local build

Until an RC is published, the guide's `helm upgrade ngrok/ngrok-operator
--version ...` becomes a build from this branch:

```bash
# from the repo root
make docker-build _helm_setup
kind load docker-image ngrok-operator-local --name upgrade-test

helm upgrade "$RELEASE" ./helm/ngrok-operator \
  --namespace "$NAMESPACE" \
  --values tests/upgrade-0.25/values-0.25.yaml \
  --set ngrok.credentials.accessToken="$NGROK_ACCESS_TOKEN" \
  --set image.repository=ngrok-operator-local \
  --set image.tag=latest \
  --set image.pullPolicy=Never \
  --wait
```

Once `0.25.0-rc.1` is published, use the guide's command as written with
`--version 0.25.0-rc.1 --devel`.

### Rolling back

The guide rolls back with the backed-up values. Run it as written, then
snapshot and diff against `0-before.txt`. IDs should match.

## Edge cases worth poking at

These aren't covered by the manifests. Try them once you're on 0.25.

- **ID conflict**: apply a legacy and a v1 IPPolicy (or CloudEndpoint) with
  the same name in one `kubectl apply`, before the legacy one gets an ID.
  Expect `IPPolicyIDConflict` / `CloudEndpointIDConflict` events on both.
- **Delete order**: delete the v1 twin of a migrated Domain/IPPolicy/
  CloudEndpoint and check the legacy one takes the reservation back.
- **Legacy delete without twin**: delete `user-domain-map` (legacy) without
  re-stamping. The guide says the operator creates a default v1 Domain only
  if something still uses the hostname, so this one should just go away.
- **Uninstall after rollback**: the guide's "avoid this if you can" section.
  Do it last, since it's destructive.

## Teardown

```bash
helm uninstall "$RELEASE" -n "$NAMESPACE"
kind delete cluster --name upgrade-test
```

`drainPolicy: Delete` and `reclaimPolicy: Delete` clean up most ngrok-side
resources on uninstall. `user-domain-retain` is `Retain` on purpose: release
`$PREFIX-user-retain$DOMAIN_SUFFIX` in the dashboard. Check the dashboard for
anything else with your prefix.
