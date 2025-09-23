package util

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
)

// ErrInvalidTrafficPolicyConfig is returned when both reference and inline traffic policies are specified
var ErrInvalidTrafficPolicyConfig = errors.New("invalid TrafficPolicy configuration: both reference and inline are set")

// ResolveTrafficPolicy resolves a traffic policy specification to a JSON string.
// Returns empty string if no traffic policy is specified (not an error).
// Callers should set their own conditions based on results.
func ResolveTrafficPolicy(ctx context.Context, kube client.Client, recorder record.EventRecorder,
	namespace string, spec interface{}) (string, error) {
	
	// Handle nil spec (no traffic policy)
	if spec == nil {
		return "", nil
	}

	// Handle CloudEndpoint traffic policy spec  
	if cloudSpec, ok := spec.(*ngrokv1alpha1.NgrokTrafficPolicySpec); ok {
		if cloudSpec == nil {
			return "", nil
		}
		return resolveCloudTrafficPolicy(ctx, kube, recorder, namespace, cloudSpec)
	}

	// Handle AgentEndpoint traffic policy spec
	if agentSpec, ok := spec.(*ngrokv1alpha1.TrafficPolicyCfg); ok {
		if agentSpec == nil {
			return "", nil
		}
		return resolveAgentTrafficPolicy(ctx, kube, recorder, namespace, agentSpec)
	}

	return "", fmt.Errorf("unsupported traffic policy spec type: %T", spec)
}

// resolveCloudTrafficPolicy handles CloudEndpoint traffic policy resolution
func resolveCloudTrafficPolicy(ctx context.Context, kube client.Client, recorder record.EventRecorder,
	namespace string, spec *ngrokv1alpha1.NgrokTrafficPolicySpec) (string, error) {
	
	// Handle inline policy
	if spec.Policy != nil {
		policyBytes, err := spec.Policy.MarshalJSON()
		if err != nil {
			return "", fmt.Errorf("failed to marshal inline TrafficPolicy: %w", err)
		}
		return string(policyBytes), nil
	}

	return "", nil
}

// resolveAgentTrafficPolicy handles AgentEndpoint traffic policy resolution
func resolveAgentTrafficPolicy(ctx context.Context, kube client.Client, recorder record.EventRecorder,
	namespace string, spec *ngrokv1alpha1.TrafficPolicyCfg) (string, error) {
	
	// Ensure mutually exclusive fields are not both set
	if spec.Reference != nil && len(spec.Inline) > 0 {
		return "", ErrInvalidTrafficPolicyConfig
	}

	var policy string
	var err error

	switch spec.Type() {
	case ngrokv1alpha1.TrafficPolicyCfgType_Inline:
		policy = string(spec.Inline)
	case ngrokv1alpha1.TrafficPolicyCfgType_K8sRef:
		// Right now, we only support traffic policies that are in the same namespace
		policy, err = findTrafficPolicyByName(ctx, kube, recorder, spec.Reference.Name, namespace)
		if err != nil {
			return "", err
		}
	}

	return policy, nil
}

// findTrafficPolicyByName fetches the TrafficPolicy CRD from the API server and returns the JSON policy as a string
func findTrafficPolicyByName(ctx context.Context, kube client.Client, recorder record.EventRecorder, tpName, tpNamespace string) (string, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("name", tpName, "namespace", tpNamespace)

	// Create a TrafficPolicy object to store the fetched result
	tp := &ngrokv1alpha1.NgrokTrafficPolicy{}
	key := client.ObjectKey{Name: tpName, Namespace: tpNamespace}

	// Attempt to get the TrafficPolicy from the API server
	if err := kube.Get(ctx, key, tp); err != nil {
		recorder.Event(tp, "Warning", "TrafficPolicyNotFound", fmt.Sprintf("Failed to find TrafficPolicy %s", tpName))
		return "", err
	}

	// Convert the JSON policy to a string
	policyBytes, err := tp.Spec.Policy.MarshalJSON()
	if err != nil {
		log.Error(err, "failed to marshal TrafficPolicy JSON")
		return "", err
	}

	return string(policyBytes), nil
}

// GetCloudEndpointTrafficPolicyName returns the traffic policy name for CloudEndpoint specs
func GetCloudEndpointTrafficPolicyName(spec *ngrokv1alpha1.CloudEndpointSpec) string {
	if spec.TrafficPolicyName != "" {
		return spec.TrafficPolicyName
	}
	if spec.TrafficPolicy != nil {
		return "inline"
	}
	return "none"
}

// GetAgentEndpointTrafficPolicyName returns the traffic policy name for AgentEndpoint specs
func GetAgentEndpointTrafficPolicyName(spec *ngrokv1alpha1.AgentEndpointSpec) string {
	if spec.TrafficPolicy != nil && spec.TrafficPolicy.Reference != nil {
		return spec.TrafficPolicy.Reference.Name
	}
	if spec.TrafficPolicy != nil && len(spec.TrafficPolicy.Inline) > 0 {
		return "inline"
	}
	return "none"
}
