package plugin

import (
	"context"
	"encoding/json"
	"testing"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ---------------------------------------------------------------------------
// Pure function tests
// ---------------------------------------------------------------------------

func TestExtractPolicyPrefix(t *testing.T) {
	// makeRuleJSON returns a single rule as JSON (not a full policy document).
	makeRuleJSON := func(name, actionType string) json.RawMessage {
		cfg := map[string]any{}
		if actionType == "custom-response" {
			cfg["status_code"] = 404
		} else {
			cfg["url"] = "https://example.internal"
		}
		type action struct {
			Type   string         `json:"type"`
			Config map[string]any `json:"config,omitempty"`
		}
		type rule struct {
			Name    string   `json:"name"`
			Actions []action `json:"actions"`
		}
		b, _ := json.Marshal(rule{Name: name, Actions: []action{{Type: actionType, Config: cfg}}})
		return b
	}

	// combineRules wraps individual rule JSONs into a full policy document.
	combineRules := func(rules ...json.RawMessage) json.RawMessage {
		type policy struct {
			OnHTTPRequest []json.RawMessage `json:"on_http_request"`
		}
		b, _ := json.Marshal(policy{OnHTTPRequest: rules})
		return b
	}

	userRule := json.RawMessage(`{"name":"auth","actions":[{"type":"deny","config":{"status_code":401}}]}`)
	fwdRule := makeRuleJSON("Generated-Route", "forward-internal")
	fallback404 := makeRuleJSON("Fallback-404", "custom-response")

	t.Run("nil policy returns nil", func(t *testing.T) {
		rules, err := extractPolicyPrefix(nil)
		if err != nil || rules != nil {
			t.Errorf("expected (nil, nil), got (%v, %v)", rules, err)
		}
	})

	t.Run("strips single forward-internal", func(t *testing.T) {
		prefix, err := extractPolicyPrefix(combineRules(fwdRule))
		if err != nil {
			t.Fatal(err)
		}
		if len(prefix) != 0 {
			t.Errorf("expected 0 prefix rules, got %d", len(prefix))
		}
	})

	t.Run("strips forward-internal + custom-response 404", func(t *testing.T) {
		prefix, err := extractPolicyPrefix(combineRules(fwdRule, fallback404))
		if err != nil {
			t.Fatal(err)
		}
		if len(prefix) != 0 {
			t.Errorf("expected 0 prefix rules, got %d", len(prefix))
		}
	})

	t.Run("preserves user rule before operator suffix", func(t *testing.T) {
		prefix, err := extractPolicyPrefix(combineRules(userRule, fwdRule, fallback404))
		if err != nil {
			t.Fatal(err)
		}
		if len(prefix) != 1 {
			t.Errorf("expected 1 prefix rule, got %d", len(prefix))
		}
	})

	t.Run("stops stripping when rule has multiple actions", func(t *testing.T) {
		multiActionRule := json.RawMessage(`{"name":"multi","actions":[{"type":"forward-internal","config":{"url":"x"}},{"type":"custom-response","config":{"status_code":200}}]}`)
		prefix, err := extractPolicyPrefix(combineRules(multiActionRule))
		if err != nil {
			t.Fatal(err)
		}
		if len(prefix) != 1 {
			t.Errorf("multi-action rule should not be stripped, got %d rules", len(prefix))
		}
	})
}

func TestBuildCloudEndpointPolicy(t *testing.T) {
	canary := "https://my-rollout-ngrok-p2-canary-default.internal"
	stable := "https://my-rollout-stable.internal"

	getRuleNames := func(t *testing.T, p json.RawMessage) []string {
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
		names := make([]string, len(parsed.OnHTTPRequest))
		for i, r := range parsed.OnHTTPRequest {
			names[i] = r.Name
		}
		return names
	}

	hasExpression := func(t *testing.T, p json.RawMessage, ruleName string) bool {
		t.Helper()
		var parsed struct {
			OnHTTPRequest []struct {
				Name        string   `json:"name"`
				Expressions []string `json:"expressions"`
			} `json:"on_http_request"`
		}
		if err := json.Unmarshal(p, &parsed); err != nil {
			t.Fatalf("parse: %v", err)
		}
		for _, r := range parsed.OnHTTPRequest {
			if r.Name == ruleName {
				return len(r.Expressions) > 0
			}
		}
		return false
	}

	t.Run("weight 0: only stable rule", func(t *testing.T) {
		p, err := buildCloudEndpointPolicy(nil, 0, canary, stable)
		if err != nil {
			t.Fatal(err)
		}
		names := getRuleNames(t, p)
		if len(names) != 1 || names[0] != "ngrok-rollout-stable" {
			t.Errorf("rules = %v", names)
		}
	})

	t.Run("weight 50: canary then stable", func(t *testing.T) {
		p, err := buildCloudEndpointPolicy(nil, 50, canary, stable)
		if err != nil {
			t.Fatal(err)
		}
		names := getRuleNames(t, p)
		if len(names) != 2 || names[0] != "ngrok-rollout-canary" || names[1] != "ngrok-rollout-stable" {
			t.Errorf("rules = %v", names)
		}
		if !hasExpression(t, p, "ngrok-rollout-canary") {
			t.Error("canary rule should have a rand.double() expression")
		}
		if hasExpression(t, p, "ngrok-rollout-stable") {
			t.Error("stable rule should be unconditional")
		}
	})

	t.Run("weight 100: only canary (unconditional)", func(t *testing.T) {
		p, err := buildCloudEndpointPolicy(nil, 100, canary, stable)
		if err != nil {
			t.Fatal(err)
		}
		names := getRuleNames(t, p)
		if len(names) != 1 || names[0] != "ngrok-rollout-canary" {
			t.Errorf("rules = %v", names)
		}
		if hasExpression(t, p, "ngrok-rollout-canary") {
			t.Error("weight=100 canary rule should be unconditional")
		}
	})

	t.Run("prefix rules appear before routing rules", func(t *testing.T) {
		prefix := []json.RawMessage{
			json.RawMessage(`{"name":"auth","actions":[{"type":"deny","config":{"status_code":401}}]}`),
		}
		p, err := buildCloudEndpointPolicy(prefix, 50, canary, stable)
		if err != nil {
			t.Fatal(err)
		}
		names := getRuleNames(t, p)
		if len(names) != 3 || names[0] != "auth" {
			t.Errorf("rules = %v", names)
		}
	})
}

func TestSaveLoadPrefixAnnotation(t *testing.T) {
	t.Run("round-trip", func(t *testing.T) {
		rules := []json.RawMessage{
			json.RawMessage(`{"name":"rule1"}`),
			json.RawMessage(`{"name":"rule2"}`),
		}
		encoded, err := savePrefixToAnnotation(rules)
		if err != nil {
			t.Fatal(err)
		}
		got, err := loadPrefixFromAnnotation(encoded)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Errorf("expected 2 rules after round-trip, got %d", len(got))
		}
	})

	t.Run("empty string returns nil", func(t *testing.T) {
		got, err := loadPrefixFromAnnotation("")
		if err != nil || got != nil {
			t.Errorf("expected (nil, nil), got (%v, %v)", got, err)
		}
	})

	t.Run("invalid base64 returns error", func(t *testing.T) {
		if _, err := loadPrefixFromAnnotation("not-valid-base64!!!"); err == nil {
			t.Error("expected error")
		}
	})
}

