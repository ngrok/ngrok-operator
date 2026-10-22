# Private Endpoints POC — findings

## Probe (Task 1) — 2026-09-29

Agents (ngrok 3.39.9, authenticated with the same PAT via `NGROK_AUTHTOKEN`):

```
python3 -m http.server 18080
ngrok http 18080 --url http://pe-probe-<u>.internal
ngrok http 18080 --url https://pe-probe-<u>.ngrok.direct
```

Both started without extra flags; `.ngrok.direct` needs no special syntax.

`Endpoints.List` (ngrok-api-go v9, private endpoints only shown; other rows were public):

```
url=https://pe-probe-<u>.ngrok.direct scheme= host= port=0 type=ephemeral bindings=[internal] pooling=false
url=http://pe-probe-<u>.internal      scheme= host= port=0 type=ephemeral bindings=[internal] pooling=false
```

- `Scheme`, `Host`, `Port` are empty in list results — everything must be parsed from `URL`.
- Both TLDs report `bindings=[internal]`. Hostname-suffix filtering and a `bindings` contains
  `internal` check would select the same set; the POC uses the suffix (see plan).

Private dial (`privatedial`, `ProtocolQUIC`, `quic.connect-endpoint.ngrok.com:443`, PAT auth):

```
pe-probe-<u>.internal:80 status: 200 OK
pe-probe-<u>.ngrok.direct:443 cert subject: CN=*.ngrok.direct issuer: CN=YE2,O=Let's Encrypt,C=US dns: [*.ngrok.direct ngrok.direct]
pe-probe-<u>.ngrok.direct:443 status: 200 OK
```

- Assumption confirmed: dialing an `https://…ngrok.direct` endpoint on 443 yields a TLS session
  terminated by ngrok with a publicly trusted wildcard cert. SNI passthrough on the shared :443
  listener works for https endpoints with verification on.
- quic-go warns `failed to sufficiently increase receive buffer size` (wanted 7168 kiB, got
  2048 kiB). Harmless for the POC; in-cluster it may be worth raising `net.core.rmem_max`.

## E2E (Task 10) — 2026-09-29

Fresh kind cluster (`kind create cluster --name pe-poc`, kind v0.32.0, Kubernetes v1.36.1),
`KIND_CLUSTER_NAME=pe-poc make deploy_with_private_endpoints` then `make kind-private-endpoints-dns`
(run twice; Corefile keeps exactly one managed block).

Endpoints started outside the cluster (ngrok 3.39.9):

```
ngrok http 18080 --url http://pe-web-<u>.internal
ngrok http 18080 --url https://pe-web-<u>.ngrok.direct
ngrok tcp 16379 --url tcp://pe-redis-<u>.internal:6379    # docker redis:7
ngrok tls 19443 --url tls://pe-tls-<u>.internal           # openssl s_server, self-signed CN=pe-tls-<u>.internal
```

Mirrored within one poll:

```
NAME                  URL                                     SCHEME   CLUSTERIP       READY
pe-261bcaa0484f5333   http://pe-web-<u>.internal              http     10.96.160.238   True
pe-5fdf6c2860f1d973   tls://pe-tls-<u>.internal:443           tls      10.96.160.238   True
pe-949caaeecef9d464   tcp://pe-redis-<u>.internal:6379        tcp      10.96.241.242   True
pe-b977cc296f6c5997   https://pe-web-<u>.ngrok.direct         https    10.96.160.238   True
```

(10.96.160.238 = shared `ngrok-operator-private-endpoints` Service; 10.96.241.242 = `pe-host-…` for the redis host.)
Note the API reports the tls URL with an explicit `:443`.

From a `nicolaka/netshoot` pod:

```
== http .internal                       200
== https .ngrok.direct (verify on)      200 ssl_verify=0
== tls .internal                        subject=CN=pe-tls-<u>.internal   (end-to-end, origin cert)
== tcp redis                            +PONG
== dig known                            10.96.160.238 / 10.96.241.242 / 10.96.160.238
== nxdomain ngrok.direct                status: NXDOMAIN
== fallthrough .internal                metadata.google.internal → status: NXDOMAIN (from upstream 172.18.0.1, not SERVFAIL)
== cluster dns                          kubernetes.default.svc.cluster.local → 10.96.0.1
```

Cleanup: stopped the `ngrok tcp` agent at 16:28:29; CR and `pe-host-…` Service gone by 16:28:33;
`dig pe-redis-<u>.internal` → NXDOMAIN (falls through upstream) after the 5s TTL.

Bug found and fixed during e2e: the private endpoints flags were added to the shared
`cliFeatureFlags` Helm helper, which also feeds agent-manager; agent-manager exited with
`unknown flag: --enable-feature-private-endpoints`. Moved to the api-manager deployment only.

Known POC limitations observed / carried:
- quic-go warns about UDP receive buffer size (`wanted 7168 kiB, got 2048 kiB`); traffic still works.
- With `watchNamespace` set to a namespace other than the release namespace, the api-manager
  cache doesn't see operator-namespace Services, so the controller can't wire hostnames. Not
  exercised here.

## E2E, revision 2 (CoreDNS rewrite + Service per hostname) — 2026-09-29

Same `pe-poc` kind cluster (CoreDNS v1.14.2), redeployed with `make deploy_with_private_endpoints`
and `make kind-private-endpoints-dns` (run twice; one managed block inside `.:53`, the revision-1
`internal:53` / `ngrok.direct:53` stanzas removed).

