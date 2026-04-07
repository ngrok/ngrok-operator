package plugin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	pluginTypes "github.com/argoproj/argo-rollouts/utils/plugin/types"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// =============================================================================
// Phase 2b — CloudEndpoint (verbose) mode
//
// When the Ingress has the endpoints-verbose mapping strategy, the operator
// creates a CloudEndpoint (public URL + traffic policy) that forward-internals
// to AgentEndpoints with .internal URLs. The plugin operates on the
// CloudEndpoint's traffic policy: it captures user-authored prefix rules
// (auth, rate-limit, etc.), then writes [prefix + canary rule + stable rule]
// on every SetWeight call.
// =============================================================================

func (r *NgrokTrafficRouter) setWeightCloudEndpoint(ctx context.Context, rollout *v1alpha1.Rollout, cfg PluginConfig, desiredWeight int32, stableAEP *ngrokv1alpha1.AgentEndpoint) pluginTypes.RpcError {
	ce, err := r.findCloudEndpoint(ctx, rollout, cfg, stableAEP)
	if err != nil {
		return rpcErrorf("failed to find CloudEndpoint: %v", err)
	}

	// If desiredWeight is 0 and the plugin does not currently own this CloudEndpoint,
	// the operator's own policy already routes all traffic to stable — nothing to do.
	if desiredWeight == 0 && ce.Annotations[rolloutManagedAnnotation] != "true" {
		return pluginTypes.RpcError{}
	}

	// On first ownership, capture the user prefix rules and stable URL from the
	// current policy before we overwrite it.
	stableURL := stableAEP.Spec.URL
	var prefixRules []json.RawMessage

	if ce.Annotations[rolloutManagedAnnotation] != "true" {
		// First call: capture the current policy prefix (user rules before forwarding).
		prefixRules, err = extractPolicyPrefix(ce.Spec.TrafficPolicy.Policy)
		if err != nil {
			return rpcErrorf("failed to extract policy prefix: %v", err)
		}
	} else {
		// Subsequent calls: restore from annotations.
		prefixRules, err = loadPrefixFromAnnotation(ce.Annotations[rolloutPolicyPrefixAnnotation])
		if err != nil {
			return rpcErrorf("failed to load stored policy prefix: %v", err)
		}
		if stored := ce.Annotations[rolloutStableURLAnnotation]; stored != "" {
			stableURL = stored
		}
	}

	canaryURL := phase2CanaryInternalURL(rollout)
	canaryUpstream := fmt.Sprintf("http://%s.%s:80", rollout.Spec.Strategy.Canary.CanaryService, rollout.Namespace)

	if err := r.ensurePhase2CanaryEndpoint(ctx, rollout, canaryURL, canaryUpstream); err != nil {
		return rpcErrorf("failed to ensure canary AgentEndpoint: %v", err)
	}

	policy, err := buildCloudEndpointPolicy(prefixRules, desiredWeight, canaryURL, stableURL)
	if err != nil {
		return rpcErrorf("failed to build CloudEndpoint policy: %v", err)
	}

	return toRpcError(r.applyCloudEndpointPolicy(ctx, ce, policy, prefixRules, stableURL))
}

func (r *NgrokTrafficRouter) applyCloudEndpointPolicy(ctx context.Context, ce *ngrokv1alpha1.CloudEndpoint, policy json.RawMessage, prefixRules []json.RawMessage, stableURL string) error {
	patch := ce.DeepCopy()
	if patch.Spec.TrafficPolicy == nil {
		patch.Spec.TrafficPolicy = &ngrokv1alpha1.NgrokTrafficPolicySpec{}
	}
	patch.Spec.TrafficPolicy.Policy = policy

	if patch.Annotations == nil {
		patch.Annotations = make(map[string]string)
	}
	patch.Annotations[rolloutManagedAnnotation] = "true"
	patch.Annotations[rolloutStableURLAnnotation] = stableURL

	encoded, err := savePrefixToAnnotation(prefixRules)
	if err != nil {
		return fmt.Errorf("encoding policy prefix: %w", err)
	}
	patch.Annotations[rolloutPolicyPrefixAnnotation] = encoded

	return r.k8sClient.Update(ctx, patch)
}

// buildCloudEndpointPolicy assembles the full CloudEndpoint policy:
//
//	[user prefix rules] + [canary forward-internal if weight > 0] + [stable forward-internal]
//
// The stable rule is always present (as a catch-all fallback) except when weight == 100,
// where all traffic routes to canary unconditionally.
func buildCloudEndpointPolicy(prefixRules []json.RawMessage, desiredWeight int32, canaryURL, stableURL string) (json.RawMessage, error) {
	var forwardingRules []tpRule

	switch {
	case desiredWeight > 0 && desiredWeight < 100:
		forwardingRules = append(forwardingRules, canaryForwardRule(desiredWeight, canaryURL))
		forwardingRules = append(forwardingRules, unconditionalForwardRule("ngrok-rollout-stable", stableURL))
	case desiredWeight == 100:
		forwardingRules = append(forwardingRules, unconditionalForwardRule("ngrok-rollout-canary", canaryURL))
	default: // 0
		forwardingRules = append(forwardingRules, unconditionalForwardRule("ngrok-rollout-stable", stableURL))
	}

	// Merge prefix + forwarding into a single on_http_request list.
	type fullPolicy struct {
		OnHTTPRequest []json.RawMessage `json:"on_http_request"`
	}

	forwardingJSON, err := json.Marshal(forwardingRules)
	if err != nil {
		return nil, err
	}
	var forwardingRaw []json.RawMessage
	if err := json.Unmarshal(forwardingJSON, &forwardingRaw); err != nil {
		return nil, err
	}

	combined := append(prefixRules, forwardingRaw...) //nolint:gocritic
	p := fullPolicy{OnHTTPRequest: combined}
	return json.Marshal(p)
}