func TestCloudEndpointForwardsTo(t *testing.T) {
	policy := forwardInternalPolicy("https://my-stable.internal")

	if !cloudEndpointForwardsTo(policy, "https://my-stable.internal") {
		t.Error("expected true for matching URL")
	}
	if cloudEndpointForwardsTo(policy, "https://other.internal") {
		t.Error("expected false for non-matching URL")
	}
	if cloudEndpointForwardsTo(nil, "https://my-stable.internal") {
		t.Error("expected false for nil policy")
	}
}

// ---------------------------------------------------------------------------
// Integration tests (fake k8s client)
// ---------------------------------------------------------------------------

func getAEP(r *NgrokTrafficRouter, ns, name string) (*ngrokv1alpha1.AgentEndpoint, error) {
	aep := &ngrokv1alpha1.AgentEndpoint{}
	err := r.k8sClient.Get(context.Background(), client.ObjectKey{Name: name, Namespace: ns}, aep)
	if errors.IsNotFound(err) {
		return nil, nil
	}
	return aep, err
}

func getCE(r *NgrokTrafficRouter, ns, name string) (*ngrokv1alpha1.CloudEndpoint, error) {
	ce := &ngrokv1alpha1.CloudEndpoint{}
	err := r.k8sClient.Get(context.Background(), client.ObjectKey{Name: name, Namespace: ns}, ce)
	if errors.IsNotFound(err) {
		return nil, nil
	}
	return ce, err
}

