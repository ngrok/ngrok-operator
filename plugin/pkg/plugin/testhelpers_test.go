package plugin

import (
	"encoding/json"
	"time"

	argorollouts "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	if err := ngrokv1alpha1.AddToScheme(s); err != nil {
		panic(err)
	}
	return s
}

func makeFakeClient(objs ...client.Object) client.Client {
	return fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(objs...).Build()
}

// makeRouter builds a NgrokTrafficRouter with a fake k8s client and a no-op sleep function.
func makeRouter(objs ...client.Object) *NgrokTrafficRouter {
	return &NgrokTrafficRouter{
		k8sClient: makeFakeClient(objs...),
		sleepFn:   func(time.Duration) {},
	}
}

func makeRollout(ns, name, stableSvc, canarySvc string, cfg PluginConfig) *argorollouts.Rollout {
	cfgJSON, _ := json.Marshal(cfg)
	return &argorollouts.Rollout{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: argorollouts.RolloutSpec{
			Strategy: argorollouts.RolloutStrategy{
				Canary: &argorollouts.CanaryStrategy{
					StableService: stableSvc,
					CanaryService: canarySvc,
					TrafficRouting: &argorollouts.RolloutTrafficRouting{
						Plugins: map[string]json.RawMessage{
							"ngrok/ngrok": cfgJSON,
						},
					},
				},
			},
		},
	}
}

// makeOperatorAEP creates a minimal operator-managed AgentEndpoint (no rollout labels).
func makeOperatorAEP(ns, name, url, upstreamURL string) *ngrokv1alpha1.AgentEndpoint {
	return &ngrokv1alpha1.AgentEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: ngrokv1alpha1.AgentEndpointSpec{
			URL:      url,
			Upstream: ngrokv1alpha1.EndpointUpstream{URL: upstreamURL},
		},
	}
}

// makeCloudEndpoint creates a CloudEndpoint with a basic forward-internal policy.
func makeCloudEndpoint(ns, name, url string, policy json.RawMessage) *ngrokv1alpha1.CloudEndpoint {
	ce := &ngrokv1alpha1.CloudEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: ngrokv1alpha1.CloudEndpointSpec{
			URL: url,
		},
	}
	if policy != nil {
		ce.Spec.TrafficPolicy = &ngrokv1alpha1.NgrokTrafficPolicySpec{Policy: policy}
	}
	return ce
}

// forwardInternalPolicy builds a simple CloudEndpoint traffic policy that forwards to url.
func forwardInternalPolicy(targetURL string) json.RawMessage {
	type action struct {
		Type   string         `json:"type"`
		Config map[string]any `json:"config"`
	}
	type rule struct {
		Name    string   `json:"name"`
		Actions []action `json:"actions"`
	}
	type policy struct {
		OnHTTPRequest []rule `json:"on_http_request"`
	}
	p := policy{
		OnHTTPRequest: []rule{{
			Name: "Generated-Route",
			Actions: []action{{
				Type:   "forward-internal",
				Config: map[string]any{"url": targetURL},
			}},
		}},
	}
	b, _ := json.Marshal(p)
	return b
}
