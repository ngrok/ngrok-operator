# Domain Controller

## Executive Summary

The Domain controller reconciles `Domain` resources by creating and managing domain reservations in the ngrok API. It handles DNS provisioning delays, CNAME target resolution, and certificate management status.

## Watches

| Resource  | Relation | Predicate                                          |
|-----------|----------|----------------------------------------------------|
| `Domain`  | Primary  | AnnotationChanged or GenerationChanged; exponential backoff rate limiter (30s base, 10m max) |

## Reconciliation Flow

1. Check if the domain is internal (URL ending in `.internal`).
   - If internal: skip ngrok API calls, remove finalizer, done.
2. Add finalizer.
3. Create or update the domain reservation via `DomainsClient`.
4. Update status with ID, domain, region, CNAME target, certificate info, and conditions.
5. Call `ReconcileStatus()`.

## Created Resources

- Domain reservation (via ngrok API)

## Status

| Field                           | Description                              |
|---------------------------------|------------------------------------------|
| `id`                            | ngrok domain ID                          |
| `domain`                        | The domain name                          |
| `region`                        | The domain's region                      |
| `cnameTarget`                   | CNAME target for custom domains          |
| `acmeChallengeCNAMETarget`      | ACME challenge CNAME target              |
| `certificate`                   | Certificate info                         |
| `certificateManagementPolicy`   | Certificate authority and key type       |
| `certificateManagementStatus`   | Renewal and provisioning status          |

## Conditions

| Type    | Description                                    |
|---------|------------------------------------------------|
| `Ready` | Whether the domain is reserved and available   |

## Error Handling

| Error Code | Description                | Behavior                     |
|------------|----------------------------|------------------------------|
| 446        | Domain attached to edge    | Retryable (exponential backoff) |
| 511        | Dangling CNAME             | Retryable (exponential backoff) |
| Default    |                            | Via `CtrlResultForErr`       |

## Special Cases

- **Internal domains**: Domains with URLs ending in `.internal` are not managed in the ngrok API. The controller removes the finalizer and takes no further action.
- **DNS provisioning**: Custom domains may take time for DNS to propagate. The exponential backoff rate limiter handles retries during this period.
