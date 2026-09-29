# Helm Chart — Bindings Forwarder Deployment

## Overview

The bindings forwarder runs as a separate deployment responsible for forwarding traffic for bound endpoints. It is only deployed when `ngrok.features.bindings.enabled: true`.

## Pod Settings

Any key from `components.common` (see [common.md](common.md)) can also be set here to override it for this component alone.

| Parameter | Description | Default |
|-----------|-------------|---------|
| `components.bindingsForwarder.replicaCount` | Number of replicas | `1` |
| `components.bindingsForwarder.resources` | Container resource requests/limits | `{}` |
| `components.bindingsForwarder.lifecycle` | Container lifecycle hooks | `{}` |
| `components.bindingsForwarder.extraVolumes` | Additional volumes | `[]` |
| `components.bindingsForwarder.extraVolumeMounts` | Additional volume mounts | `[]` |
| `components.bindingsForwarder.serviceAccount.create` | Create a ServiceAccount | `true` |
| `components.bindingsForwarder.serviceAccount.name` | ServiceAccount name (auto-generated if empty) | `""` |
| `components.bindingsForwarder.serviceAccount.annotations` | ServiceAccount annotations | `{}` |

## Operator Configuration

`components.bindingsForwarder.log` overrides `ngrok.log` for the bindings-forwarder alone.
