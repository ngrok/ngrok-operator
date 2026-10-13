# Runbook: private endpoints e2e on kind

Full rerun of the rev-2 e2e ([findings.md](../findings.md) "E2E, revision 2") plus the new checks
from T1/T2/T3/T6. Manual; ~60 min. Paste results into a new findings.md section
`## E2E rerun — <date>` using the check IDs below (B1…, A…F, I1…).

The main path tests **defaults (D8)**: only `.ngrok.direct` is mirrored and rewritten, every
scheme runs on `.ngrok.direct`, and one `.internal` endpoint is started as a negative control
that must *not* be mirrored or resolved. §4b turns `.internal` on and reruns the `.internal`
checks.

Checks for fixed bugs. If a run shows the "before fix" behavior, the fix regressed:

| Check | Bug / task | Before fix |
|---|---|---|
| A ServiceConflict | R1 / T1 | Unowned Service is overwritten |
| B port reuse | R2 / T2, D5 | New endpoint gets the freed port at once |
| C spec drift | R3 / T3, R19 | Edit sticks; not restored |

## 0. Prereqs and variables

kind, kubectl, helm, docker, ngrok agent (3.39+), openssl, python3, a PAT.

```sh
export NGROK_ACCESS_TOKEN=<PAT>          # operator
export NGROK_AUTHTOKEN=$NGROK_ACCESS_TOKEN  # agents (findings used the same PAT)
export KIND_CLUSTER_NAME=pe-poc
export NS=ngrok-operator
export U=$(openssl rand -hex 3)          # unique suffix; labels must start with a letter, "pe-" covers it
echo "U=$U"                              # record it

pes() { kubectl -n $NS get privateendpoints -o custom-columns='NAME:.metadata.name,URL:.spec.url,PORT:.spec.port,IP:.status.clusterIP,FWD:.status.forwarderPort,READY:.status.conditions[?(@.type=="Ready")].status,REASON:.status.conditions[?(@.type=="Ready")].reason'; }
nx() { kubectl exec nettest -- sh -c "$*"; }   # run in the netshoot pod (netshoot is Alpine/musl)
```

Several steps use extra terminals: export the same variables (especially `U`) in each.

## 1. Fresh cluster, deploy, DNS

```sh
kind delete cluster --name $KIND_CLUSTER_NAME 2>/dev/null
kind create cluster --name $KIND_CLUSTER_NAME
make deploy_with_private_endpoints
make kind-private-endpoints-dns
make kind-private-endpoints-dns          # second run must be a no-op
kubectl -n kube-system get cm coredns -o jsonpath='{.data.Corefile}' | grep -c 'ngrok-private-endpoints begin'
kubectl -n kube-system get cm coredns -o jsonpath='{.data.Corefile}' | grep -c -- '-internal\.'
kubectl -n $NS get deploy ngrok-operator-manager -o jsonpath='{range .spec.template.spec.containers[0].env[*]}{.name}={.value}{"\n"}{end}' | grep INCLUDE_INTERNAL
kubectl -n $NS get pods
```

Expected: `1` (exactly one managed block); `0` (no `.internal` rewrite by default);
`NGROK_OPERATOR__FEATURES__PRIVATE_ENDPOINTS__INCLUDE_INTERNAL=false`; api-manager, agent, and
`*-private-endpoint-forwarder` pods Running.

Record versions:

```sh
kind version; kubectl version | grep Server
kubectl -n kube-system get deploy coredns -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
ngrok version; git rev-parse --short HEAD
```

## 2. Start endpoints outside the cluster

One terminal (or tmux pane) each. Keep them running unless a step says stop.

```sh
# backends
python3 -m http.server 18080
docker run --rm -p 16379:6379 redis:7
openssl req -x509 -newkey rsa:2048 -nodes -keyout /tmp/pe-tls.key -out /tmp/pe-tls.crt -days 1 -subj "/CN=pe-tls-$U.ngrok.direct"
openssl s_server -accept 19443 -cert /tmp/pe-tls.crt -key /tmp/pe-tls.key -www

# agents: every scheme on .ngrok.direct
ngrok http 18080 --url http://pe-web-$U.ngrok.direct
ngrok http 18080 --url https://pe-web-$U.ngrok.direct
ngrok tls 19443 --url tls://pe-tls-$U.ngrok.direct
ngrok tcp 16379 --url tcp://pe-redis-$U.ngrok.direct:6379
# negative control: must NOT be mirrored by default (also used in §4b)
ngrok http 18080 --url http://pe-web-$U.internal
```

```sh
sleep 15; pes; kubectl -n $NS get svc -l ngrok.com/managed-by=private-endpoint-poller
```

