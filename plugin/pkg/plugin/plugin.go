package plugin

import (
	"context"
	"time"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	pluginTypes "github.com/argoproj/argo-rollouts/utils/plugin/types"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NgrokTrafficRouter implements the Argo Rollouts TrafficRouterPlugin interface.
type NgrokTrafficRouter struct {
	k8sClient client.Client
	// sleepFn is used instead of time.Sleep; override in tests to make sleeps instant.
	sleepFn func(time.Duration)
}

// doSleep calls sleepFn if set, otherwise time.Sleep.
func (r *NgrokTrafficRouter) doSleep(d time.Duration) {
	if r.sleepFn != nil {
		r.sleepFn(d)
	} else {
		time.Sleep(d)
	}
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
