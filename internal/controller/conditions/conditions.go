package conditions

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Set sets a condition on the provided conditions slice.
func Set(conditions *[]metav1.Condition, generation int64, condType string, ok bool, reason, message string) {
	status := metav1.ConditionTrue
	if !ok {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: generation,
	}

	meta.SetStatusCondition(conditions, condition)
}
