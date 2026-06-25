package conditions

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSet(t *testing.T) {
	tests := []struct {
		name       string
		conds      []metav1.Condition
		generation int64
		condType   string
		ok         bool
		reason     string
		message    string
		verify     func(t *testing.T, conds []metav1.Condition)
	}{
		{
			name:       "set new condition with ok = true",
			conds:      []metav1.Condition{},
			generation: 1,
			condType:   "Ready",
			ok:         true,
			reason:     "Initialized",
			message:    "Resource is ready",
			verify: func(t *testing.T, conds []metav1.Condition) {
				assert.Len(t, conds, 1)
				cond := conds[0]
				assert.Equal(t, "Ready", cond.Type)
				assert.Equal(t, metav1.ConditionTrue, cond.Status)
				assert.Equal(t, int64(1), cond.ObservedGeneration)
				assert.Equal(t, "Initialized", cond.Reason)
				assert.Equal(t, "Resource is ready", cond.Message)
				assert.NotZero(t, cond.LastTransitionTime)
			},
		},
		{
			name:       "set new condition with ok = false",
			conds:      []metav1.Condition{},
			generation: 2,
			condType:   "Ready",
			ok:         false,
			reason:     "Failed",
			message:    "Resource failed to initialize",
			verify: func(t *testing.T, conds []metav1.Condition) {
				assert.Len(t, conds, 1)
				cond := conds[0]
				assert.Equal(t, "Ready", cond.Type)
				assert.Equal(t, metav1.ConditionFalse, cond.Status)
				assert.Equal(t, int64(2), cond.ObservedGeneration)
				assert.Equal(t, "Failed", cond.Reason)
				assert.Equal(t, "Resource failed to initialize", cond.Message)
				assert.NotZero(t, cond.LastTransitionTime)
			},
		},
		{
			name: "update existing condition status and details",
			conds: []metav1.Condition{
				{
					Type:               "Ready",
					Status:             metav1.ConditionFalse,
					Reason:             "Failed",
					Message:            "Old error",
					ObservedGeneration: 1,
					LastTransitionTime: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
				},
			},
			generation: 2,
			condType:   "Ready",
			ok:         true,
			reason:     "Succeeded",
			message:    "Now OK",
			verify: func(t *testing.T, conds []metav1.Condition) {
				assert.Len(t, conds, 1)
				cond := conds[0]
				assert.Equal(t, "Ready", cond.Type)
				assert.Equal(t, metav1.ConditionTrue, cond.Status)
				assert.Equal(t, int64(2), cond.ObservedGeneration)
				assert.Equal(t, "Succeeded", cond.Reason)
				assert.Equal(t, "Now OK", cond.Message)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Set(&tt.conds, tt.generation, tt.condType, tt.ok, tt.reason, tt.message)
			tt.verify(t, tt.conds)
		})
	}
}
