# Helm Chart — Agent Deployment

## Overview

The agent (agent-manager) runs as a separate deployment responsible for managing ngrok agent tunnels for AgentEndpoint resources.

## Parameters

| Parameter                                | Description                                           | Default         |
|------------------------------------------|-------------------------------------------------------|-----------------|
| `agent.replicaCount`                     | Number of agent replicas                              | `1`             |
| `agent.podAnnotations`                   | Custom pod annotations (falls back to `podAnnotations`) | `{}`            |
| `agent.priorityClassName`                | Pod priority class                                    | `""`            |
| `agent.resources.limits`                 | Container resource limits                             | `{}`            |
| `agent.resources.requests`               | Container resource requests                           | `{}`            |
| `agent.serviceAccount.create`            | Create a ServiceAccount for the agent                 | `true`          |
| `agent.serviceAccount.name`              | ServiceAccount name (auto-generated if empty)         | `""`            |
| `agent.serviceAccount.annotations`       | ServiceAccount annotations                            | `{}`            |
| `agent.updateStrategy.type`              | Update strategy type                                  | `RollingUpdate` |
| `agent.terminationGracePeriodSeconds`    | Graceful shutdown time                                | `30`            |
| `agent.tolerations`                      | Pod tolerations                                       | `[]`            |
| `agent.nodeSelector`                     | Node labels for pod assignment                        | `{}`            |
| `agent.topologySpreadConstraints`        | Topology spread constraints                           | `[]`            |