func TestSetWeightCloudEndpoint_FirstCall(t *testing.T) {
	const (
		ns        = "default"
		stableURL = "https://rollout-demo-stable-default.internal"
	)

	stableAEP := makeOperatorAEP(ns, "stable-aep", stableURL, "http://stable-svc.default:80")
	ce := makeCloudEndpoint(ns, "my-ce", "https://my-app.ngrok.app", forwardInternalPolicy(stableURL))
	rollout := makeRollout(ns, "my-rollout", "stable-svc", "canary-svc", PluginConfig{UseTrafficPolicy: true})

	router := makeRouter(stableAEP, ce)
	rpcErr := router.SetWeight(rollout, 50, nil)
	if rpcErr.ErrorString != "" {
		t.Fatalf("SetWeight error: %s", rpcErr.ErrorString)
	}

	// Canary AEP should be created
	canaryName := phase2CanaryEndpointName(rollout)
	canary, err := getAEP(router, ns, canaryName)
	if err != nil {
		t.Fatalf("getting canary AEP: %v", err)
	}
	if canary == nil {
		t.Fatal("canary AEP was not created")
	}

	// CloudEndpoint should have rollout annotations set
	gotCE, err := getCE(router, ns, "my-ce")
	if err != nil {
		t.Fatalf("getting CE: %v", err)
	}
	if gotCE.Annotations[rolloutManagedAnnotation] != "true" {
		t.Error("rollout-managed annotation not set on CE")
	}
	if gotCE.Annotations[rolloutStableURLAnnotation] != stableURL {
		t.Errorf("stable URL annotation = %q, want %q", gotCE.Annotations[rolloutStableURLAnnotation], stableURL)
	}

	// Policy should have canary rule first
	threshold, found, err := extractCanaryThreshold(gotCE.Spec.TrafficPolicy.Policy)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("canary threshold not found in policy")
	}
	if threshold < 0.49 || threshold > 0.51 {
		t.Errorf("threshold = %.4f, want ~0.5000", threshold)
	}
}

func TestSetWeightCloudEndpoint_ZeroWeight_NoOwnership(t *testing.T) {
	const ns = "default"

	stableAEP := makeOperatorAEP(ns, "stable-aep", "https://stable.internal", "http://stable-svc.default:80")
	ce := makeCloudEndpoint(ns, "my-ce", "https://my-app.ngrok.app", forwardInternalPolicy("https://stable.internal"))
	rollout := makeRollout(ns, "my-rollout", "stable-svc", "canary-svc", PluginConfig{UseTrafficPolicy: true})

	router := makeRouter(stableAEP, ce)
	rpcErr := router.SetWeight(rollout, 0, nil)
	if rpcErr.ErrorString != "" {
		t.Fatalf("SetWeight error: %s", rpcErr.ErrorString)
	}

	// CE should NOT be touched when weight=0 and not owned
	gotCE, _ := getCE(router, ns, "my-ce")
	if gotCE.Annotations[rolloutManagedAnnotation] == "true" {
		t.Error("plugin should not take ownership at weight=0 when not previously owned")
	}
}

func TestRemoveManagedRoutes_CloudEndpoint(t *testing.T) {
	const (
		ns        = "default"
		stableURL = "https://stable.internal"
	)

	stableAEP := makeOperatorAEP(ns, "stable-aep", stableURL, "http://stable-svc.default:80")

	// CE is in mid-rollout state (plugin owns it)
	ce := makeCloudEndpoint(ns, "my-ce", "https://my-app.ngrok.app", forwardInternalPolicy(stableURL))
	ce.Annotations = map[string]string{
		rolloutManagedAnnotation:      "true",
		rolloutStableURLAnnotation:    stableURL,
		rolloutPolicyPrefixAnnotation: "bnVsbA==", // base64("null") — empty prefix
	}

	rollout := makeRollout(ns, "my-rollout", "stable-svc", "canary-svc", PluginConfig{UseTrafficPolicy: true})
	canaryName := phase2CanaryEndpointName(rollout)
	canaryAEP := makeOperatorAEP(ns, canaryName, phase2CanaryInternalURL(rollout), "http://canary-svc.default:80")

	router := makeRouter(stableAEP, ce, canaryAEP)
	rpcErr := router.RemoveManagedRoutes(rollout)
	if rpcErr.ErrorString != "" {
		t.Fatalf("RemoveManagedRoutes error: %s", rpcErr.ErrorString)
	}

	// All three annotations should be removed
	gotCE, _ := getCE(router, ns, "my-ce")
	if gotCE.Annotations[rolloutManagedAnnotation] == "true" {
		t.Error("rollout-managed annotation should be removed")
	}

	// Canary AEP should be deleted
	if canary, _ := getAEP(router, ns, canaryName); canary != nil {
		t.Error("canary AEP should be deleted after RemoveManagedRoutes")
	}
}