Expected: 4 CRs for this `U`, all `READY=True`, FWD ports unique in 20000–20999. Services:
`pe-web-$U-ngrok-direct` (80,443), `pe-tls-$U-ngrok-direct` (443), `pe-redis-$U-ngrok-direct`
(6379). Both `pe-web-…ngrok.direct` CRs share one IP. tls URL shows `:443`. **No** CR for
`http://pe-web-$U.internal` and no `pe-web-$U-internal` Service. VERIFY: `tls://` and `tcp://`
URLs are accepted on `.ngrok.direct` (the D8 demo assumes so; not yet run).

## 3. Baseline checks (netshoot)

```sh
kubectl run nettest --image=nicolaka/netshoot --restart=Never -- sleep infinity
kubectl wait --for=condition=Ready pod/nettest
```

```sh
echo B1;  nx "curl -s -o /dev/null -w '%{http_code}\n' http://pe-web-$U.ngrok.direct"
echo B2;  nx "curl -s -o /dev/null -w '%{http_code} ssl_verify=%{ssl_verify_result}\n' https://pe-web-$U.ngrok.direct"
echo B3;  nx "echo | openssl s_client -connect pe-tls-$U.ngrok.direct:443 -servername pe-tls-$U.ngrok.direct 2>/dev/null | grep subject="
echo B4;  nx "printf 'PING\r\n' | nc -w 3 pe-redis-$U.ngrok.direct 6379"
echo B5;  nx "dig +noall +answer pe-web-$U.ngrok.direct"
echo B6;  nx "dig +noall +answer PE-WEB-$U.Ngrok.Direct"
echo B7;  nx "dig pe-web-$U.ngrok.direct AAAA | grep -E 'status|ANSWER:'"
echo B8;  nx "dig nope-$U.ngrok.direct | grep status"
echo B9;  nx "dig metadata.google.internal | grep status"
echo B10; nx "dig +short kubernetes.default.svc.cluster.local"
echo B11; kubectl -n $NS get privateendpoints -o jsonpath='{range .items[*]}{.spec.url}{"\n"}{end}' | grep -c "pe-web-$U.internal"; \
          kubectl -n $NS get svc pe-web-$U-internal 2>&1 | head -1
echo B12; nx "dig pe-web-$U.internal | grep status; curl -s -m 5 -o /dev/null -w '%{http_code}\n' http://pe-web-$U.internal"
```

| ID | Expected |
|---|---|
| B1 | `200` |
| B2 | `200 ssl_verify=0` (public cert, verification on) |
| B3 | subject contains `CN=pe-tls-$U.ngrok.direct` (origin cert, end-to-end) |
| B4 | `+PONG` |
| B5 | `pe-web-$U.ngrok.direct. <ttl> IN A <IP of pe-web-$U-ngrok-direct>` (queried name, not Service name) |
| B6 | same IP as B5 |
| B7 | `NOERROR`, `ANSWER: 0` (not NXDOMAIN/SERVFAIL — matters for musl) |
| B8 | `NXDOMAIN` |
| B9 | `NXDOMAIN` from upstream, not SERVFAIL (multi-label not rewritten) |
| B10 | `10.96.0.1` |
| B11 | `0`, then `NotFound` (D8: `.internal` not mirrored) |
| B12 | `NXDOMAIN` from upstream (no `.internal` rewrite), curl `000` (D8: `.internal` not resolved) |

(B9 stronger variant, as in findings: temporarily add `hosts { 169.254.169.254 metadata.google.internal fallthrough }`
after the rewrites; expect that IP. Remove afterwards.)

## 4. New checks

### D. Resolver behavior: musl vs glibc, app-style lookups (T6)

```sh
kubectl run pe-musl  --image=alpine:3 --restart=Never -- sleep infinity
kubectl run pe-glibc --image=debian:stable-slim --restart=Never -- sleep infinity
kubectl wait --for=condition=Ready pod/pe-musl pod/pe-glibc
kubectl exec pe-musl  -- apk add --no-cache curl >/dev/null
kubectl exec pe-glibc -- sh -c 'apt-get update -qq && apt-get install -y -qq curl >/dev/null'
```

For `P` in `pe-musl pe-glibc`:

