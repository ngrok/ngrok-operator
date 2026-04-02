package plugin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	pluginTypes "github.com/argoproj/argo-rollouts/utils/plugin/types"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

// NgrokTrafficRouter implements the Argo Rollouts TrafficRouterPlugin interface.
type NgrokTrafficRouter struct {
	k8sClient client.Client
}

func (r *NgrokTrafficRouter) InitPlugin() pluginTypes.RpcError {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return rpcErrorf("failed to get in-cluster config: %v", err)
	}

	scheme := runtime.NewScheme()
	if err := ngrokv1alpha1.AddToScheme(scheme); err != nil {
		return rpcErrorf("failed to add ngrok scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		return rpcErrorf("failed to add core scheme: %v", err)
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return rpcErrorf("failed to create k8s client: %v", err)
	}

	r.k8sClient = c
	return pluginTypes.RpcError{}
}

func (r *NgrokTrafficRouter) UpdateHash(_ *v1alpha1.Rollout, _, _ string, _ []v1alpha1.WeightDestination) pluginTypes.RpcError {
	return pluginTypes.RpcError{}
}

func (r *NgrokTrafficRouter) SetWeight(rollout *v1alpha1.Rollout, desiredWeight int32, _ []v1alpha1.WeightDestination) pluginTypes.RpcError {
	ctx := context.Background()

	cfg, err := parsePluginConfig(rollout)
	if err != nil {
		return rpcErrorf("failed to parse plugin config: %v", err)
	}

	if cfg.UseTrafficPolicy {
		return r.setWeightPhase2(ctx, rollout, cfg, desiredWeight)
	}
	return r.setWeightPhase1(ctx, rollout, cfg, desiredWeight)
}

func (r *NgrokTrafficRouter) VerifyWeight(rollout *v1alpha1.Rollout, desiredWeight int32, _ []v1alpha1.WeightDestination) (pluginTypes.RpcVerified, pluginTypes.RpcError) {
	ctx := context.Background()

	cfg, err := parsePluginConfig(rollout)
	if err != nil {
		return pluginTypes.NotVerified, rpcErrorf("failed to parse plugin config: %v", err)
	}

	if cfg.UseTrafficPolicy {
		return r.verifyWeightPhase2(ctx, rollout, cfg, desiredWeight)
	}
	return r.verifyWeightPhase1(ctx, rollout, cfg, desiredWeight)
}

func (r *NgrokTrafficRouter) SetHeaderRoute(_ *v1alpha1.Rollout, _ *v1alpha1.SetHeaderRoute) pluginTypes.RpcError {
	return pluginTypes.RpcError{}
}

func (r *NgrokTrafficRouter) SetMirrorRoute(_ *v1alpha1.Rollout, _ *v1alpha1.SetMirrorRoute) pluginTypes.RpcError {
	return pluginTypes.RpcError{ErrorString: "SetMirrorRoute is not supported by the ngrok traffic router plugin"}
}

func (r *NgrokTrafficRouter) RemoveManagedRoutes(rollout *v1alpha1.Rollout) pluginTypes.RpcError {
	ctx := context.Background()

	cfg, err := parsePluginConfig(rollout)
	if err != nil {
		return rpcErrorf("failed to parse plugin config: %v", err)
	}

	if cfg.UseTrafficPolicy {
		return r.removeManagedRoutesPhase2(ctx, rollout, cfg)
	}
	return r.removeManagedRoutesPhase1(ctx, rollout)
}

func (r *NgrokTrafficRouter) Type() string { return "ngrok" }

// =============================================================================
// Phase 2 — Traffic Policy routing (exact weights via rand.double())
// =============================================================================
//
// Two sub-modes are selected automatically based on the stable AgentEndpoint's URL:
//
//   Collapsed mode (public URL, e.g. https://my-app.ngrok.app):
//     The operator has folded public URL + routing into one AgentEndpoint.
//     The plugin injects the canary rand.double() rule into that endpoint's
//     inline traffic policy. Stable traffic falls through to spec.upstream.url.
//
//   Verbose mode (internal URL, e.g. https://xxx.internal):
//     The operator created a CloudEndpoint (public URL + traffic policy) that
//     forward-internals to one or more AgentEndpoints. The plugin operates on
//     the CloudEndpoint's traffic policy: it captures the user-authored prefix
//     rules (auth, rate-limit, etc.), then writes [prefix + canary rule + stable
//     forward-internal] on every SetWeight call.

