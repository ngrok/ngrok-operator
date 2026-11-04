package v1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestKubernetesOperatorStatusEnabledFeaturesMarshalsAsArray(t *testing.T) {
	status := KubernetesOperatorStatus{EnabledFeatures: []string{"ingress", "bindings"}}

	data, err := json.Marshal(status)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"enabledFeatures":["ingress","bindings"]`)
}

func TestKubernetesOperatorSpecMetadataRoundTrips(t *testing.T) {
	spec := KubernetesOperatorSpec{Metadata: map[string]string{"owned-by": "ngrok-operator"}}

	data, err := json.Marshal(spec)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"metadata":{"owned-by":"ngrok-operator"}`)

	var decoded KubernetesOperatorSpec
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, spec.Metadata, decoded.Metadata)
}

func TestKubernetesOperatorGetDrainPolicy(t *testing.T) {
	tests := []struct {
		name  string
		drain *DrainConfig
		want  DrainPolicy
	}{
		{name: "unset defaults to retain", want: DrainPolicyRetain},
		{name: "empty policy defaults to retain", drain: &DrainConfig{}, want: DrainPolicyRetain},
		{name: "delete", drain: &DrainConfig{Policy: DrainPolicyDelete}, want: DrainPolicyDelete},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ko := &KubernetesOperator{Spec: KubernetesOperatorSpec{Drain: tt.drain}}
			assert.Equal(t, tt.want, ko.GetDrainPolicy())
		})
	}
}
func TestKubernetesOperatorIsDrainComplete(t *testing.T) {
	tests := []struct {
		name      string
		condition *metav1.Condition
		want      bool
	}{
		{name: "absent", want: false},
		{
			name: "still draining",
			condition: &metav1.Condition{
				Type:   KubernetesOperatorConditionDraining,
				Status: metav1.ConditionTrue,
				Reason: KubernetesOperatorReasonDrainInProgress,
			},
			want: false,
		},
		{
			name: "false for another reason",
			condition: &metav1.Condition{
				Type:   KubernetesOperatorConditionDraining,
				Status: metav1.ConditionFalse,
				Reason: "NotStarted",
			},
			want: false,
		},
		{
			name: "completed",
			condition: &metav1.Condition{
				Type:   KubernetesOperatorConditionDraining,
				Status: metav1.ConditionFalse,
				Reason: KubernetesOperatorReasonDrainCompleted,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ko := &KubernetesOperator{}
			if tt.condition != nil {
				ko.Status.Conditions = []metav1.Condition{*tt.condition}
			}
			assert.Equal(t, tt.want, ko.IsDrainComplete())
		})
	}
}