// extractPolicyPrefix strips the operator-generated routing suffix from a CloudEndpoint's
// traffic policy JSON, returning only the user-authored prefix rules (auth, rate-limit, etc.).
//
// The operator always generates a trailing suffix consisting of:
//  1. One or more path-routing rules whose sole action is "forward-internal"
//  2. A catch-all "Fallback-404" rule whose sole action is "custom-response" with status 404
//
// We walk backwards and strip both types so the plugin can insert its own routing rules
// in their place.
func extractPolicyPrefix(policy json.RawMessage) ([]json.RawMessage, error) {
	if len(policy) == 0 {
		return nil, nil
	}

	var parsed struct {
		OnHTTPRequest []json.RawMessage `json:"on_http_request"`
	}
	if err := json.Unmarshal(policy, &parsed); err != nil {
		return nil, fmt.Errorf("parsing traffic policy: %w", err)
	}

	rules := parsed.OnHTTPRequest
	// Walk backwards, trimming operator-generated routing/fallback rules.
	for len(rules) > 0 {
		var rule struct {
			Actions []struct {
				Type   string `json:"type"`
				Config struct {
					StatusCode int `json:"status_code"`
				} `json:"config"`
			} `json:"actions"`
		}
		if err := json.Unmarshal(rules[len(rules)-1], &rule); err != nil {
			break
		}
		if len(rule.Actions) != 1 {
			break
		}
		actionType := rule.Actions[0].Type
		if actionType == "forward-internal" {
			rules = rules[:len(rules)-1]
			continue
		}
		// Also strip operator's catch-all 404 fallback rule.
		if actionType == "custom-response" && rule.Actions[0].Config.StatusCode == 404 {
			rules = rules[:len(rules)-1]
			continue
		}
		break
	}
	return rules, nil
}

// cloudEndpointForwardsTo reports whether any rule in the policy has a forward-internal
// action pointing at targetURL.
func cloudEndpointForwardsTo(policy json.RawMessage, targetURL string) bool {
	if len(policy) == 0 {
		return false
	}
	var parsed struct {
		OnHTTPRequest []struct {
			Actions []struct {
				Type   string `json:"type"`
				Config struct {
					URL string `json:"url"`
				} `json:"config"`
			} `json:"actions"`
		} `json:"on_http_request"`
	}
	if err := json.Unmarshal(policy, &parsed); err != nil {
		return false
	}
	for _, rule := range parsed.OnHTTPRequest {
		for _, action := range rule.Actions {
			if action.Type == "forward-internal" && action.Config.URL == targetURL {
				return true
			}
		}
	}
	return false
}

// findCloudEndpoint locates the CloudEndpoint to operate on in Phase 2 verbose mode.
// Resolution order:
//  1. Explicit config: cloudEndpoint + cloudEndpointNamespace in the Rollout plugin config.
//  2. Auto-discover: list CloudEndpoints in the namespace, find the one whose traffic
//     policy contains a forward-internal to the stable AgentEndpoint's internal URL.
func (r *NgrokTrafficRouter) findCloudEndpoint(ctx context.Context, rollout *v1alpha1.Rollout, cfg PluginConfig, stableAEP *ngrokv1alpha1.AgentEndpoint) (*ngrokv1alpha1.CloudEndpoint, error) {
	if cfg.CloudEndpoint != "" {
		ns := cfg.CloudEndpointNamespace
		if ns == "" {
			ns = rollout.Namespace
		}
		var ce ngrokv1alpha1.CloudEndpoint
		if err := r.k8sClient.Get(ctx, client.ObjectKey{Name: cfg.CloudEndpoint, Namespace: ns}, &ce); err != nil {
			return nil, fmt.Errorf("getting CloudEndpoint %s/%s: %w", ns, cfg.CloudEndpoint, err)
		}
		return &ce, nil
	}

	// Auto-discover: list all CloudEndpoints in the namespace and look for the one that is
	// either (a) currently owned by the plugin (rollout-managed annotation), or
	// (b) forwards to the stable AEP's internal URL in its traffic policy.
	//
	// Checking for the annotation first covers the weight=100 case where the stable URL
	// no longer appears in the plugin-written policy.
	var list ngrokv1alpha1.CloudEndpointList
	if err := r.k8sClient.List(ctx, &list, client.InNamespace(rollout.Namespace)); err != nil {
		return nil, fmt.Errorf("listing CloudEndpoints: %w", err)
	}

	var policyMatch *ngrokv1alpha1.CloudEndpoint
	for i := range list.Items {
		ce := &list.Items[i]
		if ce.Annotations[rolloutManagedAnnotation] == "true" {
			return ce, nil
		}
		if ce.Spec.TrafficPolicy != nil && cloudEndpointForwardsTo(ce.Spec.TrafficPolicy.Policy, stableAEP.Spec.URL) {
			policyMatch = ce
		}
	}
	if policyMatch != nil {
		return policyMatch, nil
	}

	return nil, fmt.Errorf("no CloudEndpoint found that forwards to stable AgentEndpoint URL %q; set cloudEndpoint in the plugin config to specify it explicitly", stableAEP.Spec.URL)
}

func savePrefixToAnnotation(rules []json.RawMessage) (string, error) {
	b, err := json.Marshal(rules)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func loadPrefixFromAnnotation(encoded string) ([]json.RawMessage, error) {
	if encoded == "" {
		return nil, nil
	}
	b, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decoding policy prefix annotation: %w", err)
	}
	var rules []json.RawMessage
	if err := json.Unmarshal(b, &rules); err != nil {
		return nil, fmt.Errorf("parsing policy prefix annotation: %w", err)
	}
	return rules, nil
}
