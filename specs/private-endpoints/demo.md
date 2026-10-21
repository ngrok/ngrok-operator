# Private endpoints demo on kind

A canned, one-command demo of the POC: a kind cluster with the operator (private endpoints
on), the CoreDNS rewrite, and demo apps published as private endpoints by the operator's own
`AgentEndpoint` CRD. Only `*.ngrok.direct` endpoints are mirrored; `.internal` is not
supported (D9). No ngrok CLI on the host. For the full manual test plan see [runbooks/kind-e2e.md](runbooks/kind-e2e.md).

Status: demo run live on kind 2026-10-12 and 2026-10-13. Chainsaw suite (`make pe-demo-test`)
run on `kind-pe-demo` 2026-10-21 against the D9 build: 4/4 pass in parallel, ~50s.

## What it builds

```
pe-demo ns: web (http-echo), redis, tls (openssl s_server, self-signed CN=<prefix>-tls.ngrok.direct)
     │  AgentEndpoints (bindings: [internal]) run by the operator's agent-manager
     ▼
ngrok cloud: http+https://<p>-web.ngrok.direct
             tls://<p>-tls.ngrok.direct   tcp://<p>-redis.ngrok.direct:6379
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
make pe-demo-test      # chainsaw e2e suite (tests/chainsaw-private-endpoints), in parallel
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
   rewrite for `.ngrok.direct`.
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
   the account show up too.
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
7. **Unknown names still fail fast.** Client: `dig nope-123.ngrok.direct` → NXDOMAIN;
   `dig kubernetes.default.svc.cluster.local` still works.

## Tests

`make pe-demo-test` (same as `make e2e-tests-private-endpoints`) runs the chainsaw suite in
`tests/chainsaw-private-endpoints/` against the current cluster, 4 tests at a time. It needs
the feature on and the kind CoreDNS rewrite, so run `make pe-demo-up` first. It doesn't use the
`pe-demo` namespace or demo prefix: each test gets its own random chainsaw namespace, uses it
as the hostname prefix, brings up its own origin, `AgentEndpoint` and netshoot client pod, and
deletes them at the end. So tests don't step on each other or on the live demo.

| Test | Asserts | Proves |
|---|---|---|
| `http` | http + https PrivateEndpoints on one host `Ready`, one Service with 2 ports; name resolves to the Service IP; body `hello from <ns>`; https `200 ssl_verify=0` | CoreDNS rewrite → per-host Service → forwarder → private dial → agent → origin; public cert, no `-k` |
| `tls` | PrivateEndpoint `Ready`; `openssl s_client` subject `CN=<ns>-tls.ngrok.direct` | TLS passthrough (origin's own cert) |
| `tcp` | PrivateEndpoint `Ready`; redis `PING` → `+PONG` | raw TCP on a non-HTTP port |
| `dns` | `.internal` AgentEndpoint gets no PrivateEndpoint after two polls; unknown `.ngrok.direct` name NXDOMAIN; `kubernetes.default.svc.cluster.local` NOERROR | `.internal` not mirrored; no wildcard answer; cluster DNS intact |

In-pod checks go through `tests/chainsaw-private-endpoints/_fixtures/check.sh`, which retries
(`RETRIES`, default 20, 3s apart) to ride out the poll interval and CoreDNS negative cache.
`KUBE_NAMESPACE` sets the operator namespace (default `ngrok-operator`).

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
| `dns` test finds a `.internal` PrivateEndpoint | the api-manager runs a build from before D9 (`.internal` support removed); redeploy with `make pe-demo-up` |
| dig works, curl/nc hang | forwarder logs: bind errors, or QUIC blocked (UDP/443 egress) |
| name doesn't resolve at all | `make kind-private-endpoints-dns` again; check `kubectl -n kube-system get cm coredns -o yaml` has the `ngrok-private-endpoints` block |
| https verify fails | a middlebox intercepts TLS |
| other people's endpoints in the list | expected: the poller mirrors every `.ngrok.direct` endpoint in the account |

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
