package v1alpha1

// DomainConditionType is a type of condition for a Domain.
type DomainConditionType string

// DomainConditionReadyReason is a reason for the Ready condition on a Domain.
type DomainConditionReadyReason string

// DomainConditionCreatedReason is a reason for the DomainCreated condition on a Domain.
type DomainConditionCreatedReason string

// DomainConditionCertificateReadyReason is a reason for the CertificateReady condition on a Domain.
type DomainConditionCertificateReadyReason string

// DomainConditionDNSConfiguredReason is a reason for the DNSConfigured condition on a Domain.
type DomainConditionDNSConfiguredReason string

// DomainConditionProgressingReason is a reason for the Progressing condition on a Domain.
type DomainConditionProgressingReason string

const (
	// DomainConditionReady indicates whether the Domain is fully ready for use,
	// with certificate provisioning and DNS configuration complete.
	// For ngrok-managed domains, this will be True immediately upon creation.
	// For custom domains, this will be True when both certificate and DNS are ready.
	DomainConditionReady DomainConditionType = "Ready"

	// DomainConditionCreated indicates whether the domain has been successfully
	// created or registered via the ngrok API.
	// This condition will be True when domain creation succeeds,
	// and False if domain creation fails or the domain specification is invalid.
	DomainConditionCreated DomainConditionType = "DomainCreated"

	// DomainConditionCertificateReady indicates whether the TLS certificate for
	// the domain has been successfully provisioned.
	// For ngrok-managed domains, this is always True.
	// For custom domains, this tracks the certificate provisioning process.
	DomainConditionCertificateReady DomainConditionType = "CertificateReady"

	// DomainConditionDNSConfigured indicates whether DNS configuration for the
	// domain is complete and correctly pointing to ngrok.
	// For ngrok-managed domains, this is always True.
	// For custom domains, this tracks DNS setup status.
	DomainConditionDNSConfigured DomainConditionType = "DNSConfigured"

	// DomainConditionProgressing indicates whether the domain is currently being
	// provisioned, with setup steps in progress.
	// This condition will be True during active provisioning operations.
	DomainConditionProgressing DomainConditionType = "Progressing"
)

// Reasons for Ready condition
const (
	// DomainReasonActive is used when the Domain is fully active and ready for use,
	// with all provisioning steps complete.
	DomainReasonActive DomainConditionReadyReason = "DomainActive"

	// DomainReasonInvalid is used when the Domain specification is invalid
	// (e.g., invalid domain name format).
	DomainReasonInvalid DomainConditionReadyReason = "DomainInvalid"

	// DomainReasonCreationFailed is used when the Domain creation failed via the ngrok API.
	DomainReasonCreationFailed DomainConditionReadyReason = "DomainCreationFailed"

	// DomainReasonProvisioningError is used when there is an error during the
	// domain provisioning process (certificate or DNS).
	DomainReasonProvisioningError DomainConditionReadyReason = "ProvisioningError"
)

// Reasons for DomainCreated condition
const (
	// DomainCreatedReasonCreated is used when the domain has been successfully created
	// or registered via the ngrok API.
	DomainCreatedReasonCreated DomainConditionCreatedReason = "DomainCreated"

	// DomainCreatedReasonCreationFailed is used when the domain creation failed via the ngrok API.
	DomainCreatedReasonCreationFailed DomainConditionCreatedReason = "DomainCreationFailed"

	// DomainCreatedReasonInvalid is used when the Domain specification is invalid.
	DomainCreatedReasonInvalid DomainConditionCreatedReason = "DomainInvalid"
)

// Reasons for CertificateReady condition
const (
	// DomainCertificateReadyReasonReady is used when the TLS certificate has been
	// successfully provisioned for the domain.
	DomainCertificateReadyReasonReady DomainConditionCertificateReadyReason = "CertificateReady"

	// DomainCertificateReadyReasonNgrokManaged is used when the domain is ngrok-managed and
	// certificate management is handled automatically by ngrok.
	DomainCertificateReadyReasonNgrokManaged DomainConditionCertificateReadyReason = "NgrokManaged"

	// DomainCertificateReadyReasonProvisioningError is used when there is an error provisioning
	// the TLS certificate for the domain.
	DomainCertificateReadyReasonProvisioningError DomainConditionCertificateReadyReason = "ProvisioningError"

	// DomainCertificateReadyReasonCreationFailed is used when certificate provisioning cannot proceed
	// because the domain creation failed.
	DomainCertificateReadyReasonCreationFailed DomainConditionCertificateReadyReason = "DomainCreationFailed"

	// DomainCertificateReadyReasonInvalid is used when certificate provisioning cannot proceed
	// because the domain specification is invalid.
	DomainCertificateReadyReasonInvalid DomainConditionCertificateReadyReason = "DomainInvalid"
)

// Reasons for DNSConfigured condition
const (
	// DomainDNSConfiguredReasonConfigured is used when DNS is properly configured for the domain.
	DomainDNSConfiguredReasonConfigured DomainConditionDNSConfiguredReason = "DomainCreated"

	// DomainDNSConfiguredReasonNgrokManaged is used when the domain is ngrok-managed and
	// DNS is automatically configured by ngrok.
	DomainDNSConfiguredReasonNgrokManaged DomainConditionDNSConfiguredReason = "NgrokManaged"

	// DomainDNSConfiguredReasonProvisioningError is used when there is an error with DNS
	// configuration for the domain.
	DomainDNSConfiguredReasonProvisioningError DomainConditionDNSConfiguredReason = "ProvisioningError"

	// DomainDNSConfiguredReasonCreationFailed is used when DNS configuration cannot proceed
	// because the domain creation failed.
	DomainDNSConfiguredReasonCreationFailed DomainConditionDNSConfiguredReason = "DomainCreationFailed"

	// DomainDNSConfiguredReasonInvalid is used when DNS configuration cannot proceed
	// because the domain specification is invalid.
	DomainDNSConfiguredReasonInvalid DomainConditionDNSConfiguredReason = "DomainInvalid"
)

// Reasons for Progressing condition
const (
	// DomainReasonProvisioning is used when the domain is actively being provisioned,
	// with certificate or DNS setup in progress.
	DomainReasonProvisioning DomainConditionProgressingReason = "Provisioning"
)
