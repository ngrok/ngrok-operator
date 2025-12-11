# AGENTS.md - ngrok Kubernetes Operator

The ngrok-operator is a Kubernetes Operator that reconciles:

- Service resources of `Type=LoadBalancer`
- Ingress resources
- Gateway API resources
- ngrok CRDs

## Quick Facts

- **Module**: `github.com/ngrok/ngrok-operator`
- **Type**: Kubernetes Operator (kubebuilder v4, multigroup)
- **Domain**: `k8s.ngrok.com`
- **Language**: Go

## Entry Points & Modes

- **[main.go](main.go)** → `cmd.Execute()`
- **[cmd/](cmd/)** contains three operational modes:
  - `api-manager.go` - Manages ngrok resources via ngrok API (CRDs → ngrok API)
  - `agent-manager.go` - Runs ngrok agents in-cluster for tunneling
  - `bindings-forwarder-manager.go` - Manages service bindings and forwarding

## Directory Map

```
api/<group>/v1alpha1/        # CRD definitions (ingress, ngrok, bindings)
internal/controller/<group>/ # Reconcilers organized by API group
internal/controller/base_controller.go  # Common CRUD/finalizers/status helpers
internal/ngrokapi/           # ngrok API client wrappers
internal/ir/                 # Intermediate representation (K8s → ngrok API)
internal/store/              # State management and caching
pkg/managerdriver/           # Manager driver implementations
pkg/bindingsdriver/          # Bindings driver implementations
cmd/                         # CLI and manager entry points
config/                      # Kustomize manifests (CRDs, RBAC, etc.)
helm/ngrok-operator/         # Helm chart
```

## Resource → Controller Map

| API Group | Resources | Controller Location |
|-----------|-----------|---------------------|
| **ingress.k8s.ngrok.com/v1alpha1** | Domain, IPPolicy | [internal/controller/ingress/](internal/controller/ingress/) |
| **ingress.k8s.ngrok.com/v1alpha1** | Ingress (networking.k8s.io/v1) | [internal/controller/ingress/](internal/controller/ingress/) |
| **ngrok.k8s.ngrok.com/v1alpha1** | NgrokTrafficPolicy, KubernetesOperator | [internal/controller/ngrok/](internal/controller/ngrok/) |
| **bindings.k8s.ngrok.com/v1alpha1** | BindingConfiguration, BoundEndpoint | [internal/controller/bindings/](internal/controller/bindings/) |
| **gateway.networking.k8s.io/v1** | Gateway, GatewayClass, HTTPRoute, TLSRoute, TCPRoute | [internal/controller/gateway/](internal/controller/gateway/) |
| **core/v1** | Service | [internal/controller/service/](internal/controller/service/) |
| **agent** | AgentEndpoint | [internal/controller/agent/](internal/controller/agent/) |

## Core Patterns

- **BaseController** ([internal/controller/base_controller.go](internal/controller/base_controller.go)) - Common CRUD operations, finalizer management, status updates
- **IR** ([internal/ir/](internal/ir/)) - Translates Kubernetes resources to ngrok API concepts
- **Store** ([internal/store/](internal/store/)) - State and caching across reconcilers
- **Drivers** ([pkg/managerdriver/](pkg/managerdriver/), [pkg/bindingsdriver/](pkg/bindingsdriver/)) - Abstracts operational modes

## Development Environment

Use the Nix devShell for a consistent development environment:

```bash
nix develop                 # Enter devShell with all tools (recommended)
```

Users with [direnv](https://direnv.net/) configured will automatically enter the devShell via `.envrc`.

Run `make preflight` to verify your environment is configured correctly.

## Development Workflow

```bash
make help                   # List all available targets
make preflight              # Verify environment (Go version, controller-gen)
make generate               # Generate DeepCopy methods
make manifests              # Generate CRDs, RBAC, webhooks
make build                  # Build binaries
make test                   # Run unit tests
make e2e-tests              # Run E2E tests
make run                    # Run locally (uses kubeconfig)
make deploy                 # Deploy to cluster
make undeploy               # Remove from cluster
```

## Configuration

- **Credentials**: `helm install` requires `credentials.apiKey` and `credentials.authtoken`
- **Feature Flags**: `useExperimentalGatewayApi=true` enables Gateway API support

## Rules

- Always register new APIs with `AddToScheme` in manager setup
- Update RBAC markers (`// +kubebuilder:rbac:...`) when adding permissions
- Use status subresource (`r.Status().Update()`) for status-only updates
- Manage finalizers on resource deletion to clean up external resources
- Requeue on transient ngrok API errors (return `ctrl.Result{Requeue: true}`)
- Prefer `BaseController` helpers over raw client operations

