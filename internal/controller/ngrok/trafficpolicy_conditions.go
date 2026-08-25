package ngrok

import (
	"fmt"

	"github.com/ngrok/ngrok-operator/internal/controller/conditions"
	trafficpolicypkg "github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	"github.com/ngrok/ngrok-operator/internal/util"
)

// Shared TrafficPolicy condition vocabulary, written identically for the
// canonical ngrok.com/v1 TrafficPolicy and the deprecated
// ngrok.k8s.ngrok.com/v1alpha1 NgrokTrafficPolicy so users see the same
// condition types and reasons regardless of which kind they applied.
const (
	// Condition types.
	ConditionTrafficPolicyReady = "Ready"
	ConditionTrafficPolicyValid = "Valid"

	// Condition reasons.
	ReasonTrafficPolicyValid       = "TrafficPolicyValid"
	ReasonTrafficPolicyParseFailed = "TrafficPolicyParseFailed"
	ReasonLegacyPolicyFormat       = "LegacyPolicyFormat"
	ReasonEnabledDeprecated        = "EnabledFieldDeprecated"
)

// setTrafficPolicyConditions sets the Valid and Ready conditions from the
// result of parsing spec.policy. Both conditions share the same reason so
// deprecation warnings surface in the Ready-based printer columns.
//
// It takes the kind-agnostic TrafficPolicyResource interface rather than a
// concrete type, so both served kinds get byte-identical condition handling
// from a single implementation.
func setTrafficPolicyConditions(tp trafficpolicypkg.TrafficPolicyResource, parsed util.TrafficPolicy, parseErr error) {
	valid := parseErr == nil
	reason := ReasonTrafficPolicyValid
	message := "Traffic policy is valid"

	switch {
	case parseErr != nil:
		reason = ReasonTrafficPolicyParseFailed
		message = fmt.Sprintf("Failed to parse traffic policy: %v", parseErr)
	case parsed.IsLegacyPolicy():
		reason = ReasonLegacyPolicyFormat
		message = "Traffic policy is using legacy directions: ['inbound', 'outbound']. Update to new phases: ['on_tcp_connect', 'on_http_request', 'on_http_response']"
	case parsed.Enabled() != nil:
		reason = ReasonEnabledDeprecated
		message = "Traffic policy has 'enabled' set. This is a legacy option that will stop being supported soon."
	}

	conditions.Set(tp.GetConditions(), tp.GetGeneration(), ConditionTrafficPolicyValid, valid, reason, message)
	conditions.Set(tp.GetConditions(), tp.GetGeneration(), ConditionTrafficPolicyReady, valid, reason, message)
}
