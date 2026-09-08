# Domain

## Resource Identity

| Property    | Value                      |
|-------------|----------------------------|
| Group       | `ngrok.com`                |
| Version     | `v1`                       |
| Kind        | `Domain`                   |
| Scope       | Namespaced                 |

## Spec

| Field           | Type                    | Required | Default                                               | Validation                   |
|-----------------|-------------------------|----------|-------------------------------------------------------|------------------------------|
| `description`   | string                  | No       | `"Created by the ngrok-operator"`                      |                              |
| `metadata`      | map[string]string       | No       | `{"owned-by": "ngrok-operator"}`                       |                              |
| `domain`        | string                  | Yes      |                                                       | Immutable (CEL: `self == oldSelf`) |
| `resolvesTo`    | []DomainResolvesToEntry| No       |                                                       |                              |
| `reclaimPolicy` | DomainReclaimPolicy     | No       | `"Delete"`                                            | Enum: `Delete`, `Retain`     |

### DomainResolvesToEntry

| Field   | Type   |
|---------|--------|
| `value` | string |

### DomainReclaimPolicy

Controls what happens to the ngrok domain reservation when the Domain CR is deleted:

- **`Delete`** (default): The domain reservation is deleted from the ngrok API.
- **`Retain`**: The domain reservation is preserved in the ngrok API.

The default can be overridden globally via the Helm value `defaultDomainReclaimPolicy`.

## Status

| Field                           | Type                                    | Description                                |
|---------------------------------|-----------------------------------------|--------------------------------------------|
| `observedGeneration`            | int64                                   | Generation last reconciled by the controller |
| `id`                            | string                                  | ngrok domain ID                            |
| `domain`                        | string                                  | The domain name                            |
| `resolvesTo`                    | []DomainResolvesToEntry                | Resolved targets                           |
| `cnameTarget`                   | *string                                 | CNAME target for custom domains            |
| `acmeChallengeCNAMETarget`      | *string                                 | ACME challenge CNAME target                |
| `certificate`                   | *DomainStatusCertificateInfo            | Certificate ID                             |
| `certificateManagementPolicy`   | *DomainStatusCertificateManagementPolicy| Certificate authority and key type         |
| `certificateManagementStatus`   | *DomainStatusCertificateManagementStatus| Renewal and provisioning status            |
| `conditions`                    | []Condition                             | MaxItems: 8                                |

## Conditions

| Type               | Description                                       |
|--------------------|---------------------------------------------------|
| `Ready`            | Whether the domain is reserved and available      |
| `DomainCreated`    | Whether the domain was reserved in the ngrok API  |
| `CertificateReady` | Whether the TLS certificate is provisioned        |
| `DNSConfigured`    | Whether DNS records are configured                |

## Printer Columns

| Name           | Source                                                        | Priority |
|----------------|---------------------------------------------------------------|----------|
| ID             | `.status.id`                                                  | 0        |
| Domain         | `.status.domain`                                              | 0        |
| Reclaim Policy | `.spec.reclaimPolicy`                                         | 0        |
| Ready          | `.status.conditions[?(@.type=='Ready')].status`               | 0        |
| Age            | `.metadata.creationTimestamp`                                 | 0        |
| CNAME Target   | `.status.cnameTarget`                                         | 2        |
| Reason         | `.status.conditions[?(@.type=='Ready')].reason`               | 1        |
| Message        | `.status.conditions[?(@.type=='Ready')].message`              | 1        |

## Annotations

The Domain CRD does not consume user-facing annotations.

## Immutability

`spec.domain` is immutable and enforced at admission by a CEL transition rule. Reserved
domains cannot be renamed via the ngrok API — `ReservedDomainUpdate` accepts only
`description`, `metadata`, `certificate`, and `resolvesTo` — so honoring a rename would
mean releasing the existing reservation and reserving a new one, which risks losing the
domain to another account in the gap.

To move to a different domain, delete the Domain and create a new one. Note that
`spec.reclaimPolicy` governs whether the delete also releases the reservation in ngrok.

Every other spec field remains mutable and is reconciled onto the existing reservation.

## Notes

- Domain CRs are typically created automatically by endpoint controllers (AgentEndpoint, CloudEndpoint, Ingress, Gateway routes) rather than by users directly.
- Operator-created Domain CRs are named after the domain itself, via
  `HyphenatedDomainNameFromURL` (dots become dashes, `*` becomes `wildcard`). A different
  hostname therefore yields a different object, so the controllers create a new Domain
  rather than mutating an existing one.
- For custom domains, the `status.cnameTarget` field contains the CNAME that users must configure in their DNS provider.
- Internal domains (URLs ending in `.internal`) skip ngrok API calls entirely.
