# Migration: Kubernetes bindings → private endpoints, and operator `.internal` → `.ngrok.direct`

Status: draft, 2026-10-12 · Owner: Alex Bezek · Audience: operator devs + product (internal, not user docs)

Two migrations, both long-term and post-POC:

- **A. Bindings → private endpoints.** Get users off `BoundEndpoint` / the `kubernetes` binding
  and onto private endpoints + private dial (`PrivateEndpoint`, this POC).
- **B. Operator-generated internal endpoints → `.ngrok.direct`.** Ingress/Gateway/Service
  translation stops emitting `*.internal` AgentEndpoints. Post-1.0 (Slack 2026-10-12).

Related: [README.md](README.md) (tracker), [strategy.md §5](strategy.md#5-comparison-with-kubernetes-bindings)
(full side-by-side), [design.md](design.md). Facts below are cited `file:line` against
`alex/private-endpoints-poc`; anything not checked in code is marked **unverified**.

---

## 1. TL;DR

- Bindings is small and still labelled "in-development" (`helm/ngrok-operator/values.yaml:367`),
  so the user base is probably small. **We can measure it**: every operator registers a
  `KubernetesOperator` with its enabled features, including `bindings`
  (`cmd/api-manager.go:896-903`), so the ngrok API knows exactly which installs use it.
- For an app, the migration is a **hostname change**: `http://my-svc.my-ns` →
  `https://my-svc.ngrok.direct`. The endpoint owner restarts the endpoint with a new URL and
  without `--binding kubernetes`. The cluster admin does a one-time DNS step.
- **Four gaps block deprecation:** scoped exposure / per-cluster targeting (bindings has
  `endpointSelectors`, we mirror the whole account), pod identity to Traffic Policy, a DNS story
  that works on the major platforms, and a TCP-only egress path (we need UDP/443 today).
- **Shim recommendation:** docs-only. A hand-made ExternalName Service `my-svc` in `my-ns`
  pointing at the per-host Service keeps the old `my-svc.my-ns` name working, with no CoreDNS
  change, using exactly the mechanism bindings uses today. Don't build an operator-managed alias
  for migration alone. Do consider it as a **feature** (namespace-local names, no DNS step) when
  D1 is decided.
- **Migration B:** the PM sketch (new AEPs → repoint CLEP → delete old) is right in shape, but
  the code doesn't support "create new first" today. The AEP object name is TLD-independent, so
  changing the TLD is an in-place URL swap that races the CLEP update. It needs a dual-emit
  window. Bigger unknowns: whether `.ngrok.direct` names are account-scoped, and whether a
  `.ngrok.direct` AEP without explicit bindings could come up public.

---

## 2. Feature parity: bindings vs private endpoints

**Blocking** = we shouldn't announce bindings deprecation until it's closed (or product
explicitly waives it). **Non-blocking** = document it, or fix it before "default".

| # | Capability | Bindings (today) | Private endpoints (POC) | Gap | Deprecation |
|---|---|---|---|---|---|
| P1 | **Which endpoints reach which cluster** | Opt-in twice: endpoint carries `kubernetes` binding, and must match the operator's `endpointSelectors` CEL (`api/ngrok/v1alpha1/kubernetesoperator_types.go:49`, Helm `ngrok.features.bindings.endpointSelectors`). Poller lists only endpoints bound to *this* operator (`internal/controller/bindings/boundendpoint_poller.go:214`, `GetBoundEndpoints(koId)`), so different clusters get different sets | Every private endpoint in the account, in every cluster with the feature on. Only filter: suffix and not `kubernetes`-bound (`internal/controller/privateendpoints/poller.go:141,152`) | PE lacks scoping and per-cluster targeting; endpoint owner can't opt out | **Blocking** |
| P2 | **Caller identity → Traffic Policy** | Forwarder maps source IP → Pod (`forwarder_controller.go:201-219`), sends UID, name, namespace and `k8s.ngrok.com/*` annotations (`:358-374`) in the binding upgrade (`:249`). Exact Traffic Policy variable names **unverified** | None. Private dial metadata is per session and not visible to Traffic Policy (GAT-475) | PE lacks it; needs backend | **Blocking** if any customer policy uses it (product to confirm); else non-blocking |
| P3 | **No cluster DNS change** | Plain Kubernetes Service DNS. Target is an ExternalName Service in the app's namespace (`boundendpoint_controller.go:421-441`) | One admin-owned CoreDNS rewrite per cluster; not possible on GKE/OpenShift without D1(b) | PE regresses install UX | **Blocking** until the top platforms have a working path (D1(b), or the alias shim in §4.2 as a stopgap) |
| P4 | **Egress** | mTLS over TCP/443 to `kubernetes-binding-ingress.ngrok.io` | QUIC only, UDP/443 (HTTP/2 fallback disabled on Go 1.27) | Clusters that block UDP have no path | **Blocking** (SDK track: fix HTTP/2 fallback) |
| P5 | Namespace-local names | Endpoint picks the namespace: `<svc>.<ns>` (`api/bindings/v1alpha1/boundendpoint_types.go:44`) | Cluster-global names; all Services in the operator namespace | Different model, arguably better | Non-blocking (alias shim covers it) |
| P6 | Labels/annotations on projected Services | Helm `bindings.serviceLabels` / `serviceAnnotations` → `--bindings-service-*` (`cmd/api-manager.go:175-176`); used for e.g. mesh or monitoring annotations | None on per-host Services | Small | Non-blocking (cheap to add if asked) |
| P7 | Scale | Port range 10000–65535 (`cmd/api-manager.go:842`) ≈ 55k endpoint-ports, not measured | 20000–20999 = 1000 endpoint-ports | PE lower | Non-blocking unless a bindings user is near 1000 (check via P1 telemetry) |
| P8 | Readiness | Controller dials the target Service after create (`boundendpoint_controller.go:543`) | `Ready` = configured only | Small | Non-blocking |
| P9 | Cleanup | `BaseController` finalizers delete Services | No finalizers/ownerRefs; uninstall likely orphans (R16, unverified) | PE worse | Non-blocking for deprecation; **gate for "default"** |
| P10 | Forwarder credential | mTLS client cert from the KO registration's TLS secret (`forwarder_controller.go:223`); **no API token in forwarder pods** | PAT in forwarder pods; can private-dial any endpoint in the account | PE holds a broader secret | Non-blocking; needs a security review note |
| P11 | Operator RBAC | ClusterRole: Services in **all** namespaces (`templates/api-manager/bindings-cluster-role.yaml`), Pods cluster-wide for the forwarder (`templates/bindings-forwarder/role.yaml:69-88`) | Namespace-scoped Roles | PE better | — |
| P12 | Hostname / TLS | `svc.ns`, so no public cert can match; https verification **unverified** | Real URL; `.ngrok.direct` https verifies | PE better | — |
| P13 | Hostname shapes | Exactly two labels, `<svc>.<ns>`, required (`internal/ngrokapi/bindingendpoint_aggregator.go:143-147`) | Single label under the TLD only, ≤50 chars for `.ngrok.direct` | Both constrained | — |
| P14 | Per-endpoint ceremony | `--binding kubernetes --url http://svc.ns` per endpoint (`templates/NOTES.txt:55`) | None | PE better | — |
| P15 | Pooled endpoints | Aggregated by URL into one `BoundEndpoint` (`bindingendpoint_aggregator.go:36`) | Aggregated by URL into one `PrivateEndpoint` | Parity | — |

**Blocking set for deprecation: P1, P2 (conditional), P3, P4.** P9 and the SDK pin (README
"Future") gate making PE the default, not the deprecation announcement.

---

## 3. User journey: today vs after

### 3.1 Today (bindings)

| Who | Does what |
|---|---|
| Cluster admin | `helm upgrade --set ngrok.features.bindings.enabled=true` (+ optional `ngrok.features.bindings.endpointSelectors`, default `["true"]`; `serviceLabels`/`serviceAnnotations`). Operator registers a KubernetesOperator with the `bindings` feature and gets an mTLS cert via CSR (`internal/controller/ngrok/kubernetesoperator_controller.go:174-235`) |
| Endpoint owner | `ngrok http 80 --binding kubernetes --url http://my-svc.my-ns`. Scheme defaults to `https` and port to 80/443 (`bindingendpoint_aggregator.go:17-22`); tcp needs a port |
| Operator | Poll every 10s (`cmd/api-manager.go:838`) → `BoundEndpoint` named `hashURL(url)` (`boundendpoint_poller.go:641`) in the operator namespace → upstream ClusterIP Service of the same name selecting the bindings forwarders, port → allocated forwarder port (`boundendpoint_controller.go:449-470`) → target **ExternalName** Service `my-svc` in `my-ns` → `<hash>.<operator-ns>.svc.<clusterDomain>` (`:400, :421-441`) |
| App | `curl http://my-svc.my-ns` (search path → `my-svc.my-ns.svc.cluster.local` → CNAME → upstream ClusterIP) |

### 3.2 After (private endpoints)

| Who | Does what |
|---|---|
| Cluster admin | `ngrok.features.privateEndpoints.enabled=true`, a token (`ngrok.credentials.privateEndpointForwarder.accessToken` or shared), UDP/443 egress, and **one-time CoreDNS rewrite** for `*.ngrok.direct` (D4: docs, operator doesn't edit DNS). `ngrok.features.privateEndpoints.includeInternal` only if they want `.internal` (D8) |
| Endpoint owner | `ngrok http 80 --url https://my-svc.ngrok.direct` (private binding). No per-cluster opt-in |
| Operator | `PrivateEndpoint` CR + Service `my-svc-ngrok-direct` in the operator namespace |
| App | `curl https://my-svc.ngrok.direct`, cert verifies |

### 3.3 What changes for an app

| Item | Before | After | Action |
|---|---|---|---|
| Hostname | `my-svc.my-ns` (or FQDN `my-svc.my-ns.svc.cluster.local`) | `my-svc.ngrok.direct` | App config/env change, unless the alias shim is used |
| Scheme / TLS | Usually `http://`, or https without a matching cert | `https://` with a verifiable cert is now possible | Optional; turn verification on |
| Port | Endpoint port | Same | None |
| Name resolution | Kubernetes Service DNS | CoreDNS rewrite (admin) | Admin, once per cluster |
| Namespace | Name lives in app namespace | Name is cluster-global | NetworkPolicy targeting the old ExternalName target changes: traffic now goes to forwarders in the operator namespace (same as before, really, since ExternalName resolved there too) |
| Tokens | None in forwarder (mTLS cert) | PAT in forwarder | Admin; scope the token if ngrok supports it (open Q) |
| Operator RBAC | Cluster-wide Service writes, Pod reads | Namespace-scoped | Gets smaller |
| Traffic Policy on the endpoint | Can match on caller pod (P2) | Can't yet | Blocked on GAT-475 |
| Egress | TCP/443 | UDP/443 | Firewall change, until HTTP/2 fallback |

---

## 4. Phased plan (A: bindings → private endpoints)

No dates. Each phase has entry and exit criteria. Phases line up with
[strategy.md §10](strategy.md#10-phased-path-forward) (its Phase 4 is this doc).

### M0. Coexist (now)

Both features can run at once; they select disjoint endpoints (`poller.go:141` drops
`kubernetes`-bound; bindings parses only `svc.ns` URLs, `bindingendpoint_aggregator.go:143`).

- Entry: POC (done).
- Exit: PE preview docs out (strategy Phase 1); bindings usage measured from KO feature
  registrations (count, endpoints per cluster, selector use, scheme mix); migration guide drafted.

### M1. Opt-in migration

Users move endpoint by endpoint. Cutover for one endpoint, zero downtime:

1. Endpoint owner starts a **second** endpoint, `https://my-svc.ngrok.direct`, on the same
   upstream. Keep the old `kubernetes`-bound `http://my-svc.my-ns` running. (A URL can't be both
   shapes, so it really is two endpoints. Whether the old one can be a cloud endpoint that
   `forward-internal`s to the new one: **unverified**.)
2. Wait for the `PrivateEndpoint` to be Ready; check from a pod.
3. Switch app config to the new hostname (or create the alias, §4.2), roll the app.
4. Stop the old endpoint. Its `BoundEndpoint` and Services go away within a poll.

- Entry: M0 exit; PE preview supported on the user's platform.
- Exit: blocking gaps P1–P4 closed (or waived by product); guide tested on kind + EKS; alias
  shim documented.

### M2. Private endpoints are the default

- Docs and Helm NOTES lead with private endpoints. `templates/NOTES.txt:69-77` currently
  *advertises* bindings when it is disabled; change it to point at private endpoints.
- `ngrok.features.privateEndpoints.enabled` default stays a product call (it needs the DNS step, so "on by
  default" may never make sense).
- Entry: P1–P4 closed; P9 (cleanup) fixed; SDK off the feature-branch pin; ≥1 GA platform guide.
- Exit: no new installs enabling bindings for N releases (telemetry).

### M3. Deprecate

- Helm: warning in NOTES and a deprecation comment on `bindings.*` values.
- Operator: log at startup when `--features-bindings-enabled`; a `Deprecated` condition or Warning
  event on each `BoundEndpoint`.
- Backend (needs ngrok): warn when creating an endpoint with the `kubernetes` binding (CLI,
  dashboard, API).
- Entry: M2 + announcement window agreed with product.
- Exit: active KOs with the `bindings` feature ≈ 0, or a product-set floor.

### M4. Remove

- Delete `api/bindings/`, `internal/controller/bindings/`, `cmd/bindings-forwarder-manager.go`,
  `internal/ngrokapi/bindingendpoint_aggregator.go`, Helm `bindings.*`, `bindings-forwarder/`
  templates, the bindings ClusterRole, and the KO `binding` spec field + CSR/cert renewal path.
- **Keep `pkg/bindingsdriver/`**: the PE forwarder reuses its listeners (README "What's built").
  Rename/move it then.
- Removal order matters: `BoundEndpoint`s carry finalizers. Ship one release where the
  controller only strips finalizers and deletes projected Services, *then* remove code and the
  CRD. Same pattern as the repo's existing `LEGACY-*` cleanup releases.
- Backend: retire the `kubernetes` binding and KO binding certs (ngrok side).

### 4.2 Compat shim: keep `<svc>.<ns>` names?

| Option | How | Pros | Cons |
|---|---|---|---|
| S0. None | Apps change hostnames | Zero code; real names everywhere | Every consuming app changes config in the same window |
| **S1. Docs-only alias (recommended)** | User creates `kubectl create service externalname my-svc -n my-ns --external-name my-svc-ngrok-direct.<operator-ns>.svc.cluster.local` | Same mechanism bindings uses today (ExternalName → ClusterIP). **Needs no CoreDNS rewrite**, so it also unblocks platforms where the DNS step isn't possible yet. Users own it with their own namespace RBAC | Makes the per-host Service name (`internal/privateendpoints/naming.go`) a **public contract**, which D2 (hash names) would break. https via the alias fails cert verification (same as bindings today). Plain-http Host header becomes `my-svc.my-ns`: OK if ngrok routes private dial purely by dialed `host:port` (**unverified**) |
| S2. Operator-managed alias | Namespaced object or annotation ("expose `foo.ngrok.direct` as `my-svc` in this namespace"); operator creates the ExternalName | Survives D2 renames; namespace owners opt in; could carry labels/annotations (P6) | New API surface. Writing into app namespaces brings back cross-namespace Service RBAC (P11) |
| S3. Auto-project `kubernetes`-bound endpoints through private dial | — | Zero user action | Not possible: `kubernetes`-bound `svc.ns` endpoints aren't private-dialable. Needs backend work. Rejected |

**Recommend S1 for migration**, with two commitments: (a) the per-host Service name stays
stable for as long as S1 is documented, or we provide a stable alias target; (b) revisit S2
as a product feature (namespace-local names, no DNS step) alongside D1, not as a migration shim.

---

## 5. Migration B: operator-generated `*.internal` endpoints → `*.ngrok.direct`

### 5.1 What the code does today

| Path | When an internal AEP is created | Internal URL | AEP object name |
|---|---|---|---|
| Ingress / Gateway (`pkg/managerdriver`) | `endpoints-collapsed` (default, `internal/ir/ir.go:135`): only hostnames with >1 upstream. `endpoints-verbose`: always | `buildInternalEndpointURL` (`pkg/managerdriver/utils.go:64-117`): `<scheme>://<5 hex of sha256(Service UID)>-<svc>-<ns>[-mtls-<5hex>][-<clusterDomain>][-<port> for http/s].internal[:<port> for tcp/tls]`. Single label, dots replaced (`:109`), `.internal` appended at `:111` | `internalAgentEndpointName` (`utils.go:24`): deterministic and **TLD-independent** |
| LoadBalancer Service (`internal/controller/service`) | `endpoints-verbose` only; collapsed makes one public AEP, no internal | `tcp://<Service UID>.<name>.<ns>.internal:<port>` (`controller.go:584`): **multi-label** | `GenerateName: <svc>-internal-` (`:621`) |

- The CLEP routes with `forward-internal` to `agentEndpoint.Spec.URL`
  (`pkg/managerdriver/translator.go:536`, `service/controller.go:587`).
- Internal AEPs set **no `bindings`** (`translator.go:829-840`; Service path sets none either):
  the `.internal` TLD implies the internal binding.
- Generated endpoints carry `owned-by` metadata (`pkg/managerdriver/driver.go:199`), but users
  can override it.
- The agent driver rebinds make-before-break on any spec change: bind new, then close old
  (`pkg/agent/driver.go:271-330`). Per agent-manager replica.

### 5.2 Checking the PM sketch ("create new AEPs → repoint CLEP → delete old; URLs contain UUIDs and change, so low impact")

| Claim | Reality |
|---|---|
| "URLs contain UUIDs and change" | **Partly.** They're derived from the Service UID (5-hex hash for Ingress/Gateway, full UID for Service). They're stable for the life of the Service and change only on Service recreate. A user *could* have copied one into their own policy. Low but non-zero impact |
| "Create new AEPs first" | **Not how the code works.** The AEP object name doesn't include the TLD, so flipping the TLD updates the same object in place. The agent driver then binds the new URL and closes the old one, while the CLEP is updated separately by the api-manager's CloudEndpoint controller. No ordering between them, so there's a window (seconds) where the CLEP points at a URL that isn't bound |
| "Then update CLEP, then delete old" | Needs a **dual-emit window** (below) |
| "Low user impact" | Agreed for HTTP request traffic. Not for long-lived TCP/WebSocket: closing the old AEP likely drops in-flight streams (**unverified**). Service LB (tcp) users are the ones exposed |

Ways to get create-before-delete:

1. **Fallback in the CLEP policy (preferred if it works).** Emit both AEPs (new object name, e.g.
   suffixed). CLEP rule: `forward-internal` to the new URL with `on_error: continue`, then
   `forward-internal` to the old URL. One release later drop the old AEP and the fallback rule.
   Stateless, so it fits the translator. Needs: `forward-internal` supports `on_error`
   (**unverified**; `internal/trafficpolicy/policy.go:435` doesn't model it).
2. **Release-staged dual-emit.** Release N: emit both AEPs, CLEP still old. N+1: CLEP → new.
   N+2: drop old. Downgrade-safe, slow. Same shape as the repo's `LEGACY-*` dual-write releases
   (e.g. `service/controller.go:608-611`).
3. **Status-gated switch.** Translator switches the CLEP only once the new AEP is Ready. Correct,
   but makes the translator stateful. Avoid.
4. **Accept the blip.** In-place flip. Simple, but a seconds-long outage per hostname on upgrade.

Recommend (1), falling back to (2). Gate either behind a flag (e.g.
`--internal-endpoint-tld=internal|ngrok.direct`, default `internal` until the flip) so users
can opt in early and roll back.

### 5.3 Risks

- **Service LB names are multi-label.** `<uid>.<name>.<ns>.internal` can't become
  `.ngrok.direct` as is (PE only handles single labels, and multi-label `.ngrok.direct` support
  is an open Q, D6). Move the Service path onto `buildInternalEndpointURL`-style single-label
  names. Watch the 63-char label limit: `<5hex>-<svc>-<ns>-<port>` overflows for long names.
  Check what happens today with long names under `.internal` (`utils.go` does no truncation).
- **Name scope of `.ngrok.direct`.** If `.ngrok.direct` hostnames are unique across all ngrok
  accounts (not per account like `.internal`), deterministic names such as
  `abcde-web-default-80.ngrok.direct` collide between customers and leak service/namespace names.
  **Must confirm with backend before anything else.** If global, add an account/cluster-unique
  component (or wait for agent `baseDomain`, D6b).
- **Accidentally public.** Internal AEPs rely on the TLD to imply the internal binding. A
  `.ngrok.direct` AEP must set `bindings` explicitly (`internal` today, `private` after the
  rename). Test that omitting it fails closed. High severity if wrong.
- **R18 comes back by default.** D8 mirrors `.ngrok.direct` by default. Once generated AEPs are
  `.ngrok.direct`, every cluster with PE on mirrors them: hairpin traffic, forwarder ports used up,
  and other clusters in the account see this cluster's backends. Needs a reliable
  "operator-generated" marker the poller can skip. `owned-by` is user-overridable, so use a
  dedicated one. Don't filter user-authored AEPs (README R18).
- **Hand-written references.** Users who copied a generated `.internal` URL into their own
  CloudEndpoint, TrafficPolicy CR, or another account's policy break when the old AEP goes.
  Release notes + the dual-emit window. No way to detect it from the operator.
- **In-flight connections.** Closing the old AEP likely cuts open TCP/WebSocket streams. Keep the
  old AEP through the window; document a one-time reconnect.
- **Rollback.** Downgrading after the flip makes the old operator compute `.internal` again: an
  in-place flip back with the same blip, and orphaned suffixed AEP objects if (1)/(2) introduced
  new names. The flag gives a no-downgrade rollback. Test both directions in e2e.
- **Billing.** The dual-emit window doubles internal AEPs (× agent replicas). Product to confirm
  this is acceptable or not billed.
- **Multi-replica agents.** Each replica rebinds on its own; pooled endpoints mean the old URL
  lives until the last replica rebinds. Fine for (1)/(2), adds jitter for (4).

---

## 6. Risks and open questions for product

Risks (migration A):

- We deprecate on the wrong signal. Use KO feature telemetry, not guesses.
- The S1 alias quietly becomes a long-term API, blocking D2 (hash Service names).
- The forwarder PAT has account-wide private dial reach, broader than the bindings mTLS cert
  (P10). Security teams may push back.
- Users on clusters that block UDP or can't change DNS have **no** path off bindings until P3/P4
  are solved. Deprecating before then strands them.

Open questions:

1. **Who uses bindings?** Can product pull KOs with `features: bindings` (count, endpoint counts,
   selectors other than `"true"`, schemes)? This sets the urgency and whether M1–M3 can be short.
2. **Is identity (P2) used?** Any customer Traffic Policy matching on Kubernetes caller fields?
   If none, P2 drops out of the blocking set.
3. **Scoping model (P1).** What replaces `endpointSelectors` and per-cluster targeting: operator
   selectors, endpoint-side opt-out, Traffic Policy on identity? (Same as strategy.md Q2/Q3/Q5.)
4. **Backend timeline for the `kubernetes` binding.** Does ngrok want to retire it (and KO binding
   certs) on a date? That drives M4.
5. **Alias shim.** OK to document S1 and freeze the per-host Service name while it's documented?
   Or is namespace-local naming wanted as a feature (S2)?
6. **`.ngrok.direct` name scope.** Account-scoped or globally unique? Blocks migration B design.
7. **Multi-label `.ngrok.direct`.** Is `a.b.ngrok.direct` allowed? Affects Service LB naming and D6b.
8. **Migration B billing and timing.** Post-1.0 confirmed; is a dual-emit window of one release
   acceptable cost-wise?
9. **Token scoping.** Can the forwarder's credential be limited to private dial only?

---

## 7. TODOs

- [ ] MIG-1 Ask product/backend for bindings usage from KO feature registrations (Q1); record numbers here.
- [ ] MIG-2 Confirm with backend whether `.ngrok.direct` hostnames are account-scoped (Q6) and whether multi-label is allowed (Q7).
- [ ] MIG-3 Find out the Traffic Policy variable names bindings pod identity populates and whether any customer uses them (Q2).
- [ ] MIG-4 Verify in kind: S1 ExternalName alias to a per-host Service works for http, tcp and tls (no CoreDNS rewrite); http with a mismatched Host header still reaches the endpoint.
- [ ] MIG-5 Verify: can a `kubernetes`-bound endpoint be a cloud endpoint that `forward-internal`s to a private one (smoother M1 cutover)?
- [ ] MIG-6 Write the user migration guide (§3.3 + §4 M1 steps + S1), tested on kind and EKS.
- [ ] MIG-7 Decide whether to freeze `naming.go` Service names as a contract while S1 is documented; note it in design.md.
- [ ] MIG-8 Change `helm/ngrok-operator/templates/NOTES.txt:69-77` to stop advertising bindings (at M2).
- [ ] MIG-9 Add deprecation signals (Helm NOTES warning, startup log, BoundEndpoint condition/event) at M3.
- [ ] MIG-10 Plan the M4 removal release: finalizer-strip release first, keep and relocate `pkg/bindingsdriver`, drop KO `binding` field and CSR path.
- [ ] MIG-11 Verify `forward-internal` supports `on_error: continue` (decides option 1 vs 2 in §5.2).
- [ ] MIG-12 Verify a `.ngrok.direct` AgentEndpoint without explicit `bindings` does not come up public; make generated `.ngrok.direct` AEPs always set it.
- [ ] MIG-13 Design an operator-generated marker the PE poller skips (R18 at default), without filtering user-authored AEPs.
- [ ] MIG-14 Move the Service LB verbose path to single-label internal names; check label-length overflow for long svc/ns names today.
- [ ] MIG-15 Measure whether closing an AEP drops in-flight TCP/WebSocket streams.
- [ ] MIG-16 Prototype the TLD flag + dual-emit in managerdriver and the Service controller; e2e upgrade and downgrade.
