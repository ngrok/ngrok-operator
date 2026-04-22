# Multiple Operator Installations

## Overview

The ngrok-operator architecture supports multiple independent operator installations in the same Kubernetes cluster. Each installation operates in isolation with its own set of managed resources.

## Isolation Mechanisms

### KubernetesOperator Predicate

Each operator deployment only reconciles its own KubernetesOperator CR, enforced by a predicate that matches on:

- **Name**: The Helm release name
- **Namespace**: The pod's namespace (from `POD_NAMESPACE` environment variable)

This prevents one operator from interfering with another's registration or drain state.

### Leader Election

Each deployment has its own leader election lease:

- Default lease name: `ngrok-operator-leader`
- Configurable via `--election-id` flag
- Different releases in different namespaces naturally isolate since leases are namespaced

### Drain State

Each operator instance maintains independent drain state:

- `DrainOrchestrator` is configured with the specific `K8sOpName` and `K8sOpNamespace`
- `StateChecker` only monitors its own KubernetesOperator CR
- Draining one installation does not affect others

### Namespace Watching

Each deployment can be scoped to watch different namespaces:

- `features.ingress.watchNamespace` for the api-manager
- `--watch-namespace` for the agent-manager
- Scoping prevents resource conflicts between installations

## Deployment Model

```text
Cluster:
├── ngrok-operator (release-a) in namespace-a
│   ├── api-manager (independent leader election)
│   ├── agent-manager
│   └── bindings-forwarder (if enabled)
│
└── ngrok-operator (release-b) in namespace-b
    ├── api-manager (independent leader election)
    ├── agent-manager
    └── bindings-forwarder (if enabled)
```

Each instance:
- Reconciles only its own KubernetesOperator CR
- Elects leaders independently
- Has separate drain state
- Can watch different namespaces
- Maintains a separate ngrok API registration
- Can use the same or different ngrok API accounts

## Conflict Avoidance

| Mechanism                     | What it prevents                              |
|-------------------------------|-----------------------------------------------|
| Release name uniqueness       | ConfigMap lease collisions                    |
| Namespace+name predicate      | KubernetesOperator CR overlap                 |
| Metadata with namespace UID   | ngrok API ID reuse across reinstalls          |
| Per-namespace TLS secrets     | mTLS certificate collisions                   |