func (r *NgrokTrafficRouter) setWeightPhase2(ctx context.Context, rollout *v1alpha1.Rollout, cfg PluginConfig, desiredWeight int32) pluginTypes.RpcError {
	stableAEP, err := r.findSourceEndpoint(ctx, rollout.Namespace, rollout.Spec.Strategy.Canary.StableService)
	if err != nil {
		return rpcErrorf("failed to find stable AgentEndpoint: %v", err)
	}

	if isInternalURL(stableAEP.Spec.URL) {
		return r.setWeightCloudEndpoint(ctx, rollout, cfg, desiredWeight, stableAEP)
	}
	return r.setWeightCollapsed(ctx, rollout, desiredWeight, stableAEP)
}

// --- Collapsed mode ----------------------------------------------------------

func (r *NgrokTrafficRouter) setWeightCollapsed(ctx context.Context, rollout *v1alpha1.Rollout, desiredWeight int32, stableAEP *ngrokv1alpha1.AgentEndpoint) pluginTypes.RpcError {
	// If desiredWeight is 0 and the plugin does not currently own this AgentEndpoint,
	// the operator's own policy already routes all traffic to stable — nothing to do.
	if desiredWeight == 0 && stableAEP.Annotations[rolloutManagedAnnotation] != "true" {
		return pluginTypes.RpcError{}
	}

	canaryURL := phase2CanaryInternalURL(rollout)
	canaryUpstream := fmt.Sprintf("http://%s.%s:80", rollout.Spec.Strategy.Canary.CanaryService, rollout.Namespace)

	if err := r.ensurePhase2CanaryEndpoint(ctx, rollout, canaryURL, canaryUpstream); err != nil {
		return rpcErrorf("failed to ensure canary AgentEndpoint: %v", err)
	}

	policy := buildCollapsedPolicy(desiredWeight, canaryURL)
	return toRpcError(r.applyCollapsedPolicy(ctx, stableAEP, policy))
}

func buildCollapsedPolicy(desiredWeight int32, canaryURL string) json.RawMessage {
	var rules []tpRule
	switch {
	case desiredWeight > 0 && desiredWeight < 100:
		rules = append(rules, canaryForwardRule(desiredWeight, canaryURL))
	case desiredWeight == 100:
		rules = append(rules, unconditionalForwardRule("ngrok-rollout-canary", canaryURL))
	}
	// desiredWeight == 0: no rules; stable traffic goes to spec.upstream.url implicitly.
	return marshalPolicy(rules)
}

func (r *NgrokTrafficRouter) applyCollapsedPolicy(ctx context.Context, aep *ngrokv1alpha1.AgentEndpoint, policy json.RawMessage) error {
	patch := aep.DeepCopy()
	patch.Spec.TrafficPolicy = &ngrokv1alpha1.TrafficPolicyCfg{Inline: policy}
	if patch.Annotations == nil {
		patch.Annotations = make(map[string]string)
	}
	patch.Annotations[rolloutManagedAnnotation] = "true"
	return r.k8sClient.Update(ctx, patch)
}

// --- CloudEndpoint (verbose) mode --------------------------------------------

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

// =============================================================================
// Phase 2 — VerifyWeight
// =============================================================================

