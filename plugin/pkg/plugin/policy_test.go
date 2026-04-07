package plugin

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"
)

func TestCanaryForwardRule(t *testing.T) {
	tests := []struct {
		weight    int32
		wantExpr  string
		wantURL   string
	}{
		{25, "rand.double() <= 0.2500", "https://canary.internal"},
		{50, "rand.double() <= 0.5000", "https://canary.internal"},
		{75, "rand.double() <= 0.7500", "https://canary.internal"},
		{1, "rand.double() <= 0.0100", "https://canary.internal"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("weight=%d", tt.weight), func(t *testing.T) {
			r := canaryForwardRule(tt.weight, tt.wantURL)
			if r.Name != "ngrok-rollout-canary" {
				t.Errorf("name = %q, want %q", r.Name, "ngrok-rollout-canary")
			}
			if len(r.Expressions) != 1 || r.Expressions[0] != tt.wantExpr {
				t.Errorf("expression = %v, want [%q]", r.Expressions, tt.wantExpr)
			}
			if len(r.Actions) != 1 {
				t.Fatalf("actions len = %d, want 1", len(r.Actions))
			}
			cfg, _ := r.Actions[0]["config"].(map[string]interface{})
			if cfg["url"] != tt.wantURL {
				t.Errorf("action url = %v, want %q", cfg["url"], tt.wantURL)
			}
		})
	}
}

func TestUnconditionalForwardRule(t *testing.T) {
	r := unconditionalForwardRule("ngrok-rollout-stable", "https://stable.internal")
	if r.Name != "ngrok-rollout-stable" {
		t.Errorf("name = %q", r.Name)
	}
	if len(r.Expressions) != 0 {
		t.Errorf("expected no expressions, got %v", r.Expressions)
	}
	if len(r.Actions) != 1 {
		t.Fatalf("actions len = %d", len(r.Actions))
	}
	cfg, _ := r.Actions[0]["config"].(map[string]interface{})
	if cfg["url"] != "https://stable.internal" {
		t.Errorf("action url = %v", cfg["url"])
	}
}

func TestMarshalPolicy(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		got := marshalPolicy(nil)
		var p struct {
			OnHTTPRequest []json.RawMessage `json:"on_http_request"`
		}
		if err := json.Unmarshal(got, &p); err != nil {
			t.Fatal(err)
		}
		if len(p.OnHTTPRequest) != 0 {
			t.Errorf("expected empty on_http_request, got %d rules", len(p.OnHTTPRequest))
		}
	})

	t.Run("one rule", func(t *testing.T) {
		rules := []tpRule{canaryForwardRule(50, "https://canary.internal")}
		got := marshalPolicy(rules)
		var p struct {
			OnHTTPRequest []struct {
				Name string `json:"name"`
			} `json:"on_http_request"`
		}
		if err := json.Unmarshal(got, &p); err != nil {
			t.Fatal(err)
		}
		if len(p.OnHTTPRequest) != 1 || p.OnHTTPRequest[0].Name != "ngrok-rollout-canary" {
			t.Errorf("unexpected rules: %+v", p.OnHTTPRequest)
		}
	})
}

func TestExtractCanaryThreshold(t *testing.T) {
	tests := []struct {
		name      string
		policy    json.RawMessage
		wantFound bool
		wantVal   float64
	}{
		{
			name:      "nil policy",
			policy:    nil,
			wantFound: false,
		},
		{
			name:      "empty policy",
			policy:    json.RawMessage(`{}`),
			wantFound: false,
		},
		{
			name:      "25% canary",
			policy:    marshalPolicy([]tpRule{canaryForwardRule(25, "https://canary.internal")}),
			wantFound: true,
			wantVal:   0.25,
		},
		{
			name:      "75% canary",
			policy:    marshalPolicy([]tpRule{canaryForwardRule(75, "https://canary.internal")}),
			wantFound: true,
			wantVal:   0.75,
		},
		{
			name: "100% canary (unconditional)",
			policy: marshalPolicy([]tpRule{
				unconditionalForwardRule("ngrok-rollout-canary", "https://canary.internal"),
			}),
			wantFound: true,
			wantVal:   1.0,
		},
		{
			name:      "no canary rule",
			policy:    marshalPolicy([]tpRule{unconditionalForwardRule("ngrok-rollout-stable", "https://stable.internal")}),
			wantFound: false,
		},
		{
			name:      "malformed json",
			policy:    json.RawMessage(`not-json`),
			wantFound: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			threshold, found, err := extractCanaryThreshold(tt.policy)
			if tt.name == "malformed json" {
				if err == nil {
					t.Error("expected error for malformed json")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if found != tt.wantFound {
				t.Errorf("found = %v, want %v", found, tt.wantFound)
			}
			if tt.wantFound && math.Abs(threshold-tt.wantVal) > 0.0001 {
				t.Errorf("threshold = %.4f, want %.4f", threshold, tt.wantVal)
			}
		})
	}
}
