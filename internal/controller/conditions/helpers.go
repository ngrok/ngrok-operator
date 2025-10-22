package conditions

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FindCondition is a generic helper that finds a condition in a list of conditions.
// It accepts typed condition types (e.g., DomainConditionType, IPPolicyConditionType)
// and eliminates the need for explicit string() casting at call sites.
//
// Example usage:
//
//	condition := FindCondition(domain.Status.Conditions, ingressv1alpha1.DomainConditionReady)
//	if condition != nil && condition.Status == metav1.ConditionTrue {
//	    // Domain is ready
//	}
func FindCondition[T ~string](conditions []metav1.Condition, condType T) *metav1.Condition {
	return meta.FindStatusCondition(conditions, string(condType))
}