func (r *NgrokTrafficRouter) verifyWeightPhase2(ctx context.Context, rollout *v1alpha1.Rollout, cfg PluginConfig, desiredWeight int32) (pluginTypes.RpcVerified, pluginTypes.RpcError) {
	stableAEP, err := r.findSourceEndpoint(ctx, rollout.Namespace, rollout.Spec.Strategy.Canary.StableService)
	if err != nil {
		return pluginTypes.NotVerified, rpcErrorf("failed to find stable AgentEndpoint: %v", err)
	}

	var policyJSON json.RawMessage
	if isInternalURL(stableAEP.Spec.URL) {
		ce, err := r.findCloudEndpoint(ctx, rollout, cfg, stableAEP)
		if err != nil {
			return pluginTypes.NotVerified, rpcErrorf("failed to find CloudEndpoint: %v", err)
		}
		if ce.Spec.TrafficPolicy != nil {
			policyJSON = ce.Spec.TrafficPolicy.Policy
		}
	} else {
		if stableAEP.Spec.TrafficPolicy != nil {
			policyJSON = stableAEP.Spec.TrafficPolicy.Inline
		}
	}

	threshold, found, err := extractCanaryThreshold(policyJSON)
	if err != nil {
		return pluginTypes.NotVerified, rpcErrorf("failed to parse traffic policy: %v", err)
	}

	if desiredWeight == 0 {
		if !found {
			return pluginTypes.Verified, pluginTypes.RpcError{}
		}
		return pluginTypes.NotVerified, pluginTypes.RpcError{}
	}
	if !found {
		return pluginTypes.NotVerified, pluginTypes.RpcError{}
	}

	actualWeight := int32(math.Round(threshold * 100.0))
	if abs32(actualWeight-desiredWeight) <= 1 {
		return pluginTypes.Verified, pluginTypes.RpcError{}
	}
	return pluginTypes.NotVerified, pluginTypes.RpcError{}
}

// =============================================================================
// Phase 2 — RemoveManagedRoutes
// =============================================================================

func (r *NgrokTrafficRouter) removeManagedRoutesPhase2(ctx context.Context, rollout *v1alpha1.Rollout, cfg PluginConfig) pluginTypes.RpcError {
	// Delete the plugin-created canary AgentEndpoint.
	canaryName := phase2CanaryEndpointName(rollout)
	canary := &ngrokv1alpha1.AgentEndpoint{}
	if err := r.k8sClient.Get(ctx, client.ObjectKey{Name: canaryName, Namespace: rollout.Namespace}, canary); err == nil {
		if err := r.k8sClient.Delete(ctx, canary); err != nil && !errors.IsNotFound(err) {
			return rpcErrorf("failed to delete canary AgentEndpoint: %v", err)
		}
	} else if !errors.IsNotFound(err) {
		return rpcErrorf("failed to get canary AgentEndpoint: %v", err)
	}

	// Remove ownership annotation from whichever resource the plugin was managing.
	stableAEP, err := r.findSourceEndpoint(ctx, rollout.Namespace, rollout.Spec.Strategy.Canary.StableService)
	if err != nil {
		return pluginTypes.RpcError{} // source gone — nothing to clean up
	}

	if isInternalURL(stableAEP.Spec.URL) {
		ce, err := r.findCloudEndpoint(ctx, rollout, cfg, stableAEP)
		if err != nil {
			return pluginTypes.RpcError{} // cloud endpoint gone — nothing to clean up
		}
		if ce.Annotations[rolloutManagedAnnotation] == "true" {
			// Restore the CloudEndpoint policy to all-stable (weight=0) and remove rollout
			// annotations in one update. This ensures traffic is correct immediately and that
			// the operator's next reconcile will find no annotation and restore its own policy.
			prefixRules, _ := loadPrefixFromAnnotation(ce.Annotations[rolloutPolicyPrefixAnnotation])
			stableURL := ce.Annotations[rolloutStableURLAnnotation]
			if stableURL == "" {
				stableURL = stableAEP.Spec.URL
			}
			restoredPolicy, policyErr := buildCloudEndpointPolicy(prefixRules, 0, "", stableURL)
			patch := ce.DeepCopy()
			if policyErr == nil && patch.Spec.TrafficPolicy != nil {
				patch.Spec.TrafficPolicy.Policy = restoredPolicy
			}
			delete(patch.Annotations, rolloutManagedAnnotation)
			delete(patch.Annotations, rolloutPolicyPrefixAnnotation)
			delete(patch.Annotations, rolloutStableURLAnnotation)
			if err := r.k8sClient.Update(ctx, patch); err != nil {
				return rpcErrorf("failed to restore CloudEndpoint policy and remove rollout annotations: %v", err)
			}
		}
	} else {
		if stableAEP.Annotations[rolloutManagedAnnotation] == "true" {
			patch := stableAEP.DeepCopy()
			delete(patch.Annotations, rolloutManagedAnnotation)
			if err := r.k8sClient.Update(ctx, patch); err != nil {
				return rpcErrorf("failed to remove rollout-managed annotation from AgentEndpoint: %v", err)
			}
		}
	}

	return pluginTypes.RpcError{}
}

