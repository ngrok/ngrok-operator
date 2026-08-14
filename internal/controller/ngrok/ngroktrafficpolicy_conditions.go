// LEGACY-trafficpolicy-kind: delete this whole file at cleanup. The shared
// Condition*/Reason* constants live in trafficpolicy_conditions.go (the
// canonical ngrok.com/v1 sibling) so they survive the deletion.

package ngrok

import (
	"fmt"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller/conditions"
	"github.com/ngrok/ngrok-operator/internal/util"
)

// setTrafficPolicyConditions sets the Valid and Ready conditions from the
// result of parsing spec.policy. Both conditions share the same reason so
// deprecation warnings surface in the Ready-based printer columns.
func setTrafficPolicyConditions(tp *ngrokv1alpha1.NgrokTrafficPolicy, parsed util.TrafficPolicy, parseErr error) {
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

	conditions.Set(&tp.Status.Conditions, tp.Generation, ConditionTrafficPolicyValid, valid, reason, message)
	conditions.Set(&tp.Status.Conditions, tp.Generation, ConditionTrafficPolicyReady, valid, reason, message)
}
