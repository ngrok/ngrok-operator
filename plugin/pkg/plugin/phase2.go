package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	pluginTypes "github.com/argoproj/argo-rollouts/utils/plugin/types"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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

func (r *NgrokTrafficRouter) removeManagedRoutesPhase2(ctx context.Context, rollout *v1alpha1.Rollout, cfg PluginConfig) pluginTypes.RpcError {
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
			// Restore the CloudEndpoint policy to all-stable FIRST, before deleting the canary
			// AgentEndpoint. The canary tunnel drops instantly on delete, but the CloudEndpoint
			// policy update is an eventually-consistent ngrok API call. Updating the policy first
			// gives the controller time to push the change to the ngrok edge before the canary
			// tunnel disappears, eliminating the brief window where the policy still routes some
			// traffic to a dead tunnel.
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

			// Brief pause to let the CloudEndpoint controller reconcile and push the updated
			// policy to the ngrok API before the canary tunnel disappears.
			r.doSleep(2 * time.Second)
		}
	} else {
		if stableAEP.Annotations[rolloutManagedAnnotation] == "true" {
			patch := stableAEP.DeepCopy()
			delete(patch.Annotations, rolloutManagedAnnotation)
			if err := r.k8sClient.Update(ctx, patch); err != nil {
				return rpcErrorf("failed to remove rollout-managed annotation from AgentEndpoint: %v", err)
			}
			r.doSleep(2 * time.Second)
		}
	}

	// Delete the plugin-created canary AgentEndpoint now that the routing policy
	// has been updated and had time to propagate.
	canaryName := phase2CanaryEndpointName(rollout)
	canary := &ngrokv1alpha1.AgentEndpoint{}
	if err := r.k8sClient.Get(ctx, client.ObjectKey{Name: canaryName, Namespace: rollout.Namespace}, canary); err == nil {
		if err := r.k8sClient.Delete(ctx, canary); err != nil && !errors.IsNotFound(err) {
			return rpcErrorf("failed to delete canary AgentEndpoint: %v", err)
		}
	} else if !errors.IsNotFound(err) {
		return rpcErrorf("failed to get canary AgentEndpoint: %v", err)
	}

	return pluginTypes.RpcError{}
}

// ensurePhase2CanaryEndpoint creates the plugin-owned canary AgentEndpoint with an
// internal URL if it does not already exist.
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

// isInternalURL reports whether url is an ngrok internal endpoint URL (ends in .internal).
// AgentEndpoints created by the operator in verbose mode have internal URLs; those created
// in collapsed mode have public URLs.
func isInternalURL(url string) bool {
	lower := strings.ToLower(url)
	host := lower
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
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
