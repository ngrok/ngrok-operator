# Helm Chart — Common Configuration

## Chart Structure

The ngrok-operator ships as two Helm charts:

| Chart             | Purpose                          |
|-------------------|----------------------------------|
| `ngrok-operator`  | Operator deployments and RBAC    |
| `ngrok-crds`      | Custom Resource Definitions      |

The CRD chart can be installed automatically via `installCRDs: true` (default) or separately.

**Dependencies:** Bitnami common library (`common-2.36.0.tgz`).

## Common Parameters

| Parameter            | Description                                             | Default                                    |
|----------------------|---------------------------------------------------------|--------------------------------------------|
| `nameOverride`       | Partially override generated resource names             | `""`                                       |
| `fullnameOverride`   | Fully override generated resource names                 | `""`                                       |
| `description`        | Operator description in ngrok dashboard                 | `"The official ngrok Kubernetes Operator."` |
| `commonLabels`       | Labels applied to all deployed objects                  | `{}`                                       |
| `commonAnnotations`  | Annotations applied to all deployed objects             | `{}`                                       |
| `podAnnotations`     | Custom pod annotations for all pods                     | `{}`                                       |
| `podLabels`          | Custom pod labels for all pods                          | `{}`                                       |
| `oneClickDemoMode`   | Start without credentials for demo purposes             | `false`                                    |

## Image Configuration

| Parameter             | Description                              | Default                  |
|-----------------------|------------------------------------------|--------------------------|
| `image.registry`      | Docker registry                          | `docker.io`              |
| `image.repository`    | Image repository                         | `ngrok/ngrok-operator`   |
| `image.tag`           | Image tag (defaults to chart appVersion) | `""`                     |
| `image.pullPolicy`    | Image pull policy                        | `IfNotPresent`           |
| `image.pullSecrets`   | Array of imagePullSecrets                | `[]`                     |

## ngrok Configuration

| Parameter        | Description                                              | Default             |
|------------------|----------------------------------------------------------|---------------------|
| `region`         | ngrok region for tunnels (empty = closest region)        | `""`                |
| `rootCAs`        | CA trust mode: `"trusted"` or `"host"`                   | `""`                |
| `serverAddr`     | Custom ngrok server address                              | `""`                |
| `apiURL`         | Custom ngrok API URL                                     | `""`                |
| `ngrokMetadata`  | Key-value metadata for all ngrok API resources           | `{}`                |
| `clusterDomain`  | Kubernetes cluster domain for DNS resolution             | `svc.cluster.local` |

## Logging

| Parameter              | Description                            | Default  |
|------------------------|----------------------------------------|----------|
| `log.level`            | Log level: `debug`, `info`, `error`    | `info`   |
| `log.stacktraceLevel`  | Stacktrace level: `info`, `error`      | `error`  |
| `log.format`           | Log format: `console`, `json`          | `json`   |

## Credentials

| Parameter                  | Description                                    | Default |
|----------------------------|------------------------------------------------|---------|
| `credentials.secret.name`  | Secret name (auto-generated if empty)          | `""`    |
| `credentials.apiKey`       | ngrok API key                                  | `""`    |
| `credentials.authtoken`    | ngrok auth token                               | `""`    |

See [authentication.md](../authentication.md) for details on credential management.

## CRD Installation

| Parameter      | Description                              | Default |
|----------------|------------------------------------------|---------|
| `installCRDs`  | Install CRDs alongside the operator      | `true`  |

## Domain and Drain Policies

| Parameter                     | Description                                                | Default    |
|-------------------------------|------------------------------------------------------------|------------|
| `drainPolicy`                 | Drain policy on uninstall: `"Delete"` or `"Retain"`        | `"Retain"` |
| `defaultDomainReclaimPolicy`  | Default reclaim policy for Domains: `"Delete"` or `"Retain"` | `"Delete"` |

## Deprecated Parameters

| Deprecated                | Replacement                   |
|---------------------------|-------------------------------|
| `metaData`                | `ngrokMetadata`               |
| `ingressClass.name`       | `ingress.ingressClass.name`   |
| `ingressClass.create`     | `ingress.ingressClass.create` |
| `ingressClass.default`    | `ingress.ingressClass.default`|
| `watchNamespace`          | `ingress.watchNamespace`      |
| `controllerName`          | `ingress.controllerName`      |
