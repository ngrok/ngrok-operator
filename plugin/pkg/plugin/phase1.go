package plugin

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	pluginTypes "github.com/argoproj/argo-rollouts/utils/plugin/types"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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

// findSourceEndpoint returns the operator-created AgentEndpoint whose upstream URL
// references the given stable service. Plugin-created endpoints (those with rollout
// labels) are skipped.
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
