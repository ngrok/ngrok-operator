package ngrok

import (
	"encoding/json"
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