// =============================================================================
// Phase 1 — AgentEndpoint pool scaling (approximate weights)
// =============================================================================

func (r *NgrokTrafficRouter) setWeightPhase1(ctx context.Context, rollout *v1alpha1.Rollout, cfg PluginConfig, desiredWeight int32) pluginTypes.RpcError {
	stableService := rollout.Spec.Strategy.Canary.StableService
	canaryService := rollout.Spec.Strategy.Canary.CanaryService

	sourceEndpoint, err := r.findSourceEndpoint(ctx, rollout.Namespace, stableService)
	if err != nil {
		return rpcErrorf("failed to find source AgentEndpoint: %v", err)
	}

	sharedURL := sourceEndpoint.Spec.URL
	canaryCount := int(math.Round(float64(cfg.TotalPoolSize) * float64(desiredWeight) / 100.0))
	pluginStableCount := cfg.TotalPoolSize - canaryCount - 1
	if pluginStableCount < 0 {
		pluginStableCount = 0
	}

	stableUpstream := fmt.Sprintf("http://%s.%s:80", stableService, rollout.Namespace)
	canaryUpstream := fmt.Sprintf("http://%s.%s:80", canaryService, rollout.Namespace)

	if err := r.reconcilePool(ctx, rollout, sharedURL, "stable", stableUpstream, pluginStableCount); err != nil {
		return rpcErrorf("failed to reconcile stable pool: %v", err)
	}
	if err := r.reconcilePool(ctx, rollout, sharedURL, "canary", canaryUpstream, canaryCount); err != nil {
		return rpcErrorf("failed to reconcile canary pool: %v", err)
	}

	return pluginTypes.RpcError{}
}

func (r *NgrokTrafficRouter) verifyWeightPhase1(ctx context.Context, rollout *v1alpha1.Rollout, cfg PluginConfig, desiredWeight int32) (pluginTypes.RpcVerified, pluginTypes.RpcError) {
	canaryEndpoints, err := r.listPoolEndpoints(ctx, rollout, "canary")
	if err != nil {
		return pluginTypes.NotVerified, rpcErrorf("failed to list canary endpoints: %v", err)
	}

	totalPool := cfg.TotalPoolSize
	actualCanary := len(canaryEndpoints)
	actualWeight := int32(math.Round(float64(actualCanary) / float64(totalPool) * 100.0))
	tolerance := int32(math.Ceil(100.0 / float64(totalPool)))

	if abs32(actualWeight-desiredWeight) <= tolerance {
		return pluginTypes.Verified, pluginTypes.RpcError{}
	}
	return pluginTypes.NotVerified, pluginTypes.RpcError{}
}

func (r *NgrokTrafficRouter) removeManagedRoutesPhase1(ctx context.Context, rollout *v1alpha1.Rollout) pluginTypes.RpcError {
	var list ngrokv1alpha1.AgentEndpointList
	if err := r.k8sClient.List(ctx, &list,
		client.InNamespace(rollout.Namespace),
		client.MatchingLabels{
			labelRolloutName:      rollout.Name,
			labelRolloutNamespace: rollout.Namespace,
		},
	); err != nil {
		return rpcErrorf("failed to list managed endpoints: %v", err)
	}

	for i := range list.Items {
		if err := r.k8sClient.Delete(ctx, &list.Items[i]); err != nil && !errors.IsNotFound(err) {
			return rpcErrorf("failed to delete endpoint %s: %v", list.Items[i].Name, err)
		}
	}
	return pluginTypes.RpcError{}
}

// =============================================================================
// Shared Phase 2 helpers
// =============================================================================

// isInternalURL reports whether url is an ngrok internal endpoint URL (ends in .internal).
// AgentEndpoints created by the operator in verbose mode have internal URLs; those created
// in collapsed mode have public URLs.
func isInternalURL(url string) bool {
	lower := strings.ToLower(url)
	// strip scheme and check the host portion
	host := lower
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	// strip port and path
	if i := strings.IndexAny(host, ":/"); i >= 0 {
		host = host[:i]
	}
	return strings.HasSuffix(host, ".internal")
}

