package ngrok

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/util"
)

func TestSetTrafficPolicyConditions(t *testing.T) {
	tests := []struct {
		name            string
		policy          string
		expectedStatus  metav1.ConditionStatus
		expectedReason  string
		messageContains string
	}{
		{
			name:            "valid policy",
			policy:          `{"on_http_request":[{"actions":[{"type":"deny"}]}]}`,
			expectedStatus:  metav1.ConditionTrue,
			expectedReason:  ReasonTrafficPolicyValid,
			messageContains: "valid",
		},
		{
			name:            "invalid policy JSON",
			policy:          `{"on_http_request":`,
			expectedStatus:  metav1.ConditionFalse,
			expectedReason:  ReasonTrafficPolicyParseFailed,
			messageContains: "Failed to parse",
		},
		{
			name:            "legacy directions",
			policy:          `{"inbound":[{"actions":[{"type":"deny"}]}]}`,
			expectedStatus:  metav1.ConditionTrue,
			expectedReason:  ReasonLegacyPolicyFormat,
			messageContains: "legacy directions",
		},
		{
			name:            "enabled field set",
			policy:          `{"enabled":true,"on_http_request":[{"actions":[{"type":"deny"}]}]}`,
			expectedStatus:  metav1.ConditionTrue,
			expectedReason:  ReasonEnabledDeprecated,
			messageContains: "'enabled' set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tp := &ngrokv1alpha1.NgrokTrafficPolicy{
				ObjectMeta: metav1.ObjectMeta{Generation: 3},
				Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
					Policy: json.RawMessage(tt.policy),
				},
			}

			parsed, parseErr := util.NewTrafficPolicyFromJson(tp.Spec.Policy)
			setTrafficPolicyConditions(tp, parsed, parseErr)

			for _, condType := range []string{ConditionTrafficPolicyReady, ConditionTrafficPolicyValid} {
				cond := meta.FindStatusCondition(tp.Status.Conditions, condType)
				require.NotNil(t, cond, "condition %s should be set", condType)
				assert.Equal(t, tt.expectedStatus, cond.Status)
				assert.Equal(t, tt.expectedReason, cond.Reason)
				assert.Contains(t, cond.Message, tt.messageContains)
				assert.Equal(t, int64(3), cond.ObservedGeneration)
			}
		})
	}
}

// TestStatusChangeDetection guards the Reconcile skip-write-if-unchanged logic
// (compare against a DeepCopy taken before mutating). meta.SetStatusCondition
// mutates an existing condition's fields in place, so a shallow copy of
// Status would share the same backing array and always compare equal to the
// post-mutation value, even when something actually changed.
func TestStatusChangeDetection(t *testing.T) {
	tp := &ngrokv1alpha1.NgrokTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{Generation: 1},
		Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
			Policy: json.RawMessage(`{"on_http_request":[{"actions":[{"type":"deny"}]}]}`),
		},
	}

	// First reconcile: conditions don't exist yet, must report changed.
	prevStatus := *tp.Status.DeepCopy()
	parsed, parseErr := util.NewTrafficPolicyFromJson(tp.Spec.Policy)
	setTrafficPolicyConditions(tp, parsed, parseErr)
	tp.SetObservedGeneration(tp.Generation)
	assert.False(t, reflect.DeepEqual(prevStatus, tp.Status), "initial condition set must be detected as a change")

	// Second reconcile with identical inputs: no real change, must be a no-op.
	prevStatus = *tp.Status.DeepCopy()
	parsed, parseErr = util.NewTrafficPolicyFromJson(tp.Spec.Policy)
	setTrafficPolicyConditions(tp, parsed, parseErr)
	tp.SetObservedGeneration(tp.Generation)
	assert.True(t, reflect.DeepEqual(prevStatus, tp.Status), "unchanged inputs must not be reported as a change")

	// Third reconcile with a legacy policy: reason/message change while
	// Status stays True — must still be detected (this is the case a
	// LastTransitionTime-based check would miss).
	prevStatus = *tp.Status.DeepCopy()
	tp.Spec.Policy = json.RawMessage(`{"inbound":[{"actions":[{"type":"deny"}]}]}`)
	parsed, parseErr = util.NewTrafficPolicyFromJson(tp.Spec.Policy)
	setTrafficPolicyConditions(tp, parsed, parseErr)
	tp.SetObservedGeneration(tp.Generation)
	assert.True(t, reflect.DeepEqual(metav1.ConditionTrue, meta.FindStatusCondition(tp.Status.Conditions, ConditionTrafficPolicyReady).Status),
		"Ready should still be true for a legacy-format policy")
	assert.False(t, reflect.DeepEqual(prevStatus, tp.Status), "reason/message change without a status flip must be detected")
}
