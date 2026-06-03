/*
MIT License

Copyright (c) 2024 ngrok, Inc.

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

package v1

// This file provides in-memory translation from the legacy v1alpha1 groups
// (ngrok.k8s.ngrok.com, ingress.k8s.ngrok.com, bindings.k8s.ngrok.com) to the
// consolidated ngrok.com/v1 group. It is the operator's runtime support for
// reconciling old-shape CRs during the 0.24 migration window and the basis for
// the adoption pass and migration tool. All of this is removed in 1.0.
//
// Translators copy ObjectMeta name/namespace/labels/annotations and the spec
// (translated to the v1 shape) plus status. They are pure functions: metadata
// parse failures are swallowed (metadata set to nil) so reconciliation can
// continue; callers that want to surface a MetadataInvalid condition should call
// ParseMetadataString directly.

import (
	"encoding/json"

	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ParseMetadataString parses the legacy JSON-encoded metadata string into a
// map. Empty input yields a nil map with no error. Invalid JSON returns the
// parse error so the caller can surface a MetadataInvalid condition.
func ParseMetadataString(s string) (map[string]string, error) {
	if s == "" {
		return nil, nil
	}
	m := map[string]string{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, err
	}
	return m, nil
}

// parseMetadataBestEffort parses metadata, returning nil on any error.
func parseMetadataBestEffort(s string) map[string]string {
	m, err := ParseMetadataString(s)
	if err != nil {
		return nil
	}
	return m
}

func copyObjectMeta(in metav1.ObjectMeta) metav1.ObjectMeta {
	out := metav1.ObjectMeta{
		Name:      in.Name,
		Namespace: in.Namespace,
	}
	if in.Labels != nil {
		out.Labels = make(map[string]string, len(in.Labels))
		for k, v := range in.Labels {
			out.Labels[k] = v
		}
	}
	if in.Annotations != nil {
		out.Annotations = make(map[string]string, len(in.Annotations))
		for k, v := range in.Annotations {
			out.Annotations[k] = v
		}
	}
	return out
}

func refOptNamespaceFromV1Alpha1(in *ngrokv1alpha1.K8sObjectRefOptionalNamespace) *K8sObjectRefOptionalNamespace {
	if in == nil {
		return nil
	}
	return &K8sObjectRefOptionalNamespace{Name: in.Name, Namespace: in.Namespace}
}

func refFromV1Alpha1(in *ngrokv1alpha1.K8sObjectRef) *K8sObjectRef {
	if in == nil {
		return nil
	}
	return &K8sObjectRef{Name: in.Name}
}

// CloudEndpointFromV1Alpha1 translates a legacy CloudEndpoint to the v1 shape.
func CloudEndpointFromV1Alpha1(in *ngrokv1alpha1.CloudEndpoint) *CloudEndpoint {
	if in == nil {
		return nil
	}
	out := &CloudEndpoint{
		ObjectMeta: copyObjectMeta(in.ObjectMeta),
		Spec: CloudEndpointSpec{
			URL:            in.Spec.URL,
			PoolingEnabled: in.Spec.PoolingEnabled,
			Description:    in.Spec.Description,
			Metadata:       parseMetadataBestEffort(in.Spec.Metadata),
			Bindings:       in.Spec.Bindings,
		},
		Status: CloudEndpointStatus{
			ID:         in.Status.ID,
			DomainRef:  refOptNamespaceFromV1Alpha1(in.Status.DomainRef),
			Conditions: in.Status.Conditions,
		},
	}

	// Unify the split v1alpha1 trafficPolicyName / trafficPolicy fields into the
	// single v1 TrafficPolicyCfg. If both are set, prefer the targetRef, matching
	// the existing controller's runtime precedence.
	switch {
	case in.Spec.TrafficPolicyName != "":
		out.Spec.TrafficPolicy = &TrafficPolicyCfg{
			TargetRef: &K8sObjectRefOptionalNamespace{Name: in.Spec.TrafficPolicyName},
		}
	case in.Spec.TrafficPolicy != nil:
		out.Spec.TrafficPolicy = &TrafficPolicyCfg{Inline: in.Spec.TrafficPolicy.Policy}
	}

	return out
}

// AgentEndpointFromV1Alpha1 translates a legacy AgentEndpoint to the v1 shape.
func AgentEndpointFromV1Alpha1(in *ngrokv1alpha1.AgentEndpoint) *AgentEndpoint {
	if in == nil {
		return nil
	}
	out := &AgentEndpoint{
		ObjectMeta: copyObjectMeta(in.ObjectMeta),
		Spec: AgentEndpointSpec{
			URL:         in.Spec.URL,
			Upstream:    endpointUpstreamFromV1Alpha1(in.Spec.Upstream),
			Description: in.Spec.Description,
			Metadata:    parseMetadataBestEffort(in.Spec.Metadata),
			Bindings:    in.Spec.Bindings,
		},
		Status: AgentEndpointStatus{
			AssignedURL:           in.Status.AssignedURL,
			AttachedTrafficPolicy: in.Status.AttachedTrafficPolicy,
			DomainRef:             refOptNamespaceFromV1Alpha1(in.Status.DomainRef),
			Conditions:            in.Status.Conditions,
		},
	}

	if in.Spec.TrafficPolicy != nil {
		tp := &TrafficPolicyCfg{Inline: in.Spec.TrafficPolicy.Inline}
		if in.Spec.TrafficPolicy.Reference != nil {
			tp.Inline = nil
			tp.TargetRef = &K8sObjectRefOptionalNamespace{Name: in.Spec.TrafficPolicy.Reference.Name}
		}
		out.Spec.TrafficPolicy = tp
	}

	for _, ref := range in.Spec.ClientCertificateRefs {
		out.Spec.ClientCertificateRefs = append(out.Spec.ClientCertificateRefs,
			K8sObjectRefOptionalNamespace{Name: ref.Name, Namespace: ref.Namespace})
	}

	out.Spec.TLSTermination = tlsTerminationFromV1Alpha1(in.Spec.TLSTermination)

	return out
}

func endpointUpstreamFromV1Alpha1(in ngrokv1alpha1.EndpointUpstream) EndpointUpstream {
	out := EndpointUpstream{URL: in.URL}
	if in.Protocol != nil {
		p := ApplicationProtocol(*in.Protocol)
		out.Protocol = &p
	}
	if in.ProxyProtocolVersion != nil {
		v := ProxyProtocolVersion(*in.ProxyProtocolVersion)
		out.ProxyProtocolVersion = &v
	}
	return out
}

func tlsTerminationFromV1Alpha1(in *ngrokv1alpha1.EndpointTLSTermination) *EndpointTLSTermination {
	if in == nil {
		return nil
	}
	out := &EndpointTLSTermination{
		ServerCertificateRef: K8sObjectRef{Name: in.ServerCertificateRef.Name},
	}
	if in.MutualTLS != nil {
		out.MutualTLS = &EndpointMutualTLS{
			ClientCAsRef: K8sObjectRef{Name: in.MutualTLS.ClientCAsRef.Name},
			Mode:         EndpointMutualTLSMode(in.MutualTLS.Mode),
		}
	}
	return out
}

// DomainFromV1Alpha1 translates a legacy Domain to the v1 shape. spec.region is
// dropped (removed in v1) and the snake_case/CNAME-casing field renames are
// applied.
func DomainFromV1Alpha1(in *ingressv1alpha1.Domain) *Domain {
	if in == nil {
		return nil
	}
	out := &Domain{
		ObjectMeta: copyObjectMeta(in.ObjectMeta),
		Spec: DomainSpec{
			Description:   in.Spec.Description,
			Metadata:      parseMetadataBestEffort(in.Spec.Metadata),
			Domain:        in.Spec.Domain,
			ResolvesTo:    domainResolvesToFromV1Alpha1(in.Spec.ResolvesTo),
			ReclaimPolicy: DomainReclaimPolicy(in.Spec.ReclaimPolicy),
		},
		Status: DomainStatus{
			ID:                       in.Status.ID,
			Domain:                   in.Status.Domain,
			ResolvesTo:               domainResolvesToFromV1Alpha1(in.Status.ResolvesTo),
			CNAMETarget:              in.Status.CNAMETarget,
			ACMEChallengeCNAMETarget: in.Status.ACMEChallengeCNAMETarget,
			Conditions:               in.Status.Conditions,
		},
	}

	if in.Status.Certificate != nil {
		out.Status.Certificate = &DomainStatusCertificateInfo{ID: in.Status.Certificate.ID}
	}
	if in.Status.CertificateManagementPolicy != nil {
		out.Status.CertificateManagementPolicy = &DomainStatusCertificateManagementPolicy{
			Authority:      in.Status.CertificateManagementPolicy.Authority,
			PrivateKeyType: in.Status.CertificateManagementPolicy.PrivateKeyType,
		}
	}
	if in.Status.CertificateManagementStatus != nil {
		out.Status.CertificateManagementStatus = &DomainStatusCertificateManagementStatus{
			RenewsAt: in.Status.CertificateManagementStatus.RenewsAt,
		}
		if pj := in.Status.CertificateManagementStatus.ProvisioningJob; pj != nil {
			out.Status.CertificateManagementStatus.ProvisioningJob = &DomainStatusProvisioningJob{
				ErrorCode: pj.ErrorCode,
				Message:   pj.Message,
				StartedAt: pj.StartedAt,
				RetriesAt: pj.RetriesAt,
			}
		}
	}

	return out
}

func domainResolvesToFromV1Alpha1(in *[]ingressv1alpha1.DomainResolvesToEntry) *[]DomainResolvesToEntry {
	if in == nil {
		return nil
	}
	out := make([]DomainResolvesToEntry, 0, len(*in))
	for _, e := range *in {
		out = append(out, DomainResolvesToEntry{Value: e.Value})
	}
	return &out
}

// IPPolicyFromV1Alpha1 translates a legacy IPPolicy to the v1 shape.
func IPPolicyFromV1Alpha1(in *ingressv1alpha1.IPPolicy) *IPPolicy {
	if in == nil {
		return nil
	}
	out := &IPPolicy{
		ObjectMeta: copyObjectMeta(in.ObjectMeta),
		Spec: IPPolicySpec{
			Description: in.Spec.Description,
			Metadata:    parseMetadataBestEffort(in.Spec.Metadata),
		},
		Status: IPPolicyStatus{
			ID:         in.Status.ID,
			Conditions: in.Status.Conditions,
		},
	}
	for _, r := range in.Spec.Rules {
		out.Spec.Rules = append(out.Spec.Rules, IPPolicyRule{
			Description: r.Description,
			Metadata:    parseMetadataBestEffort(r.Metadata),
			CIDR:        r.CIDR,
			Action:      r.Action,
		})
	}
	for _, r := range in.Status.Rules {
		out.Status.Rules = append(out.Status.Rules, IPPolicyRuleStatus{
			ID:     r.ID,
			CIDR:   r.CIDR,
			Action: r.Action,
		})
	}
	return out
}

// KubernetesOperatorFromV1Alpha1 translates a legacy KubernetesOperator to v1.
func KubernetesOperatorFromV1Alpha1(in *ngrokv1alpha1.KubernetesOperator) *KubernetesOperator {
	if in == nil {
		return nil
	}
	out := &KubernetesOperator{
		ObjectMeta: copyObjectMeta(in.ObjectMeta),
		Spec: KubernetesOperatorSpec{
			Description:     in.Spec.Description,
			Metadata:        parseMetadataBestEffort(in.Spec.Metadata),
			EnabledFeatures: in.Spec.EnabledFeatures,
			Region:          in.Spec.Region,
		},
		Status: KubernetesOperatorStatus{
			ID:                       in.Status.ID,
			URI:                      in.Status.URI,
			RegistrationStatus:       in.Status.RegistrationStatus,
			RegistrationErrorCode:    in.Status.RegistrationErrorCode,
			RegistrationErrorMessage: in.Status.RegistrationErrorMessage,
			EnabledFeatures:          in.Status.EnabledFeatures,
			BindingsIngressEndpoint:  in.Status.BindingsIngressEndpoint,
			DrainStatus:              DrainStatus(in.Status.DrainStatus),
			DrainMessage:             in.Status.DrainMessage,
			DrainProgress:            in.Status.DrainProgress,
			DrainErrors:              in.Status.DrainErrors,
		},
	}
	if in.Spec.Deployment != nil {
		out.Spec.Deployment = &KubernetesOperatorDeployment{
			Name:      in.Spec.Deployment.Name,
			Namespace: in.Spec.Deployment.Namespace,
			Version:   in.Spec.Deployment.Version,
		}
	}
	if in.Spec.Binding != nil {
		out.Spec.Binding = &KubernetesOperatorBinding{
			EndpointSelectors: in.Spec.Binding.EndpointSelectors,
			IngressEndpoint:   in.Spec.Binding.IngressEndpoint,
			TlsSecretName:     in.Spec.Binding.TlsSecretName,
		}
	}
	if in.Spec.Drain != nil {
		out.Spec.Drain = &DrainConfig{Policy: DrainPolicy(in.Spec.Drain.Policy)}
	}
	return out
}

// TrafficPolicyFromV1Alpha1 translates a legacy NgrokTrafficPolicy to the v1
// TrafficPolicy. The v1alpha1 status.policy field is dropped (v1 has no status).
func TrafficPolicyFromV1Alpha1(in *ngrokv1alpha1.NgrokTrafficPolicy) *TrafficPolicy {
	if in == nil {
		return nil
	}
	return &TrafficPolicy{
		ObjectMeta: copyObjectMeta(in.ObjectMeta),
		Spec:       TrafficPolicySpec{Policy: in.Spec.Policy},
	}
}

// BoundEndpointFromV1Alpha1 translates a legacy BoundEndpoint to the v1 shape.
// The deprecated spec.endpointURI field is collapsed into spec.endpointURL via
// the GetEndpointURL helper.
func BoundEndpointFromV1Alpha1(in *bindingsv1alpha1.BoundEndpoint) *BoundEndpoint {
	if in == nil {
		return nil
	}
	out := &BoundEndpoint{
		ObjectMeta: copyObjectMeta(in.ObjectMeta),
		Spec: BoundEndpointSpec{
			EndpointURL: in.Spec.GetEndpointURL(),
			Scheme:      in.Spec.Scheme,
			Port:        in.Spec.Port,
			Target: EndpointTarget{
				Service:   in.Spec.Target.Service,
				Namespace: in.Spec.Target.Namespace,
				Protocol:  in.Spec.Target.Protocol,
				Port:      in.Spec.Target.Port,
				Metadata: TargetMetadata{
					Labels:      in.Spec.Target.Metadata.Labels,
					Annotations: in.Spec.Target.Metadata.Annotations,
				},
			},
		},
		Status: BoundEndpointStatus{
			HashedName:         in.Status.HashedName,
			EndpointsSummary:   in.Status.EndpointsSummary,
			Conditions:         in.Status.Conditions,
			TargetServiceRef:   refOptNamespaceFromV1Alpha1(in.Status.TargetServiceRef),
			UpstreamServiceRef: refFromV1Alpha1(in.Status.UpstreamServiceRef),
		},
	}
	for _, e := range in.Status.Endpoints {
		out.Status.Endpoints = append(out.Status.Endpoints, BindingEndpoint{Ref: e.Ref})
	}
	return out
}