func phase2CanaryInternalURL(rollout *v1alpha1.Rollout) string {
	host := fmt.Sprintf("%s-ngrok-p2-canary-%s", rollout.Name, rollout.Namespace)
	host = strings.ReplaceAll(host, ".", "-")
	return fmt.Sprintf("https://%s.internal", host)
}

func phase2CanaryEndpointName(rollout *v1alpha1.Rollout) string {
	return fmt.Sprintf("%s-ngrok-p2-canary", rollout.Name)
}

func (r *NgrokTrafficRouter) ensurePhase2CanaryEndpoint(ctx context.Context, rollout *v1alpha1.Rollout, internalURL, upstreamURL string) error {
	name := phase2CanaryEndpointName(rollout)
	ep := &ngrokv1alpha1.AgentEndpoint{}
	if err := r.k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rollout.Namespace}, ep); err == nil {
		return nil
	} else if !errors.IsNotFound(err) {
		return fmt.Errorf("getting Phase 2 canary AgentEndpoint: %w", err)
	}

	ep = &ngrokv1alpha1.AgentEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rollout.Namespace,
			Labels: map[string]string{
				labelRolloutName:      rollout.Name,
				labelRolloutNamespace: rollout.Namespace,
				labelRolloutRole:      "canary",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: rollout.APIVersion,
				Kind:       rollout.Kind,
				Name:       rollout.Name,
				UID:        types.UID(rollout.UID),
			}},
		},
		Spec: ngrokv1alpha1.AgentEndpointSpec{
			URL: internalURL,
			Upstream: ngrokv1alpha1.EndpointUpstream{
				URL: upstreamURL,
			},
			Description: fmt.Sprintf("ngrok rollout plugin Phase 2 — canary endpoint for %s/%s", rollout.Namespace, rollout.Name),
		},
	}
	return r.k8sClient.Create(ctx, ep)
}

// =============================================================================
// Traffic policy building blocks
// =============================================================================

// tpRule is the wire format for a single traffic policy rule.
type tpRule struct {
	Name        string                   `json:"name,omitempty"`
	Expressions []string                 `json:"expressions,omitempty"`
	Actions     []map[string]interface{} `json:"actions"`
}

func canaryForwardRule(desiredWeight int32, canaryURL string) tpRule {
	threshold := float64(desiredWeight) / 100.0
	return tpRule{
		Name:        "ngrok-rollout-canary",
		Expressions: []string{fmt.Sprintf("rand.double() <= %.4f", threshold)},
		Actions: []map[string]interface{}{{
			"type":   "forward-internal",
			"config": map[string]interface{}{"url": canaryURL},
		}},
	}
}

func unconditionalForwardRule(name, url string) tpRule {
	return tpRule{
		Name: name,
		Actions: []map[string]interface{}{{
			"type":   "forward-internal",
			"config": map[string]interface{}{"url": url},
		}},
	}
}

func marshalPolicy(rules []tpRule) json.RawMessage {
	type policy struct {
		OnHTTPRequest []tpRule `json:"on_http_request"`
	}
	b, _ := json.Marshal(policy{OnHTTPRequest: rules})
	return json.RawMessage(b)
}

// extractCanaryThreshold finds the rand.double() threshold from the "ngrok-rollout-canary"
// rule in the given policy JSON. Returns (threshold, true, nil) if found.
func extractCanaryThreshold(raw json.RawMessage) (threshold float64, found bool, err error) {
	if len(raw) == 0 {
		return 0, false, nil
	}
	var policy struct {
		OnHTTPRequest []struct {
			Name        string   `json:"name"`
			Expressions []string `json:"expressions"`
		} `json:"on_http_request"`
	}
	if err = json.Unmarshal(raw, &policy); err != nil {
		return 0, false, err
	}
	for _, rule := range policy.OnHTTPRequest {
		if rule.Name != "ngrok-rollout-canary" {
			continue
		}
		if len(rule.Expressions) == 0 {
			return 1.0, true, nil // desiredWeight == 100, unconditional
		}
		var t float64
		if _, scanErr := fmt.Sscanf(rule.Expressions[0], "rand.double() <= %f", &t); scanErr == nil {
			return t, true, nil
		}
	}
	return 0, false, nil
}

