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

import (
	"encoding/json"
	"testing"

	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	commonv1alpha1 "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestParseMetadataString(t *testing.T) {
	m, err := ParseMetadataString("")
	assert.NoError(t, err)
	assert.Nil(t, m)

	m, err = ParseMetadataString(`{"env":"prod","team":"platform"}`)
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"env": "prod", "team": "platform"}, m)

	_, err = ParseMetadataString(`{not valid json`)
	assert.Error(t, err)
}

func TestCloudEndpointFromV1Alpha1_trafficPolicyName(t *testing.T) {
	in := &ngrokv1alpha1.CloudEndpoint{
		ObjectMeta: metav1.ObjectMeta{Name: "ep", Namespace: "default"},
		Spec: ngrokv1alpha1.CloudEndpointSpec{
			URL:               "https://example.ngrok.app",
			TrafficPolicyName: "my-policy",
			Metadata:          `{"env":"prod"}`,
		},
		Status: ngrokv1alpha1.CloudEndpointStatus{ID: "ep_123"},
	}

	out := CloudEndpointFromV1Alpha1(in)
	require.NotNil(t, out)
	require.NotNil(t, out.Spec.TrafficPolicy)
	require.NotNil(t, out.Spec.TrafficPolicy.TargetRef)
	assert.Equal(t, "my-policy", out.Spec.TrafficPolicy.TargetRef.Name)
	assert.Nil(t, out.Spec.TrafficPolicy.Inline)
	assert.Equal(t, map[string]string{"env": "prod"}, out.Spec.Metadata)
	assert.Equal(t, "ep_123", out.Status.ID)
}

func TestCloudEndpointFromV1Alpha1_inlinePolicy(t *testing.T) {
	inline := json.RawMessage(`{"on_http_request":[]}`)
	in := &ngrokv1alpha1.CloudEndpoint{
		Spec: ngrokv1alpha1.CloudEndpointSpec{
			URL:           "https://example.ngrok.app",
			TrafficPolicy: &ngrokv1alpha1.NgrokTrafficPolicySpec{Policy: inline},
		},
	}
	out := CloudEndpointFromV1Alpha1(in)
	require.NotNil(t, out.Spec.TrafficPolicy)
	assert.Nil(t, out.Spec.TrafficPolicy.TargetRef)
	assert.JSONEq(t, string(inline), string(out.Spec.TrafficPolicy.Inline))
}

func TestCloudEndpointFromV1Alpha1_bothPreferTargetRef(t *testing.T) {
	in := &ngrokv1alpha1.CloudEndpoint{
		Spec: ngrokv1alpha1.CloudEndpointSpec{
			URL:               "https://example.ngrok.app",
			TrafficPolicyName: "my-policy",
			TrafficPolicy:     &ngrokv1alpha1.NgrokTrafficPolicySpec{Policy: json.RawMessage(`{}`)},
		},
	}
	out := CloudEndpointFromV1Alpha1(in)
	require.NotNil(t, out.Spec.TrafficPolicy.TargetRef)
	assert.Equal(t, "my-policy", out.Spec.TrafficPolicy.TargetRef.Name)
	assert.Nil(t, out.Spec.TrafficPolicy.Inline)
}

func TestCloudEndpointFromV1Alpha1_invalidMetadataBestEffort(t *testing.T) {
	in := &ngrokv1alpha1.CloudEndpoint{
		Spec: ngrokv1alpha1.CloudEndpointSpec{URL: "https://x", Metadata: `{bad json`},
	}
	out := CloudEndpointFromV1Alpha1(in)
	assert.Nil(t, out.Spec.Metadata, "invalid metadata should translate to nil, not crash")
}

