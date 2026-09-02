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
| `domain`        | string                  | Yes      |                                                       |                              |
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
| `id`                            | string                                  | ngrok domain ID. Empty when the domain is covered by a wildcard |
| `domain`                        | string                                  | The domain name                            |
| `coveredByWildcardDomain`       | string                                  | The wildcard reservation serving this hostname (e.g. `*.example.com`), set when the operator skipped reserving the domain itself |
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
| `Ready`            | Whether the domain is backed by a reservation — its own or a covering wildcard's — and available |
| `DomainCreated`    | Whether the domain is provisioned in the ngrok API, either by its own reservation or by a covering wildcard's |
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
| Wildcard       | `.status.coveredByWildcardDomain`                             | 1        |
| Reason         | `.status.conditions[?(@.type=='Ready')].reason`               | 1        |
| Message        | `.status.conditions[?(@.type=='Ready')].message`              | 1        |

## Annotations

The Domain CRD does not consume user-facing annotations.

## Notes

- Domain CRs are typically created automatically by endpoint controllers (AgentEndpoint, CloudEndpoint, Ingress, Gateway routes) rather than by users directly.
- For custom domains, the `status.cnameTarget` field contains the CNAME that users must configure in their DNS provider.
- Internal domains (URLs ending in `.internal`) skip ngrok API calls entirely.
- A domain that is a direct child of an already-reserved wildcard is not reserved on its own. The CR still exists, `status.coveredByWildcardDomain` names the covering wildcard, and `status.id` stays empty. See [controllers/domain.md](../controllers/domain.md#wildcard-domain-coverage).