// =============================================================================
// Phase 1 pool helpers
// =============================================================================

func (r *NgrokTrafficRouter) findSourceEndpoint(ctx context.Context, namespace, stableService string) (*ngrokv1alpha1.AgentEndpoint, error) {
	var list ngrokv1alpha1.AgentEndpointList
	if err := r.k8sClient.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("listing AgentEndpoints: %w", err)
	}

	serviceFragment := stableService + "." + namespace
	for i := range list.Items {
		ep := &list.Items[i]
		if _, ok := ep.Labels[labelRolloutName]; ok {
			continue // skip plugin-created endpoints
		}
		if strings.Contains(ep.Spec.Upstream.URL, serviceFragment) {
			return ep, nil
		}
	}
	return nil, fmt.Errorf("no operator-created AgentEndpoint found with upstream matching %q in namespace %q", stableService, namespace)
}

func (r *NgrokTrafficRouter) reconcilePool(ctx context.Context, rollout *v1alpha1.Rollout, sharedURL, role, upstreamURL string, desired int) error {
	existing, err := r.listPoolEndpoints(ctx, rollout, role)
	if err != nil {
		return err
	}

	for len(existing) > desired {
		last := existing[len(existing)-1]
		if err := r.k8sClient.Delete(ctx, last); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("deleting endpoint %s: %w", last.Name, err)
		}
		existing = existing[:len(existing)-1]
	}

	for i := len(existing); i < desired; i++ {
		ep := r.buildPoolEndpoint(rollout, sharedURL, role, upstreamURL, i)
		if err := r.k8sClient.Create(ctx, ep); err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("creating endpoint %s: %w", ep.Name, err)
		}
	}
	return nil
}

func (r *NgrokTrafficRouter) listPoolEndpoints(ctx context.Context, rollout *v1alpha1.Rollout, role string) ([]*ngrokv1alpha1.AgentEndpoint, error) {
	var list ngrokv1alpha1.AgentEndpointList
	if err := r.k8sClient.List(ctx, &list,
		client.InNamespace(rollout.Namespace),
		client.MatchingLabels{
			labelRolloutName:      rollout.Name,
			labelRolloutNamespace: rollout.Namespace,
			labelRolloutRole:      role,
		},
	); err != nil {
		return nil, fmt.Errorf("listing %s pool endpoints: %w", role, err)
	}

	result := make([]*ngrokv1alpha1.AgentEndpoint, len(list.Items))
	for i := range list.Items {
		result[i] = &list.Items[i]
	}
	return result, nil
}

func (r *NgrokTrafficRouter) buildPoolEndpoint(rollout *v1alpha1.Rollout, sharedURL, role, upstreamURL string, index int) *ngrokv1alpha1.AgentEndpoint {
	name := fmt.Sprintf("%s-ngrok-%s-%d", rollout.Name, role, index)
	return &ngrokv1alpha1.AgentEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rollout.Namespace,
			Labels: map[string]string{
				labelRolloutName:      rollout.Name,
				labelRolloutNamespace: rollout.Namespace,
				labelRolloutRole:      role,
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: rollout.APIVersion,
				Kind:       rollout.Kind,
				Name:       rollout.Name,
				UID:        types.UID(rollout.UID),
			}},
		},
		Spec: ngrokv1alpha1.AgentEndpointSpec{
			URL:         sharedURL,
			Upstream:    ngrokv1alpha1.EndpointUpstream{URL: upstreamURL},
			Description: fmt.Sprintf("ngrok rollout plugin — %s pool for %s/%s", role, rollout.Namespace, rollout.Name),
		},
	}
}

// =============================================================================
// Misc helpers
// =============================================================================

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

func toRpcError(err error) pluginTypes.RpcError {
	if err == nil {
		return pluginTypes.RpcError{}
	}
	return pluginTypes.RpcError{ErrorString: err.Error()}
}

func rpcErrorf(format string, args ...any) pluginTypes.RpcError {
	return pluginTypes.RpcError{ErrorString: fmt.Sprintf(format, args...)}
}

func abs32(x int32) int32 {
	if x < 0 {
		return -x
	}
	return x
}
