# Helm Charts

> Structure and configuration of the operator's Helm charts for deployment and CRD management.

<!-- Last updated: 2026-04-08 -->

## Overview

The operator ships two Helm charts:

| Chart | Path | Purpose |
|-------|------|---------|
| **ngrok-operator** | `helm/ngrok-operator/` | Deploys the operator (API Manager, Agent Manager, and optionally Bindings Forwarder) |
| **ngrok-crds** | `helm/ngrok-crds/` | Installs CRD definitions (separated for lifecycle independence) |

## Chart: ngrok-operator

### Deployed Resources

| Template | Resource | Description |
|----------|----------|-------------|
| `controller-deployment.yaml` | Deployment | API Manager (main controller) |
| `agent/deployment.yaml` | Deployment | Agent Manager (tunnel establishment) |
| `bindings-forwarder/deployment.yaml` | Deployment | Bindings Forwarder (optional) |
| `controller-cm.yaml` | ConfigMap | Configuration for controllers |
| `credentials-secret.yaml` | Secret | ngrok API key and auth token |
| `ingress-class.yaml` | IngressClass | Kubernetes IngressClass for `ngrok` |
| `controller-rbac.yaml` | ClusterRole/Binding | RBAC for API Manager |
| `agent/rbac.yaml` | ClusterRole/Binding | RBAC for Agent Manager |
| `bindings-forwarder/rbac.yaml` | ClusterRole/Binding | RBAC for Bindings Forwarder |
| `controller-pdb.yaml` | PodDisruptionBudget | PDB for API Manager |
| `cleanup-hook/job.yaml` | Job (pre-delete hook) | Pre-delete cleanup hook |
| `rbac/*.yaml` | ClusterRole | Editor/viewer roles for each CRD |

### Key Configuration Parameters

#### ngrok Platform

| Parameter | Default | Description |
|-----------|---------|-------------|
| `region` | `""` (global) | ngrok region for tunnels |
| `rootCAs` | `""` | CA trust: `"trusted"` (ngrok CA) or `"host"` (host CA) |
| `serverAddr` | `""` | Custom ngrok server address |
| `apiURL` | `""` | Custom ngrok API URL |
| `ngrokMetadata` | `{}` | Key-value metadata added to all ngrok API resources |
| `clusterDomain` | `svc.cluster.local` | Kubernetes cluster domain |

#### Credentials

| Parameter | Default | Description |
|-----------|---------|-------------|
| `credentials.secret.name` | `""` | Name of existing Secret (if not creating) |
| `credentials.apiKey` | `""` | ngrok API key |
| `credentials.authtoken` | `""` | ngrok auth token |

#### Feature Flags

| Parameter | Default | Description |
|-----------|---------|-------------|
| `ingressClass.enabled` | `true` | Enable Ingress controller |
| `ingressClass.name` | `ngrok` | IngressClass name |
| `ingressClass.default` | `false` | Set as default IngressClass |
| `gateway.enabled` | `true` | Enable Gateway API support |
| `gateway.disableReferenceGrants` | `false` | Disable ReferenceGrant requirement |
| `bindings.enabled` | `false` | Enable endpoint bindings |
| `bindings.endpointSelectors` | `[]` | CEL expressions for endpoint selection |

#### Operator Manager

| Parameter | Default | Description |
|-----------|---------|-------------|
| `replicaCount` | `1` | API Manager replicas (2+ recommended for HA) |
| `oneClickDemoMode` | `false` | Start without required fields for marketplace installs |
| `log.format` | `json` | Log format |
| `log.level` | `info` | Log level |

#### Agent

| Parameter | Default | Description |
|-----------|---------|-------------|
| `agent.replicaCount` | `1` | Agent Manager replicas |
| `agent.resources` | `{}` | Resource requests/limits |

#### Cleanup & Drain

| Parameter | Default | Description |
|-----------|---------|-------------|
| `cleanupHook.enabled` | `true` | Enable pre-delete cleanup hook |
| `cleanupHook.timeout` | `""` | Cleanup timeout |
| `drainPolicy` | `Retain` | Drain policy: `Delete` or `Retain` |
| `defaultDomainReclaimPolicy` | `Delete` | Domain reclaim policy: `Delete` or `Retain` |

## Chart: ngrok-crds

Installs all CRD definitions as separate templates:

| CRD | Template |
|-----|----------|
| `boundendpoints.bindings.k8s.ngrok.com` | `bindings.k8s.ngrok.com_boundendpoints.yaml` |
| `domains.ingress.k8s.ngrok.com` | `ingress.k8s.ngrok.com_domains.yaml` |
| `ippolicies.ingress.k8s.ngrok.com` | `ingress.k8s.ngrok.com_ippolicies.yaml` |
| `agentendpoints.ngrok.k8s.ngrok.com` | `ngrok.k8s.ngrok.com_agentendpoints.yaml` |
| `cloudendpoints.ngrok.k8s.ngrok.com` | `ngrok.k8s.ngrok.com_cloudendpoints.yaml` |
| `kubernetesoperators.ngrok.k8s.ngrok.com` | `ngrok.k8s.ngrok.com_kubernetesoperators.yaml` |
| `ngroktrafficpolicies.ngrok.k8s.ngrok.com` | `ngrok.k8s.ngrok.com_ngroktrafficpolicies.yaml` |

## Source References

| Symbol / Concept | File | Lines |
|-----------------|------|-------|
| Operator chart | `helm/ngrok-operator/Chart.yaml` | — |
| Values | `helm/ngrok-operator/values.yaml` | — |
| CRDs chart | `helm/ngrok-crds/Chart.yaml` | — |
| API Manager deployment | `helm/ngrok-operator/templates/controller-deployment.yaml` | — |
| Agent deployment | `helm/ngrok-operator/templates/agent/deployment.yaml` | — |
