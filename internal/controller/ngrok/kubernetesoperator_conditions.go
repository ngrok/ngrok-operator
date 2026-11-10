package ngrok

import (
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	"github.com/ngrok/ngrok-operator/internal/controller/conditions"
)

// Condition type and reason constants live in the api package
// (api/ngrok/v1alpha1/kubernetesoperator_types.go) because the drain package
// also sets these conditions and cannot import this package without a cycle.

// setKubernetesOperatorRegisteredCondition sets the Registered condition
func setKubernetesOperatorRegisteredCondition(ko *ngrokv1.KubernetesOperator, registered bool, reason, message string) {
	conditions.Set(&ko.Status.Conditions, ko.Generation, ngrokv1.KubernetesOperatorConditionRegistered, registered, reason, message)
}

// setKubernetesOperatorReadyCondition sets the Ready condition
func setKubernetesOperatorReadyCondition(ko *ngrokv1.KubernetesOperator, ready bool, reason, message string) {
	conditions.Set(&ko.Status.Conditions, ko.Generation, ngrokv1.KubernetesOperatorConditionReady, ready, reason, message)
}

// setKubernetesOperatorDrainingCondition sets the Draining condition
func setKubernetesOperatorDrainingCondition(ko *ngrokv1.KubernetesOperator, draining bool, reason, message string) {
	conditions.Set(&ko.Status.Conditions, ko.Generation, ngrokv1.KubernetesOperatorConditionDraining, draining, reason, message)
}
