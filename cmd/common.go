package cmd

import (
	"fmt"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	"k8s.io/utils/ptr"
)

func validateDomainReclaimPolicy(policy string) (*ngrokv1.DomainReclaimPolicy, error) {
	switch policy {
	case string(ngrokv1.DomainReclaimPolicyDelete):
		return ptr.To(ngrokv1.DomainReclaimPolicyDelete), nil
	case string(ngrokv1.DomainReclaimPolicyRetain):
		return ptr.To(ngrokv1.DomainReclaimPolicyRetain), nil
	default:
		return nil, fmt.Errorf("invalid default domain reclaim policy: %s. Allowed Values are: %v",
			policy,
			[]ngrokv1.DomainReclaimPolicy{
				ngrokv1.DomainReclaimPolicyDelete,
				ngrokv1.DomainReclaimPolicyRetain,
			},
		)
	}
}
