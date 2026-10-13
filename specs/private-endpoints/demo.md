# Private endpoints demo on kind

A canned, one-command demo of the POC: a kind cluster with the operator (private endpoints
on), the CoreDNS rewrite, and demo apps published as private endpoints by the operator's own
`AgentEndpoint` CRD. It follows D8: only `*.ngrok.direct` endpoints are mirrored by default,
and one `.internal` endpoint is there to show it is *not* mirrored. No ngrok CLI on the
host. For the full manual test plan see [runbooks/kind-e2e.md](runbooks/kind-e2e.md).

Status: the first version (tls/tcp on `.internal`) was run live on kind 2026-10-12. The D8
rework (`.ngrok.direct` for every protocol, `.internal` negative check, wait timeout, token
preflight) has not been run live yet; see "Untested" at the end.

## What it builds

```
pe-demo ns: web (http-echo), redis, tls (openssl s_server, self-signed CN=<prefix>-tls.ngrok.direct)
     │  AgentEndpoints (bindings: [internal]) run by the operator's agent-manager
     ▼
ngrok cloud: http+https://<p>-web.ngrok.direct
             tls://<p>-tls.ngrok.direct   tcp://<p>-redis.ngrok.direct:6379
             http://<p>-web.internal      (negative case: not mirrored by default)
     │  poller (10s) → PrivateEndpoint CRs → per-host Services → forwarder → private dial
     ▼
pe-demo-client pod (netshoot) dials the real hostnames
```

Traffic hairpins on purpose: client pod → forwarder → ngrok cloud → agent-manager → origin
pod, all in the same cluster. It exercises the same path as a real off-cluster endpoint.

`<prefix>` defaults to `pe-<your $USER>` (lowercase alnum, max 12 chars; random if `$USER` is
empty) and is stored as an annotation on the `pe-demo` namespace, so later commands reuse it.
Override with `PE_DEMO_PREFIX=pe-foo`. Use a unique one on a shared account.

**The poller mirrors every private endpoint in the account**, not just the demo's. On a shared
account `kubectl get privateendpoints` will list other people's endpoints too, and they get
Services and forwarder ports in your cluster. That's the POC's accepted behavior (no
selectors yet), and a fair thing to point out in a demo.

## Prerequisites

- `NGROK_ACCESS_TOKEN`: an ngrok API key / PAT for an account with private endpoints.
- docker, kind, kubectl, helm (all in `nix develop`).
- Outbound UDP/443 (private dial is QUIC-only). Corporate VPNs often block it.
- Images pulled at runtime: `hashicorp/http-echo:1.0`, `redis:7-alpine`, `nicolaka/netshoot`.

## Quickstart

```sh
export NGROK_ACCESS_TOKEN=<token>
make pe-demo-up        # token check + kind cluster (if missing) + build/deploy + DNS + demo apps; waits for Ready
make pe-demo-test      # automated PASS/FAIL checks
make pe-demo-shell     # interactive client pod
make pe-demo-down      # remove demo apps (PE_DEMO_DELETE_CLUSTER=true also deletes the cluster)
```

`pe-demo-up` steps, in order:

1. **Token preflight** (`demo.sh preflight`): `GET https://api.ngrok.com/endpoints?limit=1`
   with `NGROK_ACCESS_TOKEN`. On 401/403 it prints the HTTP status and ngrok error code
   (e.g. `ERR_NGROK_203`) and stops before building anything. Other non-200 codes only warn.
2. kind cluster (`KIND_CLUSTER_NAME`, default `ngrok-operator`) if missing; switches kubectl to
   `kind-$KIND_CLUSTER_NAME`.
3. Build + `helm upgrade --install` with `ngrok.features.privateEndpoints.enabled=true`, then the CoreDNS
   rewrite (`.ngrok.direct` only by default).
