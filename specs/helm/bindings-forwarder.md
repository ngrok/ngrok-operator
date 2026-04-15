# Helm Chart — Bindings Forwarder Deployment

## Overview

The bindings forwarder runs as a separate deployment responsible for forwarding traffic for bound endpoints. It is only deployed when `bindings.enabled: true`.

## Parameters

| Parameter                                              | Description                                     | Default         |
|--------------------------------------------------------|-------------------------------------------------|-----------------|
| `bindings.forwarder.replicaCount`                      | Number of forwarder replicas                    | `1`             |
| `bindings.forwarder.resources.limits`                  | Container resource limits                       | `{}`            |
| `bindings.forwarder.resources.requests`                | Container resource requests                     | `{}`            |
| `bindings.forwarder.serviceAccount.create`             | Create a ServiceAccount for the forwarder       | `true`          |
| `bindings.forwarder.serviceAccount.name`               | ServiceAccount name (auto-generated if empty)   | `""`            |
| `bindings.forwarder.serviceAccount.annotations`        | ServiceAccount annotations                      | `{}`            |
| `bindings.forwarder.updateStrategy.type`               | Update strategy type                            | `RollingUpdate` |
| `bindings.forwarder.terminationGracePeriodSeconds`     | Graceful shutdown time                          | `30`            |
| `bindings.forwarder.tolerations`                       | Pod tolerations                                 | `[]`            |
| `bindings.forwarder.nodeSelector`                      | Node labels for pod assignment                  | `{}`            |
| `bindings.forwarder.topologySpreadConstraints`         | Topology spread constraints                     | `[]`            |