func TestAgentEndpointFromV1Alpha1(t *testing.T) {
	proto := commonv1alpha1.ApplicationProtocol_HTTP2
	in := &ngrokv1alpha1.AgentEndpoint{
		Spec: ngrokv1alpha1.AgentEndpointSpec{
			URL: "tls://example.ngrok.app",
			Upstream: ngrokv1alpha1.EndpointUpstream{
				URL:      "http://localhost:8080",
				Protocol: &proto,
			},
			TrafficPolicy: &ngrokv1alpha1.TrafficPolicyCfg{
				Reference: &ngrokv1alpha1.K8sObjectRef{Name: "tp"},
			},
			Metadata: `{"team":"net"}`,
		},
		Status: ngrokv1alpha1.AgentEndpointStatus{AttachedTrafficPolicy: "tp"},
	}
	out := AgentEndpointFromV1Alpha1(in)
	require.NotNil(t, out.Spec.Upstream.Protocol)
	assert.Equal(t, ApplicationProtocol_HTTP2, *out.Spec.Upstream.Protocol)
	require.NotNil(t, out.Spec.TrafficPolicy.TargetRef)
	assert.Equal(t, "tp", out.Spec.TrafficPolicy.TargetRef.Name)
	assert.Equal(t, "tp", out.Status.AttachedTrafficPolicy)
	assert.Equal(t, map[string]string{"team": "net"}, out.Spec.Metadata)
}

func TestDomainFromV1Alpha1_renamesAndRegionDrop(t *testing.T) {
	in := &ingressv1alpha1.Domain{
		Spec: ingressv1alpha1.DomainSpec{
			Domain:        "example.com",
			Region:        "us", // dropped in v1
			ResolvesTo:    &[]ingressv1alpha1.DomainResolvesToEntry{{Value: "pop-1"}},
			ReclaimPolicy: ingressv1alpha1.DomainReclaimPolicyRetain,
			Metadata:      `{"owned-by":"x"}`,
		},
		Status: ingressv1alpha1.DomainStatus{
			ACMEChallengeCNAMETarget: ptr.To("acme.example.com"),
		},
	}
	out := DomainFromV1Alpha1(in)
	assert.Equal(t, "example.com", out.Spec.Domain)
	require.NotNil(t, out.Spec.ResolvesTo)
	assert.Equal(t, "pop-1", (*out.Spec.ResolvesTo)[0].Value)
	assert.Equal(t, DomainReclaimPolicyRetain, out.Spec.ReclaimPolicy)
	require.NotNil(t, out.Status.ACMEChallengeCNAMETarget)
	assert.Equal(t, "acme.example.com", *out.Status.ACMEChallengeCNAMETarget)
}

func TestKubernetesOperatorFromV1Alpha1_errorMessageRename(t *testing.T) {
	in := &ngrokv1alpha1.KubernetesOperator{
		Spec: ngrokv1alpha1.KubernetesOperatorSpec{Region: "global", Metadata: `{}`},
		Status: ngrokv1alpha1.KubernetesOperatorStatus{
			ID:                       "k8sop_1",
			RegistrationErrorMessage: "boom",
			DrainStatus:              ngrokv1alpha1.DrainStatusCompleted,
		},
	}
	out := KubernetesOperatorFromV1Alpha1(in)
	assert.Equal(t, "boom", out.Status.RegistrationErrorMessage)
	assert.Equal(t, DrainStatusCompleted, out.Status.DrainStatus)
}

func TestTrafficPolicyFromV1Alpha1_dropsStatus(t *testing.T) {
	in := &ngrokv1alpha1.NgrokTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "tp"},
		Spec:       ngrokv1alpha1.NgrokTrafficPolicySpec{Policy: json.RawMessage(`{"x":1}`)},
		Status:     ngrokv1alpha1.NgrokTrafficPolicyStatus{Policy: json.RawMessage(`{"x":1}`)},
	}
	out := TrafficPolicyFromV1Alpha1(in)
	assert.Equal(t, "tp", out.Name)
	assert.JSONEq(t, `{"x":1}`, string(out.Spec.Policy))
}

func TestBoundEndpointFromV1Alpha1_endpointURIFallback(t *testing.T) {
	in := &bindingsv1alpha1.BoundEndpoint{
		Spec: bindingsv1alpha1.BoundEndpointSpec{
			EndpointURI: "https://svc.ns:443", // deprecated field; URL empty
			Scheme:      "https",
			Port:        443,
			Target: bindingsv1alpha1.EndpointTarget{
				Service:   "svc",
				Namespace: "ns",
				Protocol:  "TCP",
				Port:      443,
			},
		},
	}
	out := BoundEndpointFromV1Alpha1(in)
	assert.Equal(t, "https://svc.ns:443", out.Spec.EndpointURL, "endpointURI should fall back into endpointURL")
}