4. Apply the demo (`demo.sh apply`).
5. Wait (`demo.sh wait`) up to `PE_DEMO_TIMEOUT` seconds (default 180) for the origins, every
   AgentEndpoint `Ready`, and a `Ready` PrivateEndpoint for each `.ngrok.direct` URL. Prints
   what's still pending every ~15s. On timeout it prints diagnostics (below) and exits 1.

It's safe to rerun: it rebuilds and redeploys the operator and re-applies the demo.
`make pe-demo-status` shows PrivateEndpoints, per-host Services, forwarder pods, and the demo
AgentEndpoints.

## Live demo script

Before: `make pe-demo-up`, then `make pe-demo-test` once so you know it's green. Have two
terminals: one host shell, one `make pe-demo-shell` (it prints the commands below with your
prefix filled in; `P` below is your prefix).

1. **What exists.** Host: `kubectl -n ngrok-operator get privateendpoints`.
   Point out: one CR per endpoint URL, created by the poller from the ngrok API, nobody wrote
   them. Each has a ClusterIP and a forwarder port. Other people's `.ngrok.direct` endpoints in
   the account show up too. No `.internal` rows (see step 7).
2. **Real hostnames, plain curl.** Client: `curl http://$P-web.ngrok.direct`.
   No sidecar, no proxy env, no app change. Show `dig +noall +answer $P-web.ngrok.direct`: the
   answer is the queried name (CoreDNS rewrite) pointing at the per-host Service.
3. **TLS verifies.** Client: `curl -v https://$P-web.ngrok.direct`.
   Point out `SSL certificate verify ok` with no `-k`: ngrok's public cert, end to end.
4. **TLS passthrough.** Client:
   `echo | openssl s_client -connect $P-tls.ngrok.direct:443 -servername $P-tls.ngrok.direct 2>/dev/null | grep subject=`.
   The subject is the origin's self-signed cert: the forwarder and ngrok never terminate it.
5. **Raw TCP.** Client: `printf 'PING\r\n' | nc -w 3 $P-redis.ngrok.direct 6379` → `+PONG`.
6. **Endpoints come and go.** Host:
   `kubectl -n pe-demo delete agentendpoint redis-direct`, then
   `kubectl -n ngrok-operator get privateendpoints -w`. Within ~10s the CR and its Service
   disappear; in the client, `dig $P-redis.ngrok.direct` turns NXDOMAIN (after the short DNS
   TTL). Bring it back with `./scripts/private-endpoints-demo/demo.sh apply`; it reappears
   within a poll.
7. **`.ngrok.direct` only by default.** Host: `kubectl -n pe-demo get agentendpoint web-internal`
   is Ready in ngrok, but there is no PrivateEndpoint or Service for it. Client:
   `dig +short $P-web.internal` is empty. `.ngrok.direct` is the supported pattern; `.internal`
   is opt-in (below) because it collides with cloud DNS (`ec2.internal`, GKE
   `metadata.internal`) and with the operator's own Ingress/Gateway endpoints.
8. **Unknown names still fail fast.** Client: `dig nope-123.ngrok.direct` → NXDOMAIN;
   `dig kubernetes.default.svc.cluster.local` still works.

## Opting in to `.internal`

Off by default (D8). To mirror `.internal` endpoints too, on a running demo cluster:

```sh
helm upgrade ngrok-operator ./helm/ngrok-operator -n ngrok-operator --reuse-values \
  --set ngrok.features.privateEndpoints.includeInternal=true     # api-manager --features-private-endpoints-include-internal
PE_INCLUDE_INTERNAL=true make kind-private-endpoints-dns   # add the .internal CoreDNS rewrite
PE_INCLUDE_INTERNAL=true make pe-demo-test
```

What changes: the poller creates a PrivateEndpoint + Service for `http://$P-web.internal`
(and any other `.internal` endpoint in the account, including ones the operator itself creates
for Ingress/Gateway), `$P-web.internal` resolves to that Service, and with
`PE_INCLUDE_INTERNAL=true` the `.internal` checks flip from "not mirrored" to "resolves and
returns 200". Running `make pe-demo-test` *without* the env var after opting in fails the
negative check, as intended. `make pe-demo-up` always deploys the defaults, so it undoes the
Helm opt-in (rerun the `helm upgrade` afterwards).

