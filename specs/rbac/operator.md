# Operator (api-manager) RBAC

## ClusterRole

The main operator ClusterRole requires the following permissions:

### Core API (`""`)

| Resource                | Verbs                                          |
|-------------------------|------------------------------------------------|
| `configmaps`            | get, list, watch, create, update, patch, delete |
| `events`                | create, patch                                  |
| `namespaces`            | get, list, update, watch                       |
| `secrets`               | get, list, watch                               |
| `services`              | create, delete, get, list, patch, update, watch |
| `services/finalizers`   | patch, update                                  |
| `services/status`       | get, list, patch, update, watch                |

### ngrok API (`ngrok.com`)

| Resource                          | Verbs                                          |
|-----------------------------------|------------------------------------------------|
| `agentendpoints`                  | create, delete, get, list, patch, update, watch |
| `agentendpoints/finalizers`       | patch, update                                  |
| `agentendpoints/status`           | get, patch, update                             |
| `cloudendpoints`                  | create, delete, get, list, patch, update, watch |
| `cloudendpoints/finalizers`       | patch, update                                  |
| `cloudendpoints/status`           | get, patch, update                             |
| `kubernetesoperators`             | create, delete, get, list, patch, update, watch |
| `kubernetesoperators/finalizers`  | patch, update                                  |
| `kubernetesoperators/status`      | get, patch, update                             |
| `trafficpolicies`            | create, delete, get, list, patch, update, watch |
| `trafficpolicies/finalizers` | patch, update                                  |
| `trafficpolicies/status`     | get, patch, update                             |
| `domains`                         | create, delete, get, list, patch, update, watch |
| `domains/finalizers`              | patch, update                                  |
| `domains/status`                  | get, patch, update                             |
| `ippolicies`                      | create, delete, get, list, patch, update, watch |
| `ippolicies/finalizers`           | patch, update                                  |
| `ippolicies/status`               | get, patch, update                             |
| `boundendpoints`                  | create, delete, get, list, patch, update, watch |
| `boundendpoints/finalizers`       | patch, update                                  |
| `boundendpoints/status`           | get, patch, update                             |

### Networking (`networking.k8s.io`)

| Resource                  | Verbs                                  |
|---------------------------|----------------------------------------|
| `ingressclasses`          | get, list, watch                       |
| `ingresses`               | get, list, patch, update, watch        |
| `ingresses/finalizers`    | patch, update                          |
| `ingresses/status`        | get, list, update, watch               |

### Gateway API (`gateway.networking.k8s.io`)

| Resource                      | Verbs                                  |
|-------------------------------|----------------------------------------|
| `gatewayclasses`              | get, list, patch, update, watch        |
| `gatewayclasses/finalizers`   | patch, update                          |
| `gatewayclasses/status`       | get, list, update, watch               |
| `gateways`                    | get, list, patch, update, watch        |
| `gateways/finalizers`         | patch, update                          |
| `gateways/status`             | get, list, update, watch               |
| `httproutes`                  | get, list, patch, update, watch        |
| `httproutes/finalizers`       | patch, update                          |
| `httproutes/status`           | get, list, update, watch               |
| `tcproutes`                   | get, list, patch, update, watch        |
| `tcproutes/finalizers`        | patch, update                          |
| `tcproutes/status`            | get, list, update, watch               |
| `tlsroutes`                   | get, list, patch, update, watch        |
| `tlsroutes/finalizers`        | patch, update                          |
| `tlsroutes/status`            | get, list, update, watch               |
| `referencegrants`             | get, list, watch                       |

## Namespaced Roles

### Leader Election Role

Scoped to the operator's namespace.

| API Group              | Resource       | Verbs                                          |
|------------------------|----------------|------------------------------------------------|
| `""`                   | `configmaps`   | get, list, watch, create, update, patch, delete |
| `coordination.k8s.io`  | `leases`       | get, list, watch, create, update, patch, delete |
| `""`                   | `events`       | create, patch                                  |

### Proxy Role (ClusterRole)

For webhook authentication.

| API Group                   | Resource                | Verbs  |
|-----------------------------|-------------------------|--------|
| `authentication.k8s.io`     | `tokenreviews`          | create |
| `authorization.k8s.io`      | `subjectaccessreviews`  | create |

### Secret Manager Role (conditional)

Scoped to the operator's namespace. Only created when `bindings.enabled: true`.

| Resource  | Verbs                                          |
|-----------|------------------------------------------------|
| `secrets` | get, list, watch, create, update, patch, delete |