```sh
for P in pe-musl pe-glibc; do echo "== $P"
  kubectl exec $P -- cat /etc/resolv.conf
  kubectl exec $P -- sh -c "time getent hosts pe-web-$U.ngrok.direct"
  kubectl exec $P -- getent hosts pe-web-$U.ngrok.direct.
  kubectl exec $P -- getent hosts nope-$U.ngrok.direct; echo "exit=$?"
  kubectl exec $P -- curl -s -o /dev/null -w '%{http_code}\n' http://pe-web-$U.ngrok.direct
  kubectl exec $P -- curl -s -o /dev/null -w '%{http_code} ssl_verify=%{ssl_verify_result}\n' https://pe-web-$U.ngrok.direct
done
```

Expected, both images: resolv.conf has `search <ns>.svc.cluster.local svc.cluster.local cluster.local`
and `ndots:5`; `getent` prints the Service IP for relative and absolute names; `nope` → no
output, exit 2; curl `200` / `200 ssl_verify=0`. Record the `time` (search-list expansion
cost: three NXDOMAINs before the bare name). Any difference between musl and glibc is the
finding. VERIFY: `getent` ships in base `alpine:3` (musl-utils); if not, use curl only.

Optional (T6 mentions Go): a pure-Go resolver lookup (`net.LookupHost`) from a `golang` image.
Not scripted here.

### E. Negative cache on create (T6)

```sh
H=pe-late-$U
nx "dig $H.ngrok.direct | grep -E 'status|SOA'"     # NXDOMAIN; note SOA TTL (negative TTL)
kubectl exec pe-glibc -- getent hosts $H.ngrok.direct; echo "exit=$?"   # prime client path too
# terminal 1: poll from the pod with timestamps
kubectl exec pe-glibc -- sh -c "until getent hosts $H.ngrok.direct; do sleep 1; done; date -u +%T"
# terminal 2, right away:
date -u +%T; ngrok http 18080 --url http://$H.ngrok.direct
# terminal 3:
kubectl -n $NS get privateendpoints -w --output-watch-events
```

Record: agent start time, CR ADDED time, Ready time, resolve time. Expected: resolves within
~poll (≤10s) + negative TTL (kind CoreDNS `cache 30`, SOA min TTL from the dig above; expect
≤~20s total). If it's ≥30s, the CoreDNS denial cache is the floor; note it. Stop the agent after.

### C. Spec drift is healed (R3/T3)

```sh
PE=$(kubectl -n $NS get privateendpoints -o jsonpath="{.items[?(@.spec.url=='http://pe-web-$U.ngrok.direct')].metadata.name}")
kubectl -n $NS patch privateendpoint $PE --type merge -p '{"spec":{"port":8081}}'; date -u +%T
for i in $(seq 1 30); do echo "$(date -u +%T) $(kubectl -n $NS get privateendpoint $PE -o jsonpath='{.spec.port}') $(kubectl -n $NS get svc pe-web-$U-ngrok-direct -o jsonpath='{.spec.ports[*].port}')"; sleep 1; done
nx "curl -s -m 5 -o /dev/null -w '%{http_code}\n' http://pe-web-$U.ngrok.direct"
```

Expected: spec.port back to `80` within ~10s, Service ports `80 443`, curl `200`. If it stays
`8081`, check the api-manager log for `forbidden` (R19: Role lacks `update`), then restore by
hand: `kubectl -n $NS delete privateendpoint $PE` (poller recreates it next tick).

### B. Port reuse after delete (R2/T2) + cleanup timing

```sh
pes   # note FWD for pe-redis-$U.ngrok.direct  → OLD_FWD
# stop the `ngrok tcp ... pe-redis-$U` agent; note time
for i in $(seq 1 20); do echo "$(date -u +%T) $(kubectl -n $NS get svc pe-redis-$U-ngrok-direct --no-headers 2>&1 | head -c 60)"; sleep 1; done
nx "dig pe-redis-$U.ngrok.direct | grep status"
ngrok tcp 16379 --url tcp://pe-redis2-$U.ngrok.direct:6379
sleep 15; pes   # FWD for pe-redis2-$U.ngrok.direct → NEW_FWD
nx "printf 'PING\r\n' | nc -w 3 pe-redis2-$U.ngrok.direct 6379"
```

Expected: CR + `pe-redis-$U-ngrok-direct` Service gone within one poll (~10s); dig `NXDOMAIN`
after ≤5s TTL; new endpoint `+PONG`; `NEW_FWD != OLD_FWD` (cursor allocation). If equal, the
cursor regressed. This doesn't test the lagging-replica case (D5), which needs two forwarders
and a paused watch.

### A. Pre-existing unrelated Service is not overwritten (R1/T1)