Endpoints: the four from revision 1, plus `http://pe-web-<u>.ngrok.direct` alongside the
existing `https://pe-web-<u>.ngrok.direct`, so one hostname has two endpoints and the label
`pe-web-<u>` exists under both TLDs.

```
URL                                   IP             FWD     READY
http://pe-web-<u>.ngrok.direct        10.96.78.224   20001   True
https://pe-web-<u>.ngrok.direct       10.96.78.224   20002   True
http://pe-web-<u>.internal            10.96.81.153   20003   True
tls://pe-tls-<u>.internal:443         10.96.199.13   20004   True
tcp://pe-redis-<u>.internal:6379      10.96.54.118   20000   True

NAME                        PORT(S)
pe-redis-<u>-internal       6379/TCP
pe-tls-<u>-internal         443/TCP
pe-web-<u>-internal         80/TCP
pe-web-<u>-ngrok-direct     80/TCP,443/TCP
```

From a `nicolaka/netshoot` pod:

```
== http .internal                 200
== http .ngrok.direct             200
== https .ngrok.direct (verify)   200 ssl_verify=0
== tls .internal                  subject=CN=pe-tls-<u>.internal
== tcp redis                      +PONG
== dig answer name                pe-web-<u>.internal. 27 IN A 10.96.81.153   (answer auto works)
== uppercase query                PE-WEB-<u>.Internal → 10.96.81.153
== unknown single-label           nope-<u>.internal → NXDOMAIN
== cluster dns                    kubernetes.default.svc.cluster.local → 10.96.0.1
```

Multi-label fall-through: with a temporary CoreDNS `hosts` entry after the rewrites
(`169.254.169.254 metadata.google.internal`), `dig metadata.google.internal` returned
169.254.169.254, so multi-label `.internal` names are not rewritten. Entry removed afterwards.

Cleanup:
- Stopped the `ngrok tcp` agent at 18:03:58; CR and `pe-redis-<u>-internal` Service gone by
  18:04:00; `dig` → NXDOMAIN.
- Stopped only `http://pe-web-<u>.ngrok.direct`: its Service dropped to `443/TCP`, https still
  200, and `curl http://…` now times out (no port on the Service) rather than being refused.

## Live kind run, iteration 2 (D8 + review fixes) — 2026-10-13

Cluster `kind-pe-demo`, build from `bbc29476` (D8, PR-1, R19, R21/R27, R28, O1/O2). Run by Claude
with Alex's personal ngrok account. Prefix `pe-vscode`.

**Demo (`make pe-demo-test`): 11/11 PASS.** First run with tls and tcp on `.ngrok.direct`: both work
(`tls://` origin cert passthrough, `tcp://…:6379` redis PING). `.internal` endpoint not mirrored, no
Service, no DNS answer. One FAIL on the very first run right after upgrading from the pre-D8 build:
the stale `.internal` CRs were still there because the new poller hadn't polled yet; gone within
seconds. `demo.sh wait` now waits for that to settle.

**R19 drift heal: PASS.** Patched the redis PrivateEndpoint `spec.port` 6379→6380; restored within ~3s.

**PR-1 missing CRD: PASS.** Deleted the CRD, restarted api-manager: pod Running, logged
`WARNING: --enable-feature-private-endpoints is set but the PrivateEndpoint CRD ... is not installed,
Private Endpoints feature set will be disabled`, other controllers (ingress, service, cloudendpoint,
domain, …) started. Restored with `make pe-demo-up`; demo 11/11 again.

**Fault scenarios (`make pe-demo-faults`), all expected:**

| Scenario | Result | Detail |
|---|---|---|
| origin-restart (FT-3, R29) | FAIL ×5, expected | Origin down 25s. PrivateEndpoint + Service deleted 9s in (one poll). Recreated 5s after restart with new UID, new ClusterIP (10.96.170.250 → 10.96.73.215), new forwarder port (20006 → 20008). Client that cached the IP never recovers. Client that re-resolves recovered ~30s after the Service came back: CoreDNS `cache 30` held the NXDOMAIN from the outage (negative caching, T6). Bindings deletes on first missed poll too |
| rolling-restart (FT-5, R24) | FAIL ×2, expected | Held redis connection survived until the old pod terminated (~12s), then cut. 1 of 10 new connections failed around the rollout. Single replica, no preStop/drain. Bindings shares this |
| uninstall = disable/re-enable (FT-6, R16) | FAIL ×2 expected, 1 timing | Disable leaves 5 PrivateEndpoints and 4 per-host Services; forwarder objects removed; DNS still answers with a ClusterIP and connections are refused fast (no endpoints). Re-enable reuses the same CRs, ClusterIPs and forwarder ports. The post-re-enable curl failed because it ran ~2s before the new forwarder was in the Service endpoints; 200 seconds later. Script now retries |
| revoke (FT-5, R28) | not run | Manual (needs dashboard) |

New observations:
- Negative DNS caching: an endpoint outage that spans a poll adds up to 30s (CoreDNS `cache 30`
  denial TTL) of NXDOMAIN after recovery even for well-behaved clients. Feeds R29.
- Forwarder ports aren't stable across a delete/recreate (20006 → 20008, cursor moved on). Fine
  for routing (O1), but means R29 retention would also need to keep the port.
