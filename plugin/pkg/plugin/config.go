package plugin

import (
	"encoding/json"
	"fmt"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
)

const (
	labelRolloutName      = "k8s.ngrok.com/rollout-name"
	labelRolloutNamespace = "k8s.ngrok.com/rollout-namespace"
	labelRolloutRole      = "k8s.ngrok.com/rollout-role" // "stable" or "canary"

	// rolloutManagedAnnotation is placed on the operator-created AgentEndpoint or CloudEndpoint
	// when the plugin takes ownership of its traffic policy for Phase 2 routing. While present,
	// the operator reconciler preserves the traffic policy and this annotation rather than
	// overwriting them on each sync.
	rolloutManagedAnnotation = "k8s.ngrok.com/rollout-managed"

	// rolloutPolicyPrefixAnnotation stores the base64-encoded JSON of the user-authored
	// traffic policy rules (auth, rate-limit, etc.) that appeared before the operator's
	// forwarding suffix in the CloudEndpoint's original policy. The plugin prepends these
	// rules on every SetWeight call so user-level policy is preserved during the rollout.
	rolloutPolicyPrefixAnnotation = "k8s.ngrok.com/rollout-policy-prefix"

	// rolloutStableURLAnnotation stores the internal URL of the operator-created stable
	// AgentEndpoint so the plugin can build the stable forward-internal rule in CloudEndpoint mode.
	rolloutStableURLAnnotation = "k8s.ngrok.com/rollout-stable-url"
)

// PluginConfig is parsed from the Rollout's trafficRouting.plugins["ngrok/ngrok"] field.
type PluginConfig struct {
	// TotalPoolSize controls Phase 1 weight granularity. Default: 10.
	// Granularity = 1/TotalPoolSize. The operator-created stable endpoint counts as one slot.
	TotalPoolSize int `json:"totalPoolSize,omitempty"`

	// UseTrafficPolicy switches the plugin to Phase 2 mode: exact rand.double() routing
	// via Traffic Policy instead of AgentEndpoint pool scaling.
	// Requires the operator to be running with rollout-managed annotation support.
	UseTrafficPolicy bool `json:"useTrafficPolicy,omitempty"`

	// CloudEndpoint is the name of the operator-created CloudEndpoint to manage in Phase 2.
	// Only needed when using endpoints-verbose mapping strategy. If omitted, the plugin
	// auto-discovers the CloudEndpoint that forwards to the stable AgentEndpoint.
	CloudEndpoint string `json:"cloudEndpoint,omitempty"`

	// CloudEndpointNamespace is the namespace of the CloudEndpoint. Defaults to the
	// Rollout's namespace when omitted.
	CloudEndpointNamespace string `json:"cloudEndpointNamespace,omitempty"`
}

func parsePluginConfig(rollout *v1alpha1.Rollout) (PluginConfig, error) {
	raw, ok := rollout.Spec.Strategy.Canary.TrafficRouting.Plugins["ngrok/ngrok"]
	if !ok {
		return PluginConfig{}, fmt.Errorf("no ngrok/ngrok plugin config found in rollout %s/%s", rollout.Namespace, rollout.Name)
	}

	var cfg PluginConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return PluginConfig{}, fmt.Errorf("failed to parse ngrok plugin config: %w", err)
	}
	if cfg.TotalPoolSize <= 0 {
		cfg.TotalPoolSize = 10
	}
	return cfg, nil
}
