package plugin

import (
	"encoding/json"
	"testing"
)

func TestBuildCollapsedPolicy(t *testing.T) {
	canary := "https://my-rollout-ngrok-p2-canary-default.internal"

	getRules := func(t *testing.T, p json.RawMessage) []struct {
		Name        string   `json:"name"`
		Expressions []string `json:"expressions"`
	} {
		t.Helper()
		var parsed struct {
			OnHTTPRequest []struct {
				Name        string   `json:"name"`
				Expressions []string `json:"expressions"`
			} `json:"on_http_request"`
		}
		if err := json.Unmarshal(p, &parsed); err != nil {
			t.Fatalf("parse policy: %v", err)
		}
		return parsed.OnHTTPRequest
	}

	t.Run("weight 0: no rules (stable falls through)", func(t *testing.T) {
		p := buildCollapsedPolicy(0, canary)
		rules := getRules(t, p)
		if len(rules) != 0 {
			t.Errorf("expected 0 rules at weight=0, got %v", rules)
		}
	})

	t.Run("weight 50: conditional canary rule", func(t *testing.T) {
		p := buildCollapsedPolicy(50, canary)
		rules := getRules(t, p)
		if len(rules) != 1 || rules[0].Name != "ngrok-rollout-canary" {
			t.Errorf("rules = %+v", rules)
		}
		if len(rules[0].Expressions) != 1 {
			t.Error("expected 1 expression on canary rule")
		}
	})

	t.Run("weight 100: unconditional canary rule", func(t *testing.T) {
		p := buildCollapsedPolicy(100, canary)
		rules := getRules(t, p)
		if len(rules) != 1 || rules[0].Name != "ngrok-rollout-canary" {
			t.Errorf("rules = %+v", rules)
		}
		if len(rules[0].Expressions) != 0 {
			t.Error("weight=100 should produce unconditional rule (no expressions)")
		}
	})
}

func TestSetWeightCollapsed_FirstCall(t *testing.T) {
	const ns = "default"

	// Stable AEP has a public URL (collapsed mode)
	stableAEP := makeOperatorAEP(ns, "stable-aep", "https://my-app.ngrok.app", "http://stable-svc.default:80")
	rollout := makeRollout(ns, "my-rollout", "stable-svc", "canary-svc", PluginConfig{UseTrafficPolicy: true})

	router := makeRouter(stableAEP)
	rpcErr := router.SetWeight(rollout, 25, nil)
	if rpcErr.ErrorString != "" {
		t.Fatalf("SetWeight error: %s", rpcErr.ErrorString)
	}

	// Stable AEP should have rollout-managed annotation and inline traffic policy
	gotAEP, err := getAEP(router, ns, "stable-aep")
	if err != nil {
		t.Fatalf("getting AEP: %v", err)
	}
	if gotAEP.Annotations[rolloutManagedAnnotation] != "true" {
		t.Error("rollout-managed annotation not set")
	}
	if gotAEP.Spec.TrafficPolicy == nil || gotAEP.Spec.TrafficPolicy.Inline == nil {
		t.Fatal("inline traffic policy not set on stable AEP")
	}

	// Canary threshold should be 0.25
	threshold, found, err := extractCanaryThreshold(gotAEP.Spec.TrafficPolicy.Inline)
	if err != nil {
		t.Fatal(err)
	}
	if !found || threshold < 0.24 || threshold > 0.26 {
		t.Errorf("threshold = %.4f, found = %v; want ~0.25", threshold, found)
	}

	// Plugin canary AEP should be created
	canaryName := phase2CanaryEndpointName(rollout)
	canary, err := getAEP(router, ns, canaryName)
	if err != nil {
		t.Fatalf("getting canary AEP: %v", err)
	}
	if canary == nil {
		t.Fatal("canary AEP was not created")
	}
}

func TestSetWeightCollapsed_ZeroWeight_NoOwnership(t *testing.T) {
	const ns = "default"

	stableAEP := makeOperatorAEP(ns, "stable-aep", "https://my-app.ngrok.app", "http://stable-svc.default:80")
	rollout := makeRollout(ns, "my-rollout", "stable-svc", "canary-svc", PluginConfig{UseTrafficPolicy: true})

	router := makeRouter(stableAEP)
	rpcErr := router.SetWeight(rollout, 0, nil)
	if rpcErr.ErrorString != "" {
		t.Fatalf("SetWeight error: %s", rpcErr.ErrorString)
	}

	gotAEP, _ := getAEP(router, ns, "stable-aep")
	if gotAEP.Annotations[rolloutManagedAnnotation] == "true" {
		t.Error("plugin should not own stable AEP at weight=0 when not previously owned")
	}
}

func TestRemoveManagedRoutes_Collapsed(t *testing.T) {
	const ns = "default"

	// Stable AEP is owned by the plugin (mid-rollout)
	stableAEP := makeOperatorAEP(ns, "stable-aep", "https://my-app.ngrok.app", "http://stable-svc.default:80")
	stableAEP.Annotations = map[string]string{rolloutManagedAnnotation: "true"}

	rollout := makeRollout(ns, "my-rollout", "stable-svc", "canary-svc", PluginConfig{UseTrafficPolicy: true})
	canaryName := phase2CanaryEndpointName(rollout)
	canaryAEP := makeOperatorAEP(ns, canaryName, phase2CanaryInternalURL(rollout), "http://canary-svc.default:80")

	router := makeRouter(stableAEP, canaryAEP)
	rpcErr := router.RemoveManagedRoutes(rollout)
	if rpcErr.ErrorString != "" {
		t.Fatalf("RemoveManagedRoutes error: %s", rpcErr.ErrorString)
	}

	gotAEP, _ := getAEP(router, ns, "stable-aep")
	if gotAEP.Annotations[rolloutManagedAnnotation] == "true" {
		t.Error("rollout-managed annotation should be removed from stable AEP")
	}

	if canary, _ := getAEP(router, ns, canaryName); canary != nil {
		t.Error("canary AEP should be deleted")
	}
}
