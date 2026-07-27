package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
