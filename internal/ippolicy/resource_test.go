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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
)

// ippolicyResourceCases builds one instance of every kind that must satisfy
// IPPolicyResource, pre-populated with the same field values so the table
// below exercises identical behavior across kinds. Adding a third kind later
// means adding one entry here, not a parallel test function.
func ippolicyResourceCases() map[string]IPPolicyResource {
	return map[string]IPPolicyResource{
		"canonical": &ngrokv1.IPPolicy{
			Spec: ngrokv1.IPPolicySpec{
				Description: "test policy",
				Metadata:    map[string]string{"owned-by": "ngrok-operator"},
				Rules: []ngrokv1.IPPolicyRule{
					{CIDR: "10.0.0.0/8", Action: "allow"},
				},
			},
			Status: ngrokv1.IPPolicyStatus{ID: "ipp_123"},
		},
		// LEGACY-ippolicy-kind: delete this case at cleanup.
		"legacy": &ingressv1alpha1.IPPolicy{
			Spec: ingressv1alpha1.IPPolicySpec{
				Description: "test policy",
				Metadata:    json.RawMessage(`{"owned-by":"ngrok-operator"}`),
				Rules: []ingressv1alpha1.IPPolicyRule{
					{CIDR: "10.0.0.0/8", Action: "allow"},
				},
			},
			Status: ingressv1alpha1.IPPolicyStatus{ID: "ipp_123"},
		},
	}
}

// TestIPPolicyResource_Accessors runs the same assertions against both kinds
// via the IPPolicyResource interface only, so the two cannot drift: a field
// only one kind exposes correctly would fail here for that kind alone.
func TestIPPolicyResource_Accessors(t *testing.T) {
	for name, res := range ippolicyResourceCases() {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, "test policy", res.GetDescription())
			assert.Equal(t, map[string]string{"owned-by": "ngrok-operator"}, res.GetMetadata())

			rulesJSON, err := res.GetRulesJSON()
			require.NoError(t, err)
			assert.JSONEq(t, `[{"cidr":"10.0.0.0/8","action":"allow"}]`, string(rulesJSON))

			assert.Equal(t, "ipp_123", res.GetID())
			res.SetID("ipp_456")
			assert.Equal(t, "ipp_456", res.GetID())

			assert.Equal(t, int64(0), res.GetObservedGeneration())
			res.SetObservedGeneration(3)
			assert.Equal(t, int64(3), res.GetObservedGeneration())

			conditions := res.GetConditions()
			require.NotNil(t, conditions)
			*conditions = append(*conditions, metav1.Condition{
				Type:    "Ready",
				Status:  metav1.ConditionTrue,
				Reason:  "IPPolicyActive",
				Message: "IP Policy is active",
			})
			assert.Len(t, *res.GetConditions(), 1, "GetConditions must return a pointer into the live status, not a copy")
		})
	}
}

// newIPPolicyResource allocates a fresh, empty instance of kind T the same
// way generic reconciler code would via the IPPolicyResourcePtr constraint.
// This is a compile-time exercise as much as a runtime one: if either kind
// stops satisfying the constraint, this fails to compile rather than to run.
func newIPPolicyResource[T any, PT IPPolicyResourcePtr[T]]() PT {
	return PT(new(T))
}

func TestIPPolicyResourcePtr_AllocatesEachKind(t *testing.T) {
	canonical := newIPPolicyResource[ngrokv1.IPPolicy]()
	assert.Equal(t, "", canonical.GetDescription())

	// LEGACY-ippolicy-kind: delete this assertion at cleanup.
	legacy := newIPPolicyResource[ingressv1alpha1.IPPolicy]()
	assert.Equal(t, "", legacy.GetDescription())
}
