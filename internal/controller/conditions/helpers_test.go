package conditions

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestConditionType is a test type for condition types
type TestConditionType string

const (
	TestConditionReady      TestConditionType = "Ready"
	TestConditionInProgress TestConditionType = "InProgress"
)

func TestFindCondition(t *testing.T) {
	conditions := []metav1.Condition{
		{
			Type:   "Ready",
			Status: metav1.ConditionTrue,
			Reason: "AllGood",
		},
		{
			Type:   "InProgress",
			Status: metav1.ConditionFalse,
			Reason: "Waiting",
		},
	}

	tests := []struct {
		name      string
		condType  TestConditionType
		wantFound bool
		wantType  string
	}{
		{
			name:      "find existing condition",
			condType:  TestConditionReady,
			wantFound: true,
			wantType:  "Ready",
		},
		{
			name:      "find another existing condition",
			condType:  TestConditionInProgress,
			wantFound: true,
			wantType:  "InProgress",
		},
		{
			name:      "condition not found",
			condType:  TestConditionType("NotExists"),
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FindCondition(conditions, tt.condType)
			if tt.wantFound {
				if result == nil {
					t.Errorf("FindCondition() = nil, want non-nil")
					return
				}
				if result.Type != tt.wantType {
					t.Errorf("FindCondition() Type = %v, want %v", result.Type, tt.wantType)
				}
			} else if result != nil {
				t.Errorf("FindCondition() = %v, want nil", result)
			}
		})
	}
}

func TestFindCondition_EmptyList(t *testing.T) {
	var conditions []metav1.Condition
	result := FindCondition(conditions, TestConditionReady)
	if result != nil {
		t.Errorf("FindCondition() on empty list = %v, want nil", result)
	}
}
