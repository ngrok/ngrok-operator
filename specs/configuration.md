# Configuration

> CLI commands, flags, and runtime configuration for the operator's three binaries.

<!-- Last updated: 2026-04-08 -->

## Overview

The operator binary (`ngrok-operator`) exposes three subcommands, each corresponding to a distinct deployment. Configuration is primarily driven by CLI flags, which are set via the Helm chart's `values.yaml`.

## Binaries

### api-manager

The main control-plane binary. Runs the majority of controllers and the Manager Driver.

**Key flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--release-name` | — | Helm release name |
| `--metrics-bind-address` | `:8080` | Prometheus metrics endpoint |
| `--health-probe-bind-address` | `:8081` | Health probe endpoint |
| `--election-id` | — | Leader election ConfigMap name |
| `--server-addr` | — | ngrok server address |
| `--api-url` | — | ngrok API URL |
| `--ingress-controller-name` | — | Controller name for Ingress matching |
| `--ingress-watch-namespace` | — | Namespace to watch for Ingresses (empty = all) |
| `--manager-name` | — | Manager identifier |
| `--cluster-domain` | — | Kubernetes cluster domain |
| `--one-click-demo-mode` | `false` | Allow startup without required fields |
| `--enable-feature-ingress` | `true` | Enable Ingress controller |
| `--enable-feature-gateway` | `true` | Enable Gateway API support |
| `--enable-feature-bindings` | `false` | Enable endpoint bindings |
| `--disable-reference-grants` | `false` | Disable Gateway API ReferenceGrant requirement |
| `--default-domain-reclaim-policy` | — | Domain reclaim policy: `Delete` or `Retain` |
| `--drain-policy` | — | Drain policy: `Delete` or `Retain` |

### agent-manager

The data-plane binary that establishes ngrok tunnels.

**Key flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--release-name` | — | Helm release name |
| `--metrics-bind-address` | — | Prometheus metrics endpoint |
| `--health-probe-bind-address` | — | Health probe endpoint |
| `--watch-namespace` | — | Namespace to watch for AgentEndpoints (empty = all) |
| `--region` | — | ngrok tunnel region |
| `--server-addr` | — | ngrok server address |
| `--root-cas` | — | CA certificate source: `trusted` or `host` |
| `--enable-feature-ingress` | `true` | Enable Ingress feature |
| `--enable-feature-gateway` | `true` | Enable Gateway API feature |
| `--enable-feature-bindings` | `false` | Enable bindings feature |
| `--default-domain-reclaim-policy` | — | Domain reclaim policy |

### bindings-forwarder-manager

The bindings data-plane binary for forwarding bound endpoint traffic.

**Key flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--release-name` | — | Helm release name |
| `--metrics-bind-address` | — | Prometheus metrics endpoint |
| `--health-probe-bind-address` | — | Health probe endpoint |
| `--manager-name` | — | Manager identifier |

## Environment Variables

Credentials are provided via environment variables (sourced from the credentials Secret):

| Variable | Description |
|----------|-------------|
| `NGROK_API_KEY` | ngrok API key for API operations |
| `NGROK_AUTHTOKEN` | ngrok auth token for tunnel establishment |

## Scheme Registration

The API Manager registers the following schemes:
- `k8s.io/client-go/kubernetes/scheme` — core Kubernetes types
- `sigs.k8s.io/gateway-api/apis/v1` — Gateway API v1
- `sigs.k8s.io/gateway-api/apis/v1beta1` — Gateway API v1beta1 (ReferenceGrant)
- `sigs.k8s.io/gateway-api/apis/v1alpha2` — Gateway API v1alpha2 (TCPRoute, TLSRoute)
- `ingress.k8s.ngrok.com/v1alpha1` — Ingress CRDs
- `ngrok.k8s.ngrok.com/v1alpha1` — ngrok CRDs
- `bindings.k8s.ngrok.com/v1alpha1` — Bindings CRDs

## Source References

| Symbol / Concept | File | Lines |
|-----------------|------|-------|
| API Manager command | `cmd/api-manager.go` | 79–120 |
| Agent Manager command | `cmd/agent-manager.go` | — |
| Bindings Forwarder command | `cmd/bindings-forwarder-manager.go` | — |
| Root command | `cmd/root.go` | — |
| Common validation | `cmd/common.go` | — |
