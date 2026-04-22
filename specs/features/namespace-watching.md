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

- If the watch namespace does not match the operator's namespace, the KubernetesOperator CR may not be visible to the cache. The KubernetesOperator controller uses a name+namespace predicate that is independent of the cache scope, but other controllers that need to read the KubernetesOperator may be affected.
- Domain and IPPolicy controllers operate on resources created by other controllers, so they inherit the effective scope of the controllers that create them.
