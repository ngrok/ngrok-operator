# POC plan and verified findings

## Headline finding (verified in the mono repo)

**Private dial cannot resolve kubernetes-bound endpoints today.** This is a design fact, not a bug, and it reshapes the migration.

### Evidence

- `/dial` **hardcodes the internal binding** when resolving a host:
  `go/svc/mux/privatedial/handler.go:826-827`
  ```go
  hpb := hostport.NewHostPortBinding(dreq.Host, dreq.Port, hostport.NewInternalBinding(sess.accountID))
  eps, err := h.endpointMap.Get(hpb.Host, hpb.Port, hpb.Binding.ToResolvedBinding(sess.accountID))
  ```
- The muxmap indexes endpoints by `(host, port, binding)` — binding is part of the key:
  `go/svc/mux/muxmap/map.go:438-440` (`Key`), `:180-188` (`Get`).
- Kubernetes-bound endpoints are indexed under a **different** binding: `NewKubernetesOperatorBinding(accountID)` (`go/svc/mux/mux_k8sop.go`, `go/lib/bindings/computation.go:44-48`).
- So an internal-binding lookup for `foo.myns:8080` never matches the kubernetes-binding entry for the same host:port. No fallback exists.

### Corollaries (also verified)

- **Auth is PAT-only, account-scoped.** `handler.go:1072-1124` rejects non-`ngrok_pat_*` tokens, introspects via IAM, returns the account KSUID. No mTLS.
- **Kubernetes endpoints are account-scoped**, unique by `(host, port, binding)` — not owned by a specific operator. Two clusters projecting `foo.myns` resolve **one** account endpoint. → account identity (PAT) is sufficient for resolution; the operator's identity was never what picked the endpoint. (This corrected an earlier worry that PAT couldn't disambiguate across clusters — it can, because there's nothing to disambiguate.)
- **`DialReq.metadata` is defined but never read** (`handler.go:811-814`; `proto/lib/private_dial/private_dial.proto:74-84`). Pod identity (`conn.k8s.pod.*`) would have to be plumbed through it — that's GAT-475.

### What this means

The premise "keep the `kubernetes` binding, just swap the datapath to private dial" is **structurally blocked**: the binding you'd keep is exactly what hides the endpoint from the private-dial resolver. To be dialable over private dial, the endpoint must live in the internal/private index — which is what consolidating `kubernetes` into `private` *means* at the mux layer. The datapath swap and the consolidation are coupled.

### Unblock options (both are small-ish backend changes)

1. **Teach `/dial` to also resolve the kubernetes-operator binding** (query both indexes / merge). Minimal; keeps two binding types alive. Note it also loosens dial auth from per-operator-cert to per-account-PAT — which is the intended private-endpoints posture ("account boundary is the access-control layer"), not a regression.
2. **Make the endpoints `private`-bound** (do the consolidation). Cleaner long-term; endpoints land in the internal/private index and private dial resolves them for free. Re-opens the projection-discriminator decision ([04](04-design-options.md)).

This is the question for Euan / Gateway (see [README](README.md)).

## POC plan

### Minimal datapath POC — unblocked, do this first

Prove the datapath with **no backend change**: the operator's bindings-forwarder opens a private-dial session and relays TCP to a plain **`.internal` endpoint** (already in the internal index, so it resolves today).

- Pull the ngrok-go dialer branch via `go.mod replace` ([PR #245](https://github.com/ngrok/ngrok-go/pull/245)).
- In `forwarder_controller.go`, replace the mTLS leg (`tls.Dial(ingressEndpoint)` + `mux.UpgradeToBindingConnection`, ~lines 229-260) with the private dialer's `DialContext(ctx, "tcp", host+":"+port)`.
- Point it at a hand-created `.internal` endpoint with a known upstream. Skip the poller/projection — hand-wire the target for the POC.
- Auth: any account authtoken/PAT via env/secret. Ignore the mTLS cert path.
- Skip pod identity.

**Proves:** the branch works, `/session` + `/dial` + raw relay work from inside the operator, and the mTLS leg can be cleanly replaced. Independent of the kubernetes-endpoint resolution question.

### Full POC — after Euan answers

Same code, pointed at a `service.namespace` endpoint. Requires unblock option 1 or 2 above, i.e. a mono devenv.

## Blockers / dependencies

| Item | POC blocker? | Notes |
|---|---|---|
| Private dial can't resolve kubernetes endpoints | For full POC: **yes** | Minimal POC uses a `.internal` endpoint, so no. |
| ngrok-go dialer branch maturity | No | Rough but builds; `go.mod replace`. Proper release = GAT-361. |
| Dial ingress reachable | No | `connect-endpoint.dev-ngrok.com`; confirm from POC env. |
| PAT/authtoken | No | Any account credential. ADMIN-675/685 are for productization. |
| Pod identity / `conn.k8s.pod.*` (GAT-475) | No | Skip for POC. Mandatory for productization or those policies silently regress. |
| Poller / BoundEndpoint / Services | No | Unchanged; leave as-is (K8SOP-292 scope). |

## Scope note (K8SOP-292 as written)

K8SOP-292 scopes the datapath swap as: replace the forwarder's mTLS dial + `UpgradeToBindingConnection` with the private dialer; move pod identity into `DialReq.metadata`; keep the poller, `BoundEndpoint` CRDs, Service creation, and the per-port listener as-is. K8SOP-292 **blocks** K8SOP-293 ("retire the kubernetes binding"), which is where the [gap](03-the-gap.md) has to be answered.
