# References

Everything referenced across these notes. Migrated from the Notion "Private Dial Notes" page.

## Product / design docs

- **Private Endpoints — Product 1-Pager** (binding consolidation): https://linear.app/ngrok/document/private-endpoints-product-1-pager-20654e9d0ce9
- **Private Dial initiative**: https://linear.app/ngrok/initiative/private-dial-4773dcadb830/overview
- **Private Endpoints initiative**: https://linear.app/ngrok/initiative/private-endpoints-ba7aca22c68a/overview
- **Private Dial RFC** (Gateway, accepted): https://github.com/ngrok-private/wiki/blob/main/rfcs/2026-04-29-private-dial.md
- **2024 bindings RFC** (source/name/sink model, muxmap keyed by binding): https://github.com/ngrok-private/wiki/blob/main/rfcs/2024-03-25-binding-endpoints-and-domains.md
- **ngrok Agent — Private Dial spec** (curl/bind, M2 DNS+TUN): https://github.com/ngrok-private/product/blob/main/specs/private-dial/ngrok-agent-private-dial.md
- **ngrokd** (existing private-dial-over-k8s-bindings prototype): https://github.com/ngrok-oss/ngrokd-go · https://ngrokd.ngrok.app/

## Linear tickets

| Ticket | Title | Relevance |
|---|---|---|
| K8SOP-292 | Migrate bindings-forwarder data path from mTLS to private dial | The datapath swap. Blocked on SDK + GAT-475. |
| K8SOP-293 | Retire the kubernetes binding | Where the projection gap must be answered. Blocked by K8SOP-292. |
| K8SOP-261 | Bindings forwarder per-request TLS dial limits throughput (~35-45 RPS) | Perf driver for K8SOP-292 (target 500-5000 RPS). |
| GAT-361 | Release private dial SDK | Blocks K8SOP-292. |
| GAT-475 | Expose `DialReq.metadata` in Traffic Policy CEL (+ alias k8s pod vars) | Blocks K8SOP-292; without it `conn.k8s.pod.*` regresses. |
| GAT-675 | Private dial endpoints cannot be pinned to a region | Popsink; unsolved for private dial. |
| INF-2153 | Support region specification in the kubernetes bindings ingress | Stopgap region pin on the old datapath. |
| ADMIN-675 | Kubernetes operator PAT template | PAT issuance for productization. |
| ADMIN-685 | Register IAM client config for the operator | PAT/IAM for productization. |

## Slack threads

- **Migration LOE** ("what the robot thinks it'll take", + Benjamin's ngrok-go branch): https://ngrok.slack.com/archives/C07K9EDUTTK/p1778706808111819
- **Alex's options post** (the public writeup of the field/resource/convention/DNS options, cc Euan/BMG/Stacks): https://ngrok.slack.com/archives/C07K9EDUTTK/p1788909819619689

## Code — ngrok-operator (this repo)

| Path | What |
|---|---|
| `api/bindings/v1alpha1/boundendpoint_types.go` | `BoundEndpoint` CRD; `service.namespace` URL regex (~L41). |
| `internal/controller/bindings/boundendpoint_poller.go` | Polls `GetBoundEndpoints(koId)`; builds `BoundEndpoint`s. |
| `internal/ngrokapi/bindingendpoint_aggregator.go` | Parses `service.namespace` out of the URL. |
| `internal/controller/bindings/boundendpoint_controller.go` | Creates the Target (ExternalName) + Upstream (ClusterIP) Services. |
| `internal/controller/bindings/forwarder_controller.go` | mTLS dial + `UpgradeToBindingConnection` (~L229-260). The leg K8SOP-292 replaces. |
| `internal/mux/header.go` | `UpgradeToBindingConnection` / `ConnRequest` header (~L70). |
| `pkg/bindingsdriver/driver.go` | Per-port TCP listener manager. |
| `api/ngrok/v1alpha1/kubernetesoperator_types.go` | `KubernetesOperator` CRD; `Binding{EndpointSelectors, TlsSecretName, IngressEndpoint}`. |
| `cmd/api-manager.go` | `--bindings-endpoint-selectors` default `["true"]` (~L171). |
| `cmd/bindings-forwarder-manager.go` | Forwarder manager entrypoint. |
| `specs/mapping-strategy.md` | `endpoints-verbose` already emits `svc.ns.internal` outbound (~L56). |

## Code — ngrok mono repo (`~/code/ngrok`)

| Path | What |
|---|---|
| `go/svc/mux/privatedial/handler.go` | Private dial `/session` + `/dial`. **Hardcodes internal binding at L826-827.** PAT auth at L1072-1124. `DialReq.metadata` unread at L811-814. Also `/get-host` (L918-957, name-only existence → DNS A/NXDOMAIN) and `/get-hostport` (L963-1024) — the live-resolution API an operator-DNS would query. |
| `local/ngrok/go/svc/ship/core/privatedial/privatedial.go` | Uses `golang.ngrok.com/ngrok/privatedial` (imported `ngrokdial`, L14): portable `Dialer.DialContext(ctx,"tcp","host:port")`. The reusable relay for the POC / forwarder / any DaemonSet. |
| `go/svc/mux/muxmap/map.go` | Endpoint index keyed by `(host, port, binding)` (`Key` ~L438-440, `Get` ~L180-188). |
| `go/svc/mux/mux_k8sop.go` | mTLS kubernetes binding ingress (`muxKubernetesBinding` ~L97-144); account id from cert `DNSNames[0]`; queries `NewKubernetesOperatorBinding`. |
| `go/lib/bindings/computation.go` | Binding computation; kubernetes binding is account-scoped (~L44-48). |
| `proto/lib/private_dial/private_dial.proto` | `DialReq{host, port, metadata, session_req}` (~L74-84). |
| `proto/rpx/model_endpoint.proto` | `rpx.Endpoint` has a single `binding` / `resolved_binding` (~L146-147). |

## ngrok-go

- **Private dialer branch / PR #245** (Benjamin, "builds, barely tested"): https://github.com/ngrok/ngrok-go/pull/245

## Public docs

- Kubernetes-bound endpoints & selectors: https://ngrok.com/docs/k8s/guides/bindings/
- Internal endpoints: https://ngrok.com/docs/universal-gateway/internal-endpoints/
- Kubernetes agent endpoints: https://ngrok.com/docs/universal-gateway/kubernetes-endpoints
- Local projection with bindings (blog): https://ngrok.com/blog/kubernetes-dev-loop-projection

## Origin

- Notion "Private Dial Notes" (now superseded by these docs): https://app.notion.com/p/ngrok/Private-Dial-Notes-3d1d19ffb6548049a4d4e61b0c9916e1