```sh
kubectl -n $NS create service clusterip pe-clash-$U-ngrok-direct --tcp=80:8080
kubectl -n $NS get svc pe-clash-$U-ngrok-direct -o jsonpath='{.metadata.labels}{"\n"}{.spec.selector}{"\n"}{.spec.ports}{"\n"}' | tee /tmp/clash.before
ngrok http 18080 --url http://pe-clash-$U.ngrok.direct
sleep 15; pes | grep -E 'NAME|clash'
kubectl -n $NS get svc pe-clash-$U-ngrok-direct -o jsonpath='{.metadata.labels}{"\n"}{.spec.selector}{"\n"}{.spec.ports}{"\n"}' | diff /tmp/clash.before - && echo UNCHANGED
pes   # other endpoints still Ready
```

Expected: CR `READY=False REASON=ServiceConflict`; `UNCHANGED`; other CRs still Ready. If the
Service gained `ngrok.com/managed-by` labels, a forwarder selector or targetPort 2000x, R1
regressed.

Then stop the `pe-clash` agent and check the unowned Service still exists
(`kubectl -n $NS get svc pe-clash-$U-ngrok-direct`) — operator must not delete what it doesn't
own. Delete it yourself afterwards.

## 4b. Opt-in `.internal` (D8)

Turn `.internal` on in both places (operator and DNS), rerun the `.internal` checks, then turn
it off again. Keep the §2 agents running, including `http://pe-web-$U.internal`.

```sh
helm upgrade ngrok-operator helm/ngrok-operator -n $NS --reuse-values --set ngrok.features.privateEndpoints.includeInternal=true   # from the repo root
PE_INCLUDE_INTERNAL=true make kind-private-endpoints-dns
kubectl -n kube-system get cm coredns -o jsonpath='{.data.Corefile}' | grep -c -- '-internal\.'
# extra .internal agents
ngrok tls 19443 --url tls://pe-tls-$U.internal
ngrok tcp 16379 --url tcp://pe-redis-$U.internal:6379
sleep 15; pes | grep -E "NAME|\.internal"
```

`pe-tls-$U.internal` gets the origin cert for `CN=pe-tls-$U.ngrok.direct`; I3 only checks
that TLS passes through, so a name mismatch is expected.

```sh
echo I1; nx "curl -s -o /dev/null -w '%{http_code}\n' http://pe-web-$U.internal"
echo I2; nx "printf 'PING\r\n' | nc -w 3 pe-redis-$U.internal 6379"
echo I3; nx "echo | openssl s_client -connect pe-tls-$U.internal:443 -servername pe-tls-$U.internal 2>/dev/null | grep subject="
echo I4; nx "dig +noall +answer pe-web-$U.internal; dig +noall +answer PE-WEB-$U.Internal"
echo I5; nx "dig nope-$U.internal | grep status"
echo I6; nx "dig metadata.google.internal | grep status"
echo I7; nx "dig internal. SOA | grep status; dig foo.internal SOA | grep status"
```

| ID | Expected |
|---|---|
| (setup) | Corefile count `1`; CRs for the three `.internal` URLs, `READY=True` |
| I1 | `200` |
| I2 | `+PONG` |
| I3 | `subject=CN=pe-tls-$U.ngrok.direct` (origin cert passed through) |
| I4 | queried name in the answer, same IP both lines, IP of `pe-web-$U-internal` |
| I5 | `NXDOMAIN` (captured by the rewrite: we own single-label `.internal`) |
| I6 | upstream `NXDOMAIN`, not SERVFAIL (multi-label not rewritten) |
| I7 | `internal.` upstream as before; `foo.internal` `NXDOMAIN` from the rewrite. The real apex capture (R15: `ec2.internal`, `compute.internal`, `google.internal`) is measured on EKS/k3s |

Turn it off again and check the cleanup:

```sh
helm upgrade ngrok-operator helm/ngrok-operator -n $NS --reuse-values --set ngrok.features.privateEndpoints.includeInternal=false
make kind-private-endpoints-dns
sleep 15; pes | grep -cE "\.internal"; kubectl -n $NS get svc -l ngrok.com/managed-by=private-endpoint-poller | grep -c -- '-internal '
nx "dig pe-web-$U.internal | grep status"
```

Expected: `0`, `0` (the poller deletes managed `.internal` CRs and the controller deletes their
Services within a poll); upstream `NXDOMAIN`. Stop the extra `.internal` agents.

## 5. Health, uninstall (R16), teardown

Collect logs first; F removes the pods.

```sh
kubectl -n $NS logs -l app.kubernetes.io/component=private-endpoint-forwarder --tail=200 | grep -iE 'error|warn' | sort | uniq -c | head -30
kubectl -n $NS logs deploy/ngrok-operator-manager --tail=300 | grep -i privateendpoint | grep -iE 'error|conflict' | head
```

