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
3. Look up the reserved domains that could serve the hostname — the hostname
   itself and, if it has one, its wildcard parent — in a single filtered
   `DomainsClient.List` call.
   - If the hostname itself is reserved: adopt that reservation.
   - Else if the wildcard parent is reserved: **skip the reservation** and
     record it in `status.coveredByWildcardDomain`. See
     [Wildcard domain coverage](#wildcard-domain-coverage).
   - Else: create the reservation via `DomainsClient`.
4. Update status with ID, domain, CNAME target, certificate info, and conditions.
5. Call `ReconcileStatus()`.

## Created Resources

- Domain reservation (via ngrok API)

## Status

| Field                           | Description                              |
|---------------------------------|------------------------------------------|
| `id`                            | ngrok domain ID. Empty when the domain is covered by a wildcard |
| `coveredByWildcardDomain`       | The wildcard reservation serving this hostname, when the operator skipped reserving it |
| `domain`                        | The domain name                          |
| `cnameTarget`                   | CNAME target for custom domains          |
| `acmeChallengeCNAMETarget`      | ACME challenge CNAME target              |
| `certificate`                   | Certificate info                         |
| `certificateManagementPolicy`   | Certificate authority and key type       |
| `certificateManagementStatus`   | Renewal and provisioning status          |

## Conditions

| Type    | Description                                    |
|---------|------------------------------------------------|
| `Ready` | Whether the domain is backed by a reservation — its own or a covering wildcard's — and available |

For a wildcard-covered domain, `Ready` and `DomainCreated` both use the reason
`CoveredByWildcardDomain`, and the messages name the covering wildcard. The
domain is provisioned in the platform either way; only the reservation differs.

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

For a wildcard-covered domain the reclaim policy has no effect: there is no
reservation of its own to reclaim, so deleting the CR never calls the ngrok API.

## Special Cases

- **Internal domains**: Domains with URLs ending in `.internal` are not managed in the ngrok API. The controller removes the finalizer and takes no further action.
- **Custom domains**: Require DNS configuration (CNAME to `status.cnameTarget`) before the domain becomes ready. The operator does not verify DNS — it polls the ngrok API which performs the check.

## Wildcard domain coverage

A hostname that is a **direct child** of an already-reserved wildcard does not
get its own reservation. If `*.mydomain.com` is reserved, then `a.mydomain.com`
and `b.mydomain.com` are already served by that wildcard's DNS record and
certificate, so reserving them individually would only add clutter to the
account. This applies to custom wildcards and to ngrok-owned ones such as
`*.mytest.ngrok.io`.

The Domain CR is still created — it is the object other controllers read
`status.cnameTarget` from — but the reconciler skips the ngrok API reservation.

**Matching is single-label**, mirroring DNS and TLS wildcard semantics:

| Hostname | Covered by | Notes |
|----------|-----------|-------|
| `a.mydomain.com` | `*.mydomain.com` | direct child |
| `a.b.mydomain.com` | `*.b.mydomain.com` | **not** `*.mydomain.com` |
| `mydomain.com` | — | a wildcard does not cover its own apex |
| `*.mydomain.com` | — | already a wildcard; reserved normally |

When the reservation is skipped:

- `status.id` is left **empty**. It is the handle the controller would pass to
  the delete API, and the wildcard reservation is shared with every other
  subdomain under it.
- `status.coveredByWildcardDomain` names the covering wildcard.
- `status.domain` is this domain's own hostname, never the wildcard's.
- `status.cnameTarget` mirrors the wildcard's, because that is the record which
  actually resolves this hostname.
- Certificate status mirrors the wildcard's, so readiness reflects whether the
  wildcard is genuinely usable. A custom wildcard whose certificate is still
  provisioning cannot serve its children, and the child reports `Ready=False`
  accordingly.
- `status.acmeChallengeCNAMETarget` and `status.resolvesTo` are left unset;
  they describe the wildcard's own reservation.

**Exceptions.** A Domain that sets `spec.resolvesTo` is always reserved on its
own, because `resolvesTo` is a property of the reservation and there would be
nowhere to record it otherwise.

**Existing reservations are never released.** The check runs only when a Domain
has no reservation yet. Adding a wildcard later does not retroactively remove
reservations the operator already holds, so a domain that reserved itself before
the wildcard existed keeps its reservation.

A covered domain re-verifies its coverage hourly, so if the wildcard reservation
is removed from the account the domain reserves itself on the next check.
