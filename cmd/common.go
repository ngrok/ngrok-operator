package cmd

import (
	"fmt"
	"os"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"k8s.io/utils/ptr"
)

func validateDomainReclaimPolicy(policy string) (*ingressv1alpha1.DomainReclaimPolicy, error) {
	switch policy {
	case string(ingressv1alpha1.DomainReclaimPolicyDelete):
		return ptr.To(ingressv1alpha1.DomainReclaimPolicyDelete), nil
	case string(ingressv1alpha1.DomainReclaimPolicyRetain):
		return ptr.To(ingressv1alpha1.DomainReclaimPolicyRetain), nil
	default:
		return nil, fmt.Errorf("invalid default domain reclaim policy: %s. Allowed Values are: %v",
			policy,
			[]ingressv1alpha1.DomainReclaimPolicy{
				ingressv1alpha1.DomainReclaimPolicyDelete,
				ingressv1alpha1.DomainReclaimPolicyRetain,
			},
		)
	}
}

func InKubernetes() bool {
	// The presence of the service account token file indicates we are running in Kubernetes
	const serviceAccountTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"

	_, hasServiceHost := os.LookupEnv("KUBERNETES_SERVICE_HOST")

	_, err := os.Stat(serviceAccountTokenPath)
	return err == nil && hasServiceHost
}

const (
	serviceAccountNamespacePath = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
)

// GetCurrentNamespace returns the namespace of the current pod if running in Kubernetes,
// or "" if not running in Kubernetes.
func GetCurrentNamespace() string {
	if !InKubernetes() {
		return ""
	}

	data, err := os.ReadFile(serviceAccountNamespacePath)
	if err != nil {
		return ""
	}

	return string(data)
}

const (
	deploymentNameEnvVar = "DEPLOYMENT_NAME"
)

// GetDeploymentName returns the name of the current deployment from the DEPLOYMENT_NAME environment variable,
// or "" if not set.
func GetDeploymentName() string {
	deploymentName, exists := os.LookupEnv(deploymentNameEnvVar)
	if !exists {
		return ""
	}

	return deploymentName
}
