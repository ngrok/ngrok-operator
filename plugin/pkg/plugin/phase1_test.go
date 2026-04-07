package plugin

import (
	"context"
	"fmt"
	"testing"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestSetWeightPhase1(t *testing.T) {
	const ns = "default"

	tests := []struct {
		weight       int32
		poolSize     int
		wantCanary   int
		wantStable   int // plugin-created stable endpoints (not counting operator AEP)
	}{
		{0, 10, 0, 9},
		{25, 10, 3, 6},  // round(10 * 0.25) = 3 canary, 10-3-1 = 6 stable
		{50, 10, 5, 4},
		{100, 10, 10, 0},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("weight=%d", tt.weight), func(t *testing.T) {
			sourceAEP := makeOperatorAEP(ns, "source-aep", "https://my-app.ngrok.app", "http://stable-svc.default:80")
			rollout := makeRollout(ns, "my-rollout", "stable-svc", "canary-svc", PluginConfig{TotalPoolSize: tt.poolSize})

			router := makeRouter(sourceAEP)
			rpcErr := router.SetWeight(rollout, tt.weight, nil)
			if rpcErr.ErrorString != "" {
				t.Fatalf("SetWeight error: %s", rpcErr.ErrorString)
			}

			canaryEPs, err := router.listPoolEndpoints(context.Background(), rollout, "canary")
			if err != nil {
				t.Fatal(err)
			}
			stableEPs, err := router.listPoolEndpoints(context.Background(), rollout, "stable")
			if err != nil {
				t.Fatal(err)
			}

			if len(canaryEPs) != tt.wantCanary {
				t.Errorf("canary pool = %d, want %d", len(canaryEPs), tt.wantCanary)
			}
			if len(stableEPs) != tt.wantStable {
				t.Errorf("stable pool = %d, want %d", len(stableEPs), tt.wantStable)
			}
		})
	}
}

func TestSetWeightPhase1_ScalesDown(t *testing.T) {
	const ns = "default"

	sourceAEP := makeOperatorAEP(ns, "source-aep", "https://my-app.ngrok.app", "http://stable-svc.default:80")
	rollout := makeRollout(ns, "my-rollout", "stable-svc", "canary-svc", PluginConfig{TotalPoolSize: 10})

	// Pre-create 5 canary endpoints as if a previous SetWeight(50) ran
	objs := []client.Object{sourceAEP}
	for i := 0; i < 5; i++ {
		ep := &ngrokv1alpha1.AgentEndpoint{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("my-rollout-ngrok-canary-%d", i),
				Namespace: ns,
				Labels: map[string]string{
					labelRolloutName:      "my-rollout",
					labelRolloutNamespace: ns,
					labelRolloutRole:      "canary",
				},
			},
			Spec: ngrokv1alpha1.AgentEndpointSpec{
				URL:      "https://my-app.ngrok.app",
				Upstream: ngrokv1alpha1.EndpointUpstream{URL: "http://canary-svc.default:80"},
			},
		}
		objs = append(objs, ep)
	}

	router := makeRouter(objs...)

	// Scale down to weight=25 (3 canary)
	rpcErr := router.SetWeight(rollout, 25, nil)
	if rpcErr.ErrorString != "" {
		t.Fatalf("SetWeight error: %s", rpcErr.ErrorString)
	}

	canaryEPs, _ := router.listPoolEndpoints(context.Background(), rollout, "canary")
	if len(canaryEPs) != 3 {
		t.Errorf("canary pool = %d, want 3 after scale down", len(canaryEPs))
	}
}

func TestRemoveManagedRoutesPhase1(t *testing.T) {
	const ns = "default"

	rollout := makeRollout(ns, "my-rollout", "stable-svc", "canary-svc", PluginConfig{TotalPoolSize: 10})

	// Create labelled pool endpoints
	objs := []client.Object{}
	for i := 0; i < 3; i++ {
		for _, role := range []string{"canary", "stable"} {
			ep := &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("my-rollout-ngrok-%s-%d", role, i),
					Namespace: ns,
					Labels: map[string]string{
						labelRolloutName:      "my-rollout",
						labelRolloutNamespace: ns,
						labelRolloutRole:      role,
					},
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL:      "https://my-app.ngrok.app",
					Upstream: ngrokv1alpha1.EndpointUpstream{URL: "http://svc.default:80"},
				},
			}
			objs = append(objs, ep)
		}
	}

	router := makeRouter(objs...)
	rpcErr := router.RemoveManagedRoutes(rollout)
	if rpcErr.ErrorString != "" {
		t.Fatalf("RemoveManagedRoutes error: %s", rpcErr.ErrorString)
	}

	// All labelled endpoints should be deleted
	var remaining ngrokv1alpha1.AgentEndpointList
	if err := router.k8sClient.List(context.Background(), &remaining, client.InNamespace(ns)); err != nil {
		t.Fatal(err)
	}
	for _, ep := range remaining.Items {
		if _, ok := ep.Labels[labelRolloutName]; ok {
			t.Errorf("endpoint %q still exists after RemoveManagedRoutes", ep.Name)
		}
	}
}

func TestVerifyWeightPhase1(t *testing.T) {
	const ns = "default"

	rollout := makeRollout(ns, "my-rollout", "stable-svc", "canary-svc", PluginConfig{TotalPoolSize: 10})

	// Create 3 canary endpoints (30%)
	objs := []client.Object{}
	for i := 0; i < 3; i++ {
		ep := &ngrokv1alpha1.AgentEndpoint{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("my-rollout-ngrok-canary-%d", i),
				Namespace: ns,
				Labels: map[string]string{
					labelRolloutName:      "my-rollout",
					labelRolloutNamespace: ns,
					labelRolloutRole:      "canary",
				},
			},
			Spec: ngrokv1alpha1.AgentEndpointSpec{
				URL:      "https://my-app.ngrok.app",
				Upstream: ngrokv1alpha1.EndpointUpstream{URL: "http://svc.default:80"},
			},
		}
		objs = append(objs, ep)
	}

	router := makeRouter(objs...)

	// Verifying 30% should pass (within tolerance)
	if verified, rpcErr := router.VerifyWeight(rollout, 30, nil); rpcErr.ErrorString != "" || verified == 0 {
		t.Errorf("VerifyWeight(30) = %v, %v; expected Verified", verified, rpcErr)
	}

	// Verifying 50% with only 3/10 canary endpoints should fail
	if verified, rpcErr := router.VerifyWeight(rollout, 50, nil); rpcErr.ErrorString != "" || verified != 0 {
		t.Errorf("VerifyWeight(50) = %v, %v; expected NotVerified", verified, rpcErr)
	}
}
