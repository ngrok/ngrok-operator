/*
MIT License

Copyright (c) 2022 ngrok, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package v1alpha1

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DomainReclaimPolicy string

const (
	DomainReclaimPolicyDelete DomainReclaimPolicy = "Delete"
	DomainReclaimPolicyRetain DomainReclaimPolicy = "Retain"
)

// DomainSpec defines the desired state of Domain
type DomainSpec struct {
	ngrokAPICommon `json:",inline"`

	// Domain is the domain name to reserve
	// +kubebuilder:validation:Required
	Domain string `json:"domain"`

	// Region is the region in which to reserve the domain
	// +kubebuilder:validation:Required
	Region string `json:"region,omitempty"`

	// DomainReclaimPolicy is the policy to use when the domain is deleted
	// +kubebuilder:validation:Enum=Delete;Retain
	// +kubebuilder:default=Delete
	ReclaimPolicy DomainReclaimPolicy `json:"reclaimPolicy,omitempty"`
}

// DomainStatus defines the observed state of Domain
type DomainStatus struct {

	// ID is the unique identifier of the domain
	ID string `json:"id,omitempty"`

	// Domain is the domain that was reserved
	Domain string `json:"domain,omitempty"`

	// Region is the region in which the domain was created
	Region string `json:"region,omitempty"`

	// CNAMETarget is the CNAME target for the domain
	CNAMETarget *string `json:"cnameTarget,omitempty"`

	// ACMEChallengeCNAMETarget is the CNAME target for ACME challenge (wildcards only)
	ACMEChallengeCNAMETarget *string `json:"acmeChallengeCnameTarget,omitempty"`

	// Certificate contains information about the TLS certificate
	Certificate *DomainStatusCertificateInfo `json:"certificate,omitempty"`

	// CertificateManagementPolicy contains the certificate management configuration
	CertificateManagementPolicy *DomainStatusCertificateManagementPolicy `json:"certificateManagementPolicy,omitempty"`

	// CertificateManagementStatus contains the certificate management status
	CertificateManagementStatus *DomainStatusCertificateManagementStatus `json:"certificateManagementStatus,omitempty"`

	// Conditions represent the latest available observations of the domain's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// DomainStatusCertificateInfo contains information about the TLS certificate for the domain
type DomainStatusCertificateInfo struct {
	// ID is the certificate ID
	ID string `json:"id"`
}

// DomainStatusCertificateManagementPolicy contains the certificate management configuration
type DomainStatusCertificateManagementPolicy struct {
	// Authority is the certificate authority (e.g., "letsencrypt")
	Authority string `json:"authority"`
	// PrivateKeyType is the private key type (e.g., "ecdsa")
	PrivateKeyType string `json:"privateKeyType"`
}

// DomainStatusCertificateManagementStatus contains the certificate management status
type DomainStatusCertificateManagementStatus struct {
	// RenewsAt is when the certificate will be renewed
	RenewsAt *metav1.Time `json:"renewsAt,omitempty"`
	// ProvisioningJob contains information about the current provisioning job
	ProvisioningJob *DomainStatusProvisioningJob `json:"provisioningJob,omitempty"`
}

// DomainStatusProvisioningJob contains information about a certificate provisioning job
type DomainStatusProvisioningJob struct {
	// ErrorCode indicates the type of error (e.g., "DNS_ERROR")
	ErrorCode string `json:"errorCode,omitempty"`
	// Message is a human-readable description of the current status
	Message string `json:"message,omitempty"`
	// StartedAt is when the provisioning job started
	StartedAt *metav1.Time `json:"startedAt,omitempty"`
	// RetriesAt is when the provisioning job will be retried
	RetriesAt *metav1.Time `json:"retriesAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ID",type=string,JSONPath=`.status.id`,description="Domain ID"
// +kubebuilder:printcolumn:name="Domain",type=string,JSONPath=`.status.domain`,description="Domain"
// +kubebuilder:printcolumn:name="Reclaim Policy",type=string,JSONPath=`.spec.reclaimPolicy`,description="Reclaim Policy"
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=='Ready')].status`,description="Domain Ready"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,description="Age"
// +kubebuilder:printcolumn:name="CNAME Target",type=string,JSONPath=`.status.cnameTarget`,description="CNAME Target",priority=2
// +kubebuilder:printcolumn:name="Region",type=string,JSONPath=`.status.region`,description="Region",priority=2

// Domain is the Schema for the domains API
type Domain struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DomainSpec   `json:"spec,omitempty"`
	Status DomainStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DomainList contains a list of ReservedDomain
type DomainList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Domain `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Domain{}, &DomainList{})
}

var domainNameForResourceNameReplacer = strings.NewReplacer(
	".", "-", // replace dots with dashes
	"*", "wildcard", // replace wildcard with the literal "wildcard"
)

func HyphenatedDomainNameFromURL(domain string) string {
	return domainNameForResourceNameReplacer.Replace(domain)
}
