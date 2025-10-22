# Domain Conditions

This document describes the status conditions for Domain resources.

## Ready

**Type**: `Ready`

**Description**: Indicates whether the Domain is fully ready for use, with certificate provisioning and DNS configuration complete. For ngrok-managed domains, this will be True immediately upon creation. For custom domains, this will be True when both certificate and DNS are ready.

**Possible Values**:
- `True`: Domain is fully active and ready for use
- `False`: Domain is not ready (creation failed or provisioning incomplete)
- `Unknown`: Domain status cannot be determined

**Reasons**:

| Reason | Status | Description |
|--------|--------|-------------|
| DomainActive | True | Domain is fully active and ready for use, with all provisioning steps complete |
| DomainInvalid | False | Domain specification is invalid (e.g., invalid domain name format) |
| DomainCreationFailed | False | Domain creation failed via the ngrok API |
| ProvisioningError | False | Error during the domain provisioning process (certificate or DNS) |

## DomainCreated

**Type**: `DomainCreated`

**Description**: Indicates whether the domain has been successfully created or registered via the ngrok API. This condition will be True when domain creation succeeds, and False if domain creation fails or the domain specification is invalid.

**Reasons**:

| Reason | Status | Description |
|--------|--------|-------------|
| DomainCreated | True | Domain has been successfully created or registered via the ngrok API |
| DomainCreationFailed | False | Domain creation failed via the ngrok API |
| DomainInvalid | False | Domain specification is invalid |

## CertificateReady

**Type**: `CertificateReady`

**Description**: Indicates whether the TLS certificate for the domain has been successfully provisioned. For ngrok-managed domains, this is always True. For custom domains, this tracks the certificate provisioning process.

**Reasons**:

| Reason | Status | Description |
|--------|--------|-------------|
| CertificateReady | True | TLS certificate has been successfully provisioned for the domain |
| NgrokManaged | True | Domain is ngrok-managed and certificate management is handled automatically by ngrok |
| ProvisioningError | False | Error provisioning the TLS certificate for the domain |
| DomainCreationFailed | False | Certificate provisioning cannot proceed because the domain creation failed |
| DomainInvalid | False | Certificate provisioning cannot proceed because the domain specification is invalid |

## DNSConfigured

**Type**: `DNSConfigured`

**Description**: Indicates whether DNS configuration for the domain is complete and correctly pointing to ngrok. For ngrok-managed domains, this is always True. For custom domains, this tracks DNS setup status.

**Reasons**:

| Reason | Status | Description |
|--------|--------|-------------|
| DomainCreated | True | DNS is properly configured for the domain |
| NgrokManaged | True | Domain is ngrok-managed and DNS is automatically configured by ngrok |
| ProvisioningError | False | Error with DNS configuration for the domain |
| DomainCreationFailed | False | DNS configuration cannot proceed because the domain creation failed |
| DomainInvalid | False | DNS configuration cannot proceed because the domain specification is invalid |

## Progressing

**Type**: `Progressing`

**Description**: Indicates whether the domain is currently being provisioned, with setup steps in progress. This condition will be True during active provisioning operations.

**Reasons**:

| Reason | Status | Description |
|--------|--------|-------------|
| Provisioning | True | Domain is actively being provisioned, with certificate or DNS setup in progress |

## Example Status

```yaml
status:
  conditions:
  - type: Ready
    status: "True"
    reason: DomainActive
    message: "Domain is fully ready"
    lastTransitionTime: "2024-01-15T10:30:00Z"
  - type: DomainCreated
    status: "True"
    reason: DomainCreated
    message: "Domain successfully created"
    lastTransitionTime: "2024-01-15T10:28:00Z"
  - type: CertificateReady
    status: "True"
    reason: CertificateReady
    message: "TLS certificate provisioned"
    lastTransitionTime: "2024-01-15T10:29:00Z"
  - type: DNSConfigured
    status: "True"
    reason: DomainCreated
    message: "DNS properly configured"
    lastTransitionTime: "2024-01-15T10:28:00Z"
```
