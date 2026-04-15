# Aggregation Roles

## Overview

The operator creates editor and viewer ClusterRoles for each CRD. These roles use label-based aggregation to automatically merge into the Kubernetes built-in `admin`, `edit`, and `view` ClusterRoles.

## Editor Roles

Editor roles grant full management access to the CRD resources.

| ClusterRole Name                         | Resources                |
|------------------------------------------|--------------------------|
| `boundendpoint-editor-role`              | `boundendpoints`, `boundendpoints/status` |
| `cloudendpoint-editor-role`              | `cloudendpoints`, `cloudendpoints/status` |
| `domain-editor-role`                     | `domains`, `domains/status` |
| `agentendpoint-editor-role`              | `agentendpoints`, `agentendpoints/status` |
| `ippolicy-editor-role`                   | `ippolicies`, `ippolicies/status` |
| `kubernetesoperator-editor-role`         | `kubernetesoperators`, `kubernetesoperators/status` |
| `trafficpolicy-editor-role`         | `trafficpolicies`, `trafficpolicies/status` |

**Verbs:** create, delete, get, list, patch, update, watch

## Viewer Roles

Viewer roles grant read-only access to the CRD resources.

| ClusterRole Name                         | Resources                |
|------------------------------------------|--------------------------|
| `boundendpoint-viewer-role`              | `boundendpoints`, `boundendpoints/status` |
| `cloudendpoint-viewer-role`              | `cloudendpoints`, `cloudendpoints/status` |
| `domain-viewer-role`                     | `domains`, `domains/status` |
| `agentendpoint-viewer-role`              | `agentendpoints`, `agentendpoints/status` |
| `ippolicy-viewer-role`                   | `ippolicies`, `ippolicies/status` |
| `kubernetesoperator-viewer-role`         | `kubernetesoperators`, `kubernetesoperators/status` |
| `trafficpolicy-viewer-role`         | `trafficpolicies`, `trafficpolicies/status` |

**Verbs:** get, list, watch

## Custom Annotations

The `clusterRole.annotations` Helm value allows adding custom annotations to all ClusterRoles. This is commonly used for RBAC aggregation — for example, aggregating the editor role into the built-in `admin` or `edit` roles so that users with those roles automatically gain CRD access without needing `system:masters`.
