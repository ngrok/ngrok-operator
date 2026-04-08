# {{CRD_NAME}}

> {{ONE_LINE_DESCRIPTION}}

<!-- Last updated: {{DATE}} -->

## Overview

{{NARRATIVE — what this CRD represents, which API group/version it belongs to, and
its purpose within the operator.}}

**API Group:** `{{GROUP}}`
**Version:** `{{VERSION}}`
**Kind:** `{{KIND}}`
**Scope:** `Namespaced` | `Cluster`

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `fieldA` | `string` | Yes | — | {{DESCRIPTION}} |
| `fieldB` | `int` | No | `10` | {{DESCRIPTION}} |

## Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | `[]Condition` | Standard Kubernetes conditions |
| `observedGeneration` | `int64` | Last generation reconciled |

## Validation Rules

- {{RULE — e.g., "fieldA must be a valid DNS name"}}
- {{RULE — e.g., "fieldB must be between 1 and 100"}}

## Defaulting

- {{DEFAULT — e.g., "If fieldB is omitted, it defaults to 10"}}

## Lifecycle

```mermaid
stateDiagram-v2
    [*] --> Created : User applies manifest
    Created --> Ready : Controller reconciles successfully
    Created --> Error : Reconciliation fails
    Error --> Ready : Issue resolved, re-reconcile
    Ready --> Updating : Spec changed
    Updating --> Ready : Reconciliation succeeds
    Updating --> Error : Reconciliation fails
    Ready --> Deleting : User deletes resource
    Deleting --> [*] : Finalizer completes
```

## Relationships

| Related Resource | Relationship | Description |
|-----------------|--------------|-------------|
| `OtherCRD` | References | {{DESCRIPTION}} |

## Source References

| Symbol / Concept | File | Lines |
|-----------------|------|-------|