## Tests

`make pe-demo-test` runs one check on the host, then pipes
`scripts/private-endpoints-demo/checks.sh` into the client pod. Each in-pod check retries for
~18s (poll interval + negative cache) then prints `PASS`/`FAIL`; the target exits nonzero if
any fail.

| Check | Pass condition | Proves |
|---|---|---|
| host: `.internal` not mirrored | no PrivateEndpoint with URL `http://<p>-web.internal`, no Service `<p>-web-internal` | D8 default: poller skips `.internal` |
| dns `<p>-web` / `<p>-tls` `.ngrok.direct` | resolves to the same IP as `<p>-web-ngrok-direct.<ns>.svc` / `<p>-tls-ngrok-direct.<ns>.svc` | CoreDNS rewrite → per-host Service |
| dns unknown | random `<p>-nope-N.ngrok.direct` → NXDOMAIN | no wildcard answer for names that aren't mirrored |
| dns cluster | `kubernetes.default.svc.cluster.local` → NOERROR | rewrite doesn't break cluster DNS |
| http `.ngrok.direct` | 200 and body `hello from <p>-web` | forwarder → private dial → agent → origin |
| https `.ngrok.direct` | 200, `ssl_verify_result=0` (verification on) | public cert, no `-k` |
| tls `.ngrok.direct` | `openssl s_client` subject `CN=<p>-tls.ngrok.direct` | TLS passthrough (origin's own cert) |
| tcp redis | `PING` → `+PONG` | raw TCP on a non-HTTP port |
| dns `<p>-web.internal` | empty answer (NXDOMAIN or nothing) | no `.internal` rewrite by default |

With `PE_INCLUDE_INTERNAL=true` the two `.internal` rows flip: the CR and Service must exist,
the name must resolve to the Service IP, and `http://<p>-web.internal` must return 200.

Knobs: `RETRIES` (in checks.sh), `PE_DEMO_TIMEOUT` (seconds for Ready waits, default 180),
`PE_DEMO_NS` (default `pe-demo`), `KUBE_NAMESPACE` (operator, default `ngrok-operator`),
`PE_INCLUDE_INTERNAL` (default `false`).

## Fault scenarios

Live fault tests for the correctness bar in the [tracker](README.md) (FT-3, FT-5, FT-6). FT-1,
FT-2 and FT-4 are covered by unit/envtest tests, not here. Each scenario mutates the demo
cluster, prints what it does and what it expects, then one `PASS`/`FAIL`/`OBSERVE` line per
finding and exits 1 on any `FAIL`. `FAIL` means "short of the bar"; several are expected to fail
on today's code, and that's the point. It refuses to run unless the kube context is `kind-*`.
Run `make pe-demo-up` (D8 build) first: the scenarios use `$P-redis.ngrok.direct` and the
client pod.

```sh
make pe-demo-faults FAULT=origin-restart                       # FT-3, R29
make pe-demo-faults FAULT=rolling-restart                      # FT-5, R24
make pe-demo-faults FAULT=uninstall                            # FT-6, R16: dry run, prints commands
make pe-demo-faults FAULT=uninstall FAULT_ARGS=--yes           # FT-6, R16: disable + re-enable
make pe-demo-faults FAULT=revoke                               # FT-5, R28: manual steps only
```

| Scenario | What it does | Proves (when it passes) | Expected today |
|---|---|---|---|
| `origin-restart` | Deletes the `redis-direct` AgentEndpoint for `FAULT_DOWN_SECONDS` (default 25, spans a 10s poll), re-applies it. `FAULT_ORIGIN_MODE=agent` scales the agent-manager to 0 and back instead. A client resolves `$P-redis.ngrok.direct` once and keeps dialing that IP, next to a probe that re-resolves | A short origin outage keeps the PrivateEndpoint, Service and ClusterIP, so DNS-caching apps recover | **FAIL**: CR and Service deleted within a poll and re-created with a new UID and usually a new ClusterIP; the caching client never recovers, the re-resolving one does |
| `rolling-restart` | Holds one TCP connection to redis (PING every 1s) and opens a new one every 0.5s, then `kubectl rollout restart` of the forwarder | Held connections drain instead of being cut; no new connection fails during a rollout | **FAIL** for the held connection: cut within a second or two of the restart (no preStop, no drain; 1 replica). New connections: probably a few failures around the pod swap; record the count |
| `uninstall` | `helm upgrade --reuse-values --set ngrok.features.privateEndpoints.enabled=false`, waits `FAULT_SETTLE_SECONDS` (30), lists leftovers (same list as [kind-e2e §5 F](runbooks/kind-e2e.md)), dials the demo, re-enables, runs `demo.sh wait`, compares CRs | Disable cleans up CRs and per-host Services; re-enable recovers the same names, IPs and ports | **FAIL** on cleanup: PrivateEndpoints and Services orphaned (no ownerRefs/finalizers), DNS still answers with a ClusterIP that has no endpoints (record refused vs timeout). Forwarder objects gone (Helm-owned): PASS. CoreDNS block stays (admin-owned): OBSERVE. Re-enable: PASS |
| `revoke` | Prints manual steps: watch forwarder readiness, restarts, logs and a curl loop; revoke or rotate the token in the dashboard; put a valid token in the Secret and restart | The forwarder stops claiming Ready on a terminal auth failure and recovers once the Secret holds a valid token | R28 fix in the working tree: on 401/403 at reconnect the forwarder logs `private dial session ended permanently, exiting so the pod restarts` and exits once; the restarted container sits not-Ready until a valid token is in the Secret (no crashloop). Updating the Secret + a container restart recovers it (the token is read at container start). Caveat: if the token was already bad at startup the SDK never marks the session terminal, so it just stays not-Ready. The manager may still crashloop |

`helm uninstall` itself is not scripted (it would take the demo down); use kind-e2e §5 F3.

Knobs: `FAULT_DOWN_SECONDS`, `FAULT_ORIGIN_MODE` (`endpoint`|`agent`), `FAULT_RECOVER_TIMEOUT`
(default 120), `FAULT_SETTLE_SECONDS`, plus `PE_DEMO_NS`, `KUBE_NAMESPACE`, `HELM_RELEASE_NAME`.

**Shared with bindings.** These are not new to private endpoints; the bindings forwarder has
the same shape:

- `rolling-restart`: yes. Both forwarders accept through `pkg/bindingsdriver`, which closes the
  listener on stop but does nothing for in-flight connections, and neither Deployment has a
  preStop hook. A bindings-forwarder restart cuts held bindings connections the same way.
- `origin-restart`: yes. `BoundEndpointPoller` deletes a BoundEndpoint the first time a poll
  doesn't list its endpoint (`internal/controller/bindings/boundendpoint_poller.go`, the
  "not in our desired set" branch), and the BoundEndpoint delete removes its Services, so the
  IP churns the same way.
