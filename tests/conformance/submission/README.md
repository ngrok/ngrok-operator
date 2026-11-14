# ngrok Kubernetes Operator

A Kubernetes controller that implements the Gateway API on top of ngrok's global network. It watches
Gateway and HTTPRoute resources and programs ngrok endpoints through the ngrok API, with the cluster
connecting outbound only.

## Table of Contents

| API channel | Implementation version | Mode | Report |
| --- | --- | --- | --- |
| standard | [VERSION](https://github.com/ngrok/ngrok-operator/releases/tag/ngrok-operator-VERSION) | default | [standard-VERSION-default-report.yaml](./standard-VERSION-default-report.yaml) |

## Reproduce

You need Docker, [kind](https://kind.sigs.k8s.io/), Go, and an ngrok account whose API key is in
`NGROK_ACCESS_TOKEN`. The run reserves domains in that account: generated `ngrok.app` names, custom
domains under a `.test` suffix, and six wildcard domains, so the account needs a wildcard-domain limit
of at least six.

```bash
git clone https://github.com/ngrok/ngrok-operator.git
cd ngrok-operator
git checkout ngrok-operator-VERSION
export NGROK_ACCESS_TOKEN=...   # an ngrok API key for a test account
make conformance CONFORMANCE_OPERATOR_VERSION=VERSION
```

`make conformance` creates a kind cluster, installs the standard-channel Gateway API vGATEWAY_API_VERSION
CRDs, builds and installs the operator, and runs the suite. The report is written to
`tests/conformance/reports/`. See
[tests/conformance/README.md](https://github.com/ngrok/ngrok-operator/blob/ngrok-operator-VERSION/tests/conformance/README.md).

## Notes

ngrok terminates TLS and routes requests at its edge, in front of an outbound tunnel from the
cluster, and routes by hostname. Hostnames are global across ngrok accounts. Running the suite needs
three accommodations, and one test is skipped because of the TLS model:

- **Hostnames.** The suite's hostnames (`example.com`, `foo.wildcard.io`, ...) are held by other ngrok
  accounts, so the edge can't route them to us. The operator runs with a test-only setting that appends
  a suffix to every hostname it reads from a Gateway or route (`example.com` becomes
  `example.com.<suffix>.test`), and the suite runs with a round tripper that appends the same suffix to
  each request's `Host` and removes it from the `Host` the backend echoes and from redirect locations.
  Each hostname still gets its own ngrok endpoint and the edge still routes by `Host`; only the names
  differ, and `.test` names can't belong to anyone. This is the same accommodation as the
  `X-Original-Host` header in the report for lexfrei's Cloudflare Tunnel Gateway Controller.
- **Unknown hostnames.** For a hostname with no endpoint, the edge redirects plain HTTP to HTTPS. The
  harness creates one endpoint for `*.<suffix>.test` that answers 404, which the suite expects for a
  hostname no route serves. More specific endpoints take precedence.
- **Listeners without a hostname.** ngrok routes by hostname, so a Gateway whose listener has no
  hostname gets a domain ngrok assigns it (a random `*.ngrok.app` name), published in `status.addresses`
  with type `Hostname`. This is the operator's normal behavior, not a test setting.
- **Skipped: `HTTPRouteHTTPSListener`.** TLS is terminated at ngrok's edge, which selects the endpoint
  by SNI. The test's client sends the SNI of the suite's own hostnames (`example.org`), which the edge
  can't route to this account.
