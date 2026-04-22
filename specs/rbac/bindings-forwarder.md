# Bindings Forwarder RBAC

## Overview

The bindings forwarder has dual-scoped permissions: a namespaced Role for resources in the operator's namespace, and a ClusterRole for cluster-wide pod lookups.

## Namespaced Role

Scoped to the operator's namespace.

| API Group                  | Resource               | Verbs                           |
|----------------------------|------------------------|---------------------------------|
| `ngrok.com`               | `boundendpoints`       | get, list, watch, patch, update |
| `""`                       | `events`               | create, patch                   |
| `ngrok.com`               | `kubernetesoperators`  | get, list, watch                |
| `""`                       | `secrets`              | get, list, watch                |

## ClusterRole

| API Group | Resource | Verbs            |
|-----------|----------|------------------|
| `""`      | `pods`   | get, list, watch |

## Notes

- Pod read access is cluster-wide because the forwarder needs to look up pods by IP address to identify connection sources. Pod IPs are indexed via a field indexer on `status.podIP`.
- Secret read access is needed for the TLS certificate used in mTLS communication with the ngrok ingress endpoint.
- KubernetesOperator read access is needed to fetch binding configuration and the ingress endpoint address.
- The forwarder only deploys when `bindings.enabled: true`.
