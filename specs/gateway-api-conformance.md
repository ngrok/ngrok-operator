# Gateway API conformance

Internal plan and status for passing the upstream Gateway API conformance suite and getting the
operator back on the [implementations page](https://gateway-api.sigs.k8s.io/implementations/).
Tracking issue: [K8SOP-331](https://linear.app/ngrok/issue/K8SOP-331). How to run the suite:
[tests/conformance/README.md](../tests/conformance/README.md).

## Why

The implementations page only lists projects with a recent conformance report on file. ngrok was
listed from 2024 ([#2852](https://github.com/kubernetes-sigs/gateway-api/pull/2852),
[#3775](https://github.com/kubernetes-sigs/gateway-api/pull/3775)) without ever submitting a report,
and was removed in the v1.5 review ([#4741](https://github.com/kubernetes-sigs/gateway-api/pull/4741)).
A merged report puts it back.

## The rules

From [conformance/reports/README.md](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/reports/README.md)
and [the conformance levels page](https://github.com/kubernetes-sigs/gateway-api/blob/main/site/content/en/docs/conformance/levels.md):

- A report names a profile (we target `GATEWAY-HTTP`), a Gateway API version and channel (`standard`),
  a mode (`default`) and an implementation version that is a snapshot, not a branch.
- **Success** means every core test of the profile, and every test of each extended feature claimed,
  passed. **Partial** means some were skipped, *or the run "required some steps unexpected by the
  suite"*. Partial reports are listed separately.
- Partial conformance is deprecated and is removed in v1.8. v1.7 caps a report at 5 skipped tests.
- Only reports for the current or previous Gateway API minor count. v1.7.0 shipped on 2026-10-29,
  so a v1.7 report is the target; v1.8 is expected around Feb-Mar 2027.
- `details.yaml` (the list entry) must not mention versions or commercial terms.

The closest precedent is the Cloudflare Tunnel controller ([#4945](https://github.com/kubernetes-sigs/gateway-api/pull/4945),
merged as partial): an edge in front of a tunnel, like us. It ran the suite with a test-only header
that carries the hostname the suite wants, and skipped `HTTPRouteHTTPSListener`.

## Where we started

On `main` at `ebd3e85f`, the unmodified suite (v1.6.3 and v1.7.0) failed in setup: 0 of 40 core
tests ran. With its Gateways patched to have hostnames, 2 of 40 passed and no test reached traffic.
Every failure was status: listeners had no `ResolvedRefs`, empty `supportedKinds`, `attachedRoutes`
always 0, stale `observedGeneration`, routes had no `ResolvedRefs`.

## What changed in the operator

| Change | Why | Where |
|---|---|---|
| Gateway, listener and HTTPRoute status worked out from the whole store and written by the Driver at the end of each Sync, keeping other controllers' conditions and parents | The controllers wrote partial, competing status; listener and route status depend on each other | `pkg/managerdriver/gatewayapi_status.go`, `driver.go` |
| GatewayClass `Accepted` follows the generation; `supportedFeatures` published | Accepted was never updated after the first time | `gatewayclass_controller.go` |
| Endpoint hostnames are the intersection of listener and route hostnames | A route for `foo.example.com` under listener `*.example.com` served every subdomain | `translate_gatewayapi.go`, `gateway_hostnames.go` |
| Matches ordered by the Gateway API precedence rules | Header matches were ordered alphabetically; route age was ignored | `internal/ir/ir.go` |
| Unresolved backendRefs, and rules with neither backends nor filters, answer 500 | They were dropped, so requests fell through to other rules or a 404 | `translate_gatewayapi.go` |
| A Gateway listener without a hostname gets a generated domain when `features.gateway.generatedDomainSuffix` is set | The suite's Gateways have none; ngrok needs one | `gateway_hostnames.go`, chart value |
| A hostname is only collapsed into a public AgentEndpoint when the Service is in the virtual host's namespace | Otherwise the hostname's Domain was also created in the Service's namespace, and deleting it released a hostname still in use | `translator.go` |
| A change to a Service, Secret, ReferenceGrant and the other store-fed kinds re-syncs | Objects created after the route that uses them were never picked up | `controller-handler.go`, `driver.go` |
| A Gateway reconcile loads the Secrets and ConfigMaps it references into the store | Those watches drop objects no Gateway referenced yet, so a certificate created before its Gateway was never seen | `gateway_controller.go` |
| `PathPrefix` matches whole path segments | `/v2` also matched `/v2example` | `translator.go` (`segmentPrefixExpression`), `internal/ir` |
| Internal endpoint hostnames are cut to 63 characters with a hash | Long Service and namespace names gave hostnames ngrok rejects (`ERR_NGROK_396`), retried forever | `utils.go` (`buildInternalEndpointURL`) |
| Endpoint names cut for length no longer end in `.` before the hash | `...secure.-bddd2d2c` is not a valid name | `utils.go` (`sanitizeStringForK8sName`) |
| An endpoint the API server rejects as invalid is skipped instead of failing the Sync | One bad object stopped every endpoint and status update for all Ingresses and Gateways | `endpoints.go` |
| A headless Service backend is dialed on its numeric `targetPort` | A headless Service has no cluster IP to translate the port, so its Pods were dialed on the Service port (`ERR_NGROK_3004`) | `utils.go` (`upstreamPort`) |
| A URL still held by the endpoint being replaced (`ERR_NGROK_18016/18017` for CloudEndpoints, `ERR_NGROK_334` for AgentEndpoints) is retried every 2s | These conflicts clear in seconds, but the growing error backoff kept the URL offline for minutes | `cloudendpoint_controller.go`, `agent_endpoint_controller.go` |
| A CloudEndpoint deleted right after it was created re-reads its ID from the API server before releasing its finalizer | The cache did not have the ID yet, so the ngrok endpoint was left behind, holding its URL and blocking its replacement | `cloudendpoint_controller.go` (`freshID`) |
| A header filter with only `add` or only `remove` no longer emits an empty action | ngrok rejects an empty header list (`ERR_NGROK_2204`), which kept the whole endpoint offline | `translate_gatewayapi.go` (`headerActions`) |
| Test-only `features.gateway.conformanceHostnameSuffix` | See below | `gateway_hostnames.go`, chart value |

## The hostname suffix

The suite sends requests for hostnames like `example.com`, which other ngrok accounts hold. With
`conformanceHostnameSuffix` set, the operator appends a suffix to every hostname it reads from a
Gateway or route, and the harness's HTTP client appends it to each request's `Host` and strips it
from what the suite inspects. Each hostname still has its own endpoint and the edge still routes by
`Host`; only the names change. The suffix is under `.test` (RFC 2606), so it can never be someone's
domain. Details in [tests/conformance/README.md](../tests/conformance/README.md#the-hostname-suffix).

The harness also creates one CloudEndpoint, `http://*.<suffix>`, that answers 404. The suite expects
a 404 for a hostname no route serves; without it the edge redirects a plain-HTTP request for an
unknown hostname to HTTPS.

Risk: a reviewer may count either as "steps unexpected by the suite", which makes the report partial
even with no skips. We disclose both in the submitted README.

## Account requirements

The run reserves real domains: the generated `*.ngrok.app` names, custom domains under the suffix,
and 6 wildcard domains (`*.wildcard.org`, `*.wildcard.io`, `*.anotherwildcard.io`, `*.bar.com`,
`*.foo.com`, and the catch-all). Accounts are limited to 5 wildcard domains by default
(`ERR_NGROK_418`), so runs need an account with a higher limit; a dedicated test account also keeps
runs from touching anyone's domains.

## Test status

Core GATEWAY-HTTP tests, Gateway API v1.7.0:

| Run | Passed | Notes |
|---|---|---|
| `main` (unmodified suite) | 0 / 40 | Setup fails |
| This branch, run 3 | 27 / 40 | Before the collapse, resync and catch-all fixes |
| This branch, run 4 | see the PR | |

Expected skip: `HTTPRouteHTTPSListener`. TLS terminates at the edge, and the test's client sends the
SNI of an unsuffixed test hostname (`example.org`), so the edge can't route it. This skip makes the
report partial. Upstream already has an open issue asking to split an HTTPS-listener-dependent test out
for edge- and proxy-fronted implementations, for BackendTLSPolicy
([#5103](https://github.com/kubernetes-sigs/gateway-api/issues/5103)); there is none yet for
`HTTPRouteHTTPSListener`, and we should open one.

## Open items

- **v1.8:** with partial removed, `HTTPRouteHTTPSListener` must pass or leave core, and the hostname
  suffix must be acceptable to reviewers. Raise both upstream before v1.8 (an issue like #5103 for
  this test).
- **CloudEndpoint leak:** before the `freshID` fix, four runs left a cloud endpoint in the ngrok account
  after its CloudEndpoint was gone. One of them held the URL of a Gateway the next tests used, so its
  replacement could not come online and every later test on that Gateway failed. The fix covers a
  CloudEndpoint deleted before the cache saw its ID. It cannot cover one whose status write never
  happened at all; that would need a lookup of remote endpoints by URL.
- **Endpoint type changes:** under the default mapping strategy a hostname with a single backend
  Service becomes one public AgentEndpoint, and with more it becomes a CloudEndpoint. Each change
  replaces the endpoint holding the URL, which gives a gap of a few seconds. The suite changes routes on
  the same Gateway constantly, so it hits this often. Users who change routes see the same gap.
- **TLS and TCP profiles:** the standard channel ships TLSRoute and TCPRoute as `v1` since v1.6, and
  the operator only looks for `v1alpha2`, so it disables them on current CRDs. Supporting them needs
  the gateway-api module bump (client-go 0.37, controller-runtime 0.25).
- **Extended features:** none claimed yet. Redirects, rewrites, header and query matching are
  implemented and are the next candidates.
