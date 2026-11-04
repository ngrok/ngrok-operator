/*
MIT License

Copyright (c) 2026 ngrok, Inc.

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

package ippolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
)

// ipPolicyKinds returns a fresh, generation-1 instance of every kind, so each
// conditions test runs against both and the kinds cannot drift.
func ipPolicyKinds() map[string]IPPolicyResource {
	return map[string]IPPolicyResource{
		"canonical": &ngrokv1.IPPolicy{Name: "test-ip-policy", Generation: 1},
		// LEGACY-ippolicy-kind: delete this entry at cleanup.
		"legacy": &ingressv1alpha1.IPPolicy{Name: "test-ip-policy", Generation: 1},
	}
}

func TestSetConditions(t *testing.T) {
	setters := []struct {
		name          string
		set           func(IPPolicyResource, bool, string, string)
		conditionType string
		trueReason    string
		falseReason   string
	}{
		{"Ready", SetReadyCondition, ConditionIPPolicyReady, ReasonIPPolicyActive, ReasonIPPolicyCreationFailed},
		{"Created", SetCreatedCondition, ConditionIPPolicyCreated, ReasonIPPolicyCreated, ReasonIPPolicyCreationFailed},
		{"RulesConfigured", SetRulesConfiguredCondition, ConditionIPPolicyRulesConfigured, ReasonIPPolicyRulesConfigured, ReasonIPPolicyRulesConfigurationError},
	}
	for kind := range ipPolicyKinds() {
		for _, s := range setters {
			t.Run(kind+"/"+s.name, func(t *testing.T) {
				p := ipPolicyKinds()[kind]

				s.set(p, true, s.trueReason, "it worked")
				c := meta.FindStatusCondition(*p.GetConditions(), s.conditionType)
				require.NotNil(t, c)
				assert.Equal(t, metav1.ConditionTrue, c.Status)
				assert.Equal(t, s.trueReason, c.Reason)
				assert.Equal(t, "it worked", c.Message)
				assert.Equal(t, int64(1), c.ObservedGeneration)

				s.set(p, false, s.falseReason, "it failed")
				c = meta.FindStatusCondition(*p.GetConditions(), s.conditionType)
				require.NotNil(t, c)
				assert.Equal(t, metav1.ConditionFalse, c.Status)
				assert.Equal(t, s.falseReason, c.Reason)
				assert.Equal(t, "it failed", c.Message)
			})
		}
	}
}

func TestCalculateReadyCondition(t *testing.T) {
	cases := []struct {
		name            string
		created         bool
		rulesConfigured bool
		wantReady       metav1.ConditionStatus
		wantReason      string
	}{
		{"created and rules configured", true, true, metav1.ConditionTrue, ReasonIPPolicyActive},
		{"created, rules not configured", true, false, metav1.ConditionFalse, ReasonIPPolicyRulesConfigurationError},
		{"not created", false, true, metav1.ConditionFalse, ReasonIPPolicyCreationFailed},
	}
	for kind := range ipPolicyKinds() {
		for _, tc := range cases {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				p := ipPolicyKinds()[kind]
				SetCreatedCondition(p, tc.created, ReasonIPPolicyCreated, "")
				SetRulesConfiguredCondition(p, tc.rulesConfigured, ReasonIPPolicyRulesConfigured, "")

				CalculateReadyCondition(p)

				c := meta.FindStatusCondition(*p.GetConditions(), ConditionIPPolicyReady)
				require.NotNil(t, c)
				assert.Equal(t, tc.wantReady, c.Status)
				assert.Equal(t, tc.wantReason, c.Reason)
			})
		}
	}
}
