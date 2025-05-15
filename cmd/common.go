package cmd

import (
	"fmt"

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