Expected: only the known quic-go receive-buffer warning.

### F. Feature disable and uninstall leave what behind? (R16)

Keep the agents from §2 running, so anything that survives is a real leftover, not just an
idle mirror. There are no ownerRefs or finalizers on CRs or per-host Services, so leftovers
are expected today; the point is to record exactly what remains.

```sh
leftovers() {
  echo "-- PrivateEndpoints";  kubectl -n $NS get privateendpoints --no-headers 2>&1 | wc -l
  echo "-- per-host Services"; kubectl -n $NS get svc -l ngrok.com/managed-by=private-endpoint-poller --no-headers 2>&1
  echo "-- forwarder objects"; kubectl -n $NS get deploy,sa,role,rolebinding -o name 2>&1 | grep -E 'private-endpoint' || echo none
  echo "-- CRD";               kubectl get crd privateendpoints.ngrok.com -o name 2>&1
}
leftovers | tee /tmp/r16.before
```

F1. Disable the feature, keep the release:

```sh
helm upgrade ngrok-operator helm/ngrok-operator -n $NS --reuse-values --set ngrok.features.privateEndpoints.enabled=false
sleep 30   # three poll intervals
leftovers | tee /tmp/r16.disabled
kubectl -n $NS get endpointslices -l kubernetes.io/service-name=pe-web-$U-ngrok-direct -o jsonpath='{.items[*].endpoints}{"\n"}'
nx "dig +short pe-web-$U.ngrok.direct; curl -s -m 5 -o /dev/null -w '%{http_code}\n' http://pe-web-$U.ngrok.direct; echo rc=\$?"
```

Run `helm` from the repo root. Look for:
- PrivateEndpoints and per-host Services still present, no longer updated (likely).
- Forwarder Deployment, ServiceAccount, Role, RoleBinding and the api-manager private
  endpoints Role gone (Helm-owned, expected).
- DNS still answers with the stale ClusterIP; the Service has no endpoints, so the connection
  fails. Record whether it's refused or times out, and how long it takes.

F2. Re-enable and check it picks the leftovers back up:

```sh
helm upgrade ngrok-operator helm/ngrok-operator -n $NS --reuse-values --set ngrok.features.privateEndpoints.enabled=true
kubectl -n $NS rollout status deploy -l app.kubernetes.io/component=private-endpoint-forwarder --timeout=120s
sleep 15; pes
nx "curl -s -o /dev/null -w '%{http_code}\n' http://pe-web-$U.ngrok.direct"
```

Expected: same CR names, all `READY=True`, forwarder ports unchanged; `200`.

F3. Uninstall:

```sh
helm uninstall ngrok-operator -n $NS
sleep 15
leftovers | tee /tmp/r16.uninstalled
kubectl -n kube-system get cm coredns -o jsonpath='{.data.Corefile}' | grep -c 'ngrok-private-endpoints begin'
```

Look for leftover PrivateEndpoints and per-host Services (likely orphaned), forwarder
objects (expected gone), the CRD (comes from the separate ngrok-crds chart, so it may stay),
and the CoreDNS block (admin-owned, expected `1` until removed by hand).

Manual cleanup if anything is left:

```sh
kubectl -n $NS delete privateendpoints --all
kubectl -n $NS delete svc -l ngrok.com/managed-by=private-endpoint-poller
```

Then tear down:

```sh
kind delete cluster --name $KIND_CLUSTER_NAME
```

## Record results

Paste into findings.md `## E2E rerun — <date>`:

1. Versions block (kind, k8s, CoreDNS image, ngrok agent, git sha) and `U`.
2. `pes` + Service list from §2.
3. B1–B12 one line each, like the rev-2 table.
4. D: resolv.conf (once), per-image results + `time`, any musl/glibc difference.
5. E: four timestamps + negative SOA TTL; computed delay.
6. C: timestamp loop output (first and last few lines), curl result; fixed / not fixed.
7. B: OLD_FWD, NEW_FWD, Service-gone timestamp vs agent-stop time.
8. A: `pes` line, diff result, unowned Service survived deletion (y/n).
9. Unexpected log lines.
9b. §4b: setup counts, I1–I7, cleanup counts and how long cleanup took.
10. F (R16): `/tmp/r16.before`, `/tmp/r16.disabled`, `/tmp/r16.uninstalled` side by side; disabled-state connect result (refused / timeout, seconds); F2 re-adopt result (same names and ports, y/n); CRD and CoreDNS block after uninstall.
11. Update README.md tracker: R1/R2/R3/R19 status, R16 (verified: what's orphaned), D8 verified live, T6/T10 checkboxes.
