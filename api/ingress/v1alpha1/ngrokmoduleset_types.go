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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NgrokModuleSetModules struct {
	// CircuitBreaker configuration for this module set
	CircuitBreaker *EndpointCircuitBreaker `json:"circuitBreaker,omitempty"`
	// Compression configuration for this module set
	Compression *EndpointCompression `json:"compression,omitempty"`
	// Header configuration for this module set
	Headers *EndpointHeaders `json:"headers,omitempty"`
	// IPRestriction configuration for this module set
	IPRestriction *EndpointIPPolicy `json:"ipRestriction,omitempty"`
	// OAuth configuration for this module set
	OAuth *EndpointOAuth `json:"oauth,omitempty"`
	// Policy configuration for this module set
	Policy *EndpointPolicy `json:"policy,omitempty"`
	// OIDC configuration for this module set
	OIDC *EndpointOIDC `json:"oidc,omitempty"`
	// SAML configuration for this module set
	SAML *EndpointSAML `json:"saml,omitempty"`
	// TLSTermination configuration for this module set
	TLSTermination *EndpointTLSTermination `json:"tlsTermination,omitempty"`
	// MutualTLS configuration for this module set
	MutualTLS *EndpointMutualTLS `json:"mutualTLS,omitempty"`
	// WebhookVerification configuration for this module set
	WebhookVerification *EndpointWebhookVerification `json:"webhookVerification,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// NgrokModuleSet is the Schema for the ngrokmodules API
type NgrokModuleSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Modules NgrokModuleSetModules `json:"modules,omitempty"`
}

func (ms *NgrokModuleSet) Merge(o *NgrokModuleSet) {
	if o == nil {
		return
	}

	msmod := &ms.Modules
	omod := o.Modules

	if omod.CircuitBreaker != nil {
		msmod.CircuitBreaker = omod.CircuitBreaker
	}
	if omod.Compression != nil {
		msmod.Compression = omod.Compression
	}
	if omod.Headers != nil {
		msmod.Headers = omod.Headers
	}
	if omod.IPRestriction != nil {
		msmod.IPRestriction = omod.IPRestriction
	}
	if omod.OAuth != nil {
		msmod.OAuth = omod.OAuth
	}
	if omod.Policy != nil {
		msmod.Policy = omod.Policy
	}
	if omod.OIDC != nil {
		msmod.OIDC = omod.OIDC
	}
	if omod.SAML != nil {
		msmod.SAML = omod.SAML
	}
	if omod.TLSTermination != nil {
		msmod.TLSTermination = omod.TLSTermination
	}
	if omod.MutualTLS != nil {
		msmod.MutualTLS = omod.MutualTLS
	}
	if omod.WebhookVerification != nil {
		msmod.WebhookVerification = omod.WebhookVerification
	}
}

//+kubebuilder:object:root=true

// NgrokModuleSetList contains a list of NgrokModule
type NgrokModuleSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NgrokModuleSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NgrokModuleSet{}, &NgrokModuleSetList{})
}