- `uninstall`: partly. BoundEndpoints carry finalizers and the operator has a drain path, so
  the failure mode differs (stuck finalizers rather than silent orphans). Not checked here.
- `revoke`: the R28 fix is private-endpoint-forwarder only; whether the bindings forwarder
  notices a revoked token is not checked.

## Teardown

```sh
make pe-demo-down                                 # demo namespace only
make pe-demo-down PE_DEMO_DELETE_CLUSTER=true     # also the kind cluster
```

Delete the demo before uninstalling the operator: AgentEndpoints carry a finalizer the
agent-manager must remove. The CoreDNS rewrite stays until the cluster is deleted.

## Troubleshooting

**Bad token.** `pe-demo-up` stops at the preflight with e.g.
`NGROK_ACCESS_TOKEN rejected by the ngrok API (HTTP 401, ERR_NGROK_203)` and the API's
message. Fix the token (`export NGROK_ACCESS_TOKEN=...`; it must be an API key / PAT for an
account with private endpoints) and rerun. If the token is revoked *after* deploy, the manager
crashloops; `make pe-demo-up` again with a good token redeploys the secret.

**Stuck or timed-out wait.** `demo.sh wait` prints what's pending every ~15s:
`AgentEndpoints not Ready: ...` and/or `PrivateEndpoints missing/not Ready: ...`. On timeout
(`PE_DEMO_TIMEOUT`, default 180s) it dumps: AgentEndpoint conditions, PrivateEndpoints with
conditions, per-host Services, operator and demo pods, the last 30 log lines of the
api-manager and forwarder (and the agent if any AgentEndpoint isn't Ready), each also from the
previous container if it restarted, plus the api-manager's recent poller/error lines. Rerun
just the wait with `./scripts/private-endpoints-demo/demo.sh wait` (or a longer
`PE_DEMO_TIMEOUT=600`).

| Symptom | Look at |
|---|---|
| Pending: AgentEndpoints not Ready | AgentEndpoint conditions + agent logs in the diagnostics. Agent pod not Running → token/secret |
| AgentEndpoints Ready, PrivateEndpoints missing | api-manager poller/error lines; manager pod restarts (bad token → crashloop); poller only lists every ~10s |
| PrivateEndpoint `Ready=False` | `REASON` column: `ServiceConflict` (a Service with that name already exists), `PortsExhausted` |
| `.internal` check fails by default | a leftover CR/Service from before D8 or from an opt-in; the poller should delete it within a poll. Check the manager has the D8 build |
| dig works, curl/nc hang | forwarder logs: bind errors, or QUIC blocked (UDP/443 egress) |
| name doesn't resolve at all | `make kind-private-endpoints-dns` again; check `kubectl -n kube-system get cm coredns -o yaml` has the `ngrok-private-endpoints` block |
| https verify fails | you hit `.internal` with https (no public cert there), or a middlebox intercepts TLS |
| other people's endpoints in the list | expected: the poller mirrors the whole account (`.ngrok.direct` only by default) |

## Untested / assumptions

- The D8 version has not run live. Checked: `bash -n`, shellcheck, `make -n pe-demo-up`,
  `kubectl apply --dry-run=client` of the rendered manifest, the preflight against a stubbed
  `curl` (200/401/500/network error), and the diagnostics helpers read-only against a kind
  cluster running the previous demo.
- `faults.sh` has not run live. Checked: `bash -n`, shellcheck (including the in-pod loops),
  `make -n pe-demo-faults`, the uninstall dry run and `revoke` output, the held-connection
  loop against a local fake redis, and the read-only helpers (PrivateEndpoint/Service lookup,
  leftovers list) against the `kind-pe-demo` cluster, which still ran the pre-D8 demo.
- `tls://<p>-tls.ngrok.direct` and `tcp://<p>-redis.ngrok.direct:6379` with `bindings:
  [internal]`: in the earlier run, AgentEndpoints and PrivateEndpoints went Ready for
  http/https on `.ngrok.direct` and tls/tcp on `.internal`, never tls/tcp on `.ngrok.direct`.
  If the API rejects them, the AgentEndpoint conditions in the
  wait diagnostics will say so.
- `AgentEndpoint` with explicit `bindings: [internal]` on `*.ngrok.direct` is accepted (seen
  Ready for http/https in the first run).
- TLS passthrough uses a `tcp://` upstream so ngrok-go forwards raw bytes (a `tls://` upstream
  would wrap the stream in a second TLS layer).
- Upgrading a cluster that ran the older demo: `demo.sh apply` deletes the old
  `redis-internal`/`tls-internal` AgentEndpoints; their PrivateEndpoints disappear once the D8
  poller runs.
