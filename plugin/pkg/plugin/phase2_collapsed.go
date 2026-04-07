package plugin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	pluginTypes "github.com/argoproj/argo-rollouts/utils/plugin/types"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
)

// =============================================================================
// Phase 2a — Collapsed mode
//
// The operator has created a single AgentEndpoint with a public URL
// (e.g. https://my-app.ngrok.app). The plugin injects a rand.double() canary
// rule into that endpoint's inline traffic policy. Stable traffic falls through
// to spec.upstream.url implicitly.
// =============================================================================

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

// buildCollapsedPolicy returns the inline traffic policy JSON for the stable AgentEndpoint
// in collapsed mode. At weight 0 there are no rules (stable traffic falls through to
// spec.upstream.url). At weight 100 an unconditional forward-internal rule sends all
// traffic to the canary endpoint.
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
