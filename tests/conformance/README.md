# Gateway API conformance

This directory runs the upstream [Gateway API conformance suite](https://gateway-api.sigs.k8s.io/concepts/conformance/)
against the operator and produces the report we submit to
[kubernetes-sigs/gateway-api](https://github.com/kubernetes-sigs/gateway-api/tree/main/conformance/reports).
A merged report is what keeps the operator on the
[implementations page](https://gateway-api.sigs.k8s.io/implementations/).

It is its own Go module so it can pin the suite's Gateway API version (see `go.mod`) without moving the
operator's.

## Running it

You need a kind cluster (the target creates one if `ngrok-operator` doesn't exist), Docker, and an ngrok
account whose API key is in `NGROK_ACCESS_TOKEN`. The account must be able to reserve `ngrok.app`
subdomains and custom domains, including wildcards.

```bash
make conformance            # install CRDs + operator, then run the suite
make conformance-setup      # only install the Gateway API CRDs and deploy the operator
make conformance-test       # only run the suite against what is deployed

# One test, while working on it:
make conformance-test CONFORMANCE_RUN_TEST=HTTPRouteSimpleSameNamespace
```

The report lands in `tests/conformance/reports/` (git-ignored). A full run takes about 30 minutes.

Settings, all optional (`tools/make/conformance.mk`):

| Variable | Default | Meaning |
|---|---|---|
| `CONFORMANCE_GATEWAY_API_VERSION` | `v1.7.0` | CRDs installed; keep in step with `go.mod` |
| `CONFORMANCE_PROFILES` | `GATEWAY-HTTP` | Profiles to run |
| `CONFORMANCE_SUPPORTED_FEATURES` | `Gateway,HTTPRoute,ReferenceGrant` | Features we claim; keep in step with `SupportedFeatures` in `internal/controller/gateway/gatewayclass_controller.go` |
| `CONFORMANCE_SKIP_TESTS` | none | Comma-separated tests to skip. Every skip must be explained in the submitted README |
| `CONFORMANCE_HOSTNAME_SUFFIX` | `<you>.gwapi-conformance.test` | See [the hostname suffix](#the-hostname-suffix) |
| `CONFORMANCE_OPERATOR_VERSION` | `git describe` | Version written into the report; a submitted report must name a release |

A run reserves real domains in the ngrok account. They use `reclaimPolicy: Delete`, so deleting the
suite's namespaces (it does this itself) releases them. Check the account afterwards when a run was
interrupted.

## How the operator is set up for the suite

The suite's Gateways have listeners without a hostname. ngrok routes by hostname, so the operator asks
ngrok to assign such a Gateway a domain (a random `*.ngrok.app` name, the way `ngrok http 80` gets one) and
publishes it in `status.addresses`. That is ordinary product behavior with no setting; see "The assigned
domain" in [specs/features/gateway-api.md](../../specs/features/gateway-api.md).

One operator setting is set by `make conformance-setup`:
`ngrok.features.gateway.conformanceHostnameSuffix=<suffix>`. Test only, explained next.

### The hostname suffix

The suite sends requests for hosts such as `example.com` and `foo.wildcard.io`. Other ngrok accounts
already hold many of those names, and the ngrok edge routes a request by its `Host`, so a request for
`example.com` can never reach our endpoints.

To get around it, the operator under test appends a suffix to every hostname it reads from a Gateway or
route (`example.com` becomes `example.com.alex.gwapi-conformance.test`) and reserves those names instead.
The suite's HTTP client (`hostmap.go`) appends the same suffix to the `Host` of each request, and removes
it from the parts of the response the suite inspects: the `Host` the backend saw and the host of a
redirect.

What this does and does not change:

- **Unchanged:** the endpoints, their traffic policy, and how the edge routes. Each hostname still gets
  its own ngrok endpoint, and the edge still picks it by `Host`. Appending a suffix keeps every relation
  the suite tests: `*.example.com` still covers `foo.example.com`, and `example.com` still doesn't.
- **Changed:** only the names. Real users use domains they own, so they never need this.
- `.test` is reserved by [RFC 2606](https://www.rfc-editor.org/rfc/rfc2606) and never resolves, so the
  reserved names can't belong to anyone. Nothing needs DNS: requests go to the Gateway's address with
  the suffixed `Host`.

The report for lexfrei's [Cloudflare Tunnel Gateway Controller](https://github.com/kubernetes-sigs/gateway-api/pull/4945)
(a community project) was accepted with the same kind of accommodation, for the same reason (TLS and routing at an edge in front
of a tunnel). We disclose ours in the submitted README.

## Submitting a report

1. Release the operator. A report must name a released version.
2. Run the suite against that release with `CONFORMANCE_OPERATOR_VERSION=<version>`.
3. Copy the report, unmodified, to `conformance/reports/<gateway api minor>/ngrok-ngrok-operator/` in a
   fork of kubernetes-sigs/gateway-api, with the README from `submission/` (version and results filled
   in) and `conformance/list/implementations/ngrok-ngrok-operator/details.yaml`.
4. Open the PR with `submission/PR-body.md`.

See [specs/gateway-api-conformance.md](../../specs/gateway-api-conformance.md) for the rules the report
must meet and the state of each test.
