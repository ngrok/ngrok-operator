# Namespace Watching

## Overview

The operator can be scoped to watch specific namespaces instead of the entire cluster. This is useful for multi-tenant clusters or installations where the operator should only manage resources in certain namespaces.

## Configuration

| Component     | Flag                         | Helm Value                | Default |
|---------------|------------------------------|---------------------------|---------|
| api-manager   | `--ingress-watch-namespace`  | `features.ingress.watchNamespace`  | `""`    |
| agent-manager | `--watch-namespace`          | (none)                    | `""`    |

An empty value means "watch all namespaces" (cluster-wide).

## Behavior

When a watch namespace is set:

- The controller-runtime cache is restricted to the specified namespace via `cache.Options.DefaultNamespaces`.
- Only resources in that namespace are visible to the controllers.
- Resources in other namespaces are not watched or reconciled.

## Affected Controllers

| Controller                | Respects Watch Namespace |
|---------------------------|--------------------------|
| Ingress                   | Yes                      |
| Gateway API (all)         | Yes                      |
| Service LoadBalancer       | Yes                      |
| AgentEndpoint (agent)     | Yes                      |
| KubernetesOperator        | No (always watches only its own CR) |
| BoundEndpoint             | No (watches operator namespace only) |

## Caveats

Setting `watchNamespace` to a value different from the operator's release namespace is **not supported**. The operator's TLS Secret and KubernetesOperator CR both live in the release namespace. With a mismatched `watchNamespace`, the controller-runtime cache scoped to `watchNamespace` will not see those resources, causing the operator to fail to function correctly. See [rbac/README.md](../rbac/README.md) for the full details on this constraint.
