package ngrok

import (
	"fmt"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	"github.com/ngrok/ngrok-operator/internal/controller/conditions"
	"github.com/ngrok/ngrok-operator/internal/util"
)

// Shared TrafficPolicy condition vocabulary. Both the canonical
// ngrok.com/v1 TrafficPolicy reconciler (this file) and the deprecated
// ngrok.k8s.ngrok.com/v1alpha1 NgrokTrafficPolicy reconciler
// (ngroktrafficpolicy_conditions.go) write these condition types/reasons so
// users see the same vocabulary regardless of which kind they applied.
// The constants live here — not in the LEGACY-tagged sibling — so the
// cleanup deletion of the sibling doesn't strand them.
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

// setV1TrafficPolicyConditions is the ngrok.com/v1 twin of
// setTrafficPolicyConditions. It writes the shared Valid/Ready conditions on
// the canonical TrafficPolicy kind.
func setV1TrafficPolicyConditions(tp *ngrokv1.TrafficPolicy, parsed util.TrafficPolicy, parseErr error) {
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
