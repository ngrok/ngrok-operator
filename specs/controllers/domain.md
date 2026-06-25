# Domain Controller

## Summary

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

## Requeue Behavior

The Domain controller uses an exponential backoff rate limiter (30s base, 10m max) for all retries. The `Ready` condition remains `False` and the controller keeps requeuing until:

- The ngrok API confirms the domain reservation is active.
- For custom domains: DNS is configured correctly (CNAME pointing to `cnameTarget`).
- For domains with managed certificates: the certificate has been issued.

There is no timeout — the controller will requeue indefinitely until the domain becomes ready or is deleted.

## Reclaim Policy

The `spec.reclaimPolicy` field controls what happens to the ngrok domain reservation when the Domain CR is deleted:

| Value    | Behavior                                                      |
|----------|---------------------------------------------------------------|
| `Delete` | The ngrok domain reservation is deleted (default)             |
| `Retain` | The ngrok domain reservation is kept; only the CR is removed  |

The default is set via `features.defaultDomainReclaimPolicy` in Helm values (default: `Delete`). Use `Retain` to preserve reserved domains across operator reinstalls or when managing domains outside of the operator's lifecycle.

## Special Cases

- **Internal domains**: Domains with URLs ending in `.internal` are not managed in the ngrok API. The controller removes the finalizer and takes no further action.
- **Custom domains**: Require DNS configuration (CNAME to `status.cnameTarget`) before the domain becomes ready. The operator does not verify DNS — it polls the ngrok API which performs the check.
