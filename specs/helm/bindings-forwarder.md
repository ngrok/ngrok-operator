# Helm Chart — Bindings Forwarder Deployment

## Overview

The bindings forwarder runs as a separate deployment responsible for forwarding traffic for bound endpoints. It is only deployed when `features.bindings.enabled: true`.

## Pod Settings

Any key from `defaults` (see [common.md](common.md)) can also be set here to override it for this component alone.

| Parameter | Description | Default |
|-----------|-------------|---------|
| `bindingsForwarder.replicaCount` | Number of replicas | `1` |
| `bindingsForwarder.resources` | Container resource requests/limits | `{}` |
| `bindingsForwarder.lifecycle` | Container lifecycle hooks | `{}` |
| `bindingsForwarder.terminationGracePeriodSeconds` | Graceful shutdown time | `30` |
| `bindingsForwarder.extraVolumes` | Additional volumes | `[]` |
| `bindingsForwarder.extraVolumeMounts` | Additional volume mounts | `[]` |
| `bindingsForwarder.updateStrategy.type` | Update strategy type | `RollingUpdate` |
| `bindingsForwarder.serviceAccount.create` | Create a ServiceAccount | `true` |
| `bindingsForwarder.serviceAccount.name` | ServiceAccount name (auto-generated if empty) | `""` |
| `bindingsForwarder.serviceAccount.annotations` | ServiceAccount annotations | `{}` |

## Operator Configuration

`bindingsForwarder.config.log` overrides the shared `log` settings for the bindings-forwarder alone. There are no bindings-forwarder-only settings at this time.
