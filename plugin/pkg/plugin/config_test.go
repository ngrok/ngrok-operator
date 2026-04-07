package plugin

import (
	"encoding/json"
	"testing"

	argorollouts "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestParsePluginConfig(t *testing.T) {
	makeRolloutWithRaw := func(raw json.RawMessage) *argorollouts.Rollout {
		return &argorollouts.Rollout{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			Spec: argorollouts.RolloutSpec{
				Strategy: argorollouts.RolloutStrategy{
					Canary: &argorollouts.CanaryStrategy{
						TrafficRouting: &argorollouts.RolloutTrafficRouting{
							Plugins: map[string]json.RawMessage{"ngrok/ngrok": raw},
						},
					},
				},
			},
		}
	}

	t.Run("defaults TotalPoolSize to 10", func(t *testing.T) {
		cfg, err := parsePluginConfig(makeRolloutWithRaw(json.RawMessage(`{}`)))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.TotalPoolSize != 10 {
			t.Errorf("TotalPoolSize = %d, want 10", cfg.TotalPoolSize)
		}
	})

	t.Run("respects explicit TotalPoolSize", func(t *testing.T) {
		cfg, err := parsePluginConfig(makeRolloutWithRaw(json.RawMessage(`{"totalPoolSize":20}`)))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.TotalPoolSize != 20 {
			t.Errorf("TotalPoolSize = %d, want 20", cfg.TotalPoolSize)
		}
	})

	t.Run("UseTrafficPolicy", func(t *testing.T) {
		cfg, err := parsePluginConfig(makeRolloutWithRaw(json.RawMessage(`{"useTrafficPolicy":true}`)))
		if err != nil {
			t.Fatal(err)
		}
		if !cfg.UseTrafficPolicy {
			t.Error("UseTrafficPolicy should be true")
		}
	})

	t.Run("missing plugin key returns error", func(t *testing.T) {
		r := &argorollouts.Rollout{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			Spec: argorollouts.RolloutSpec{
				Strategy: argorollouts.RolloutStrategy{
					Canary: &argorollouts.CanaryStrategy{
						TrafficRouting: &argorollouts.RolloutTrafficRouting{
							Plugins: map[string]json.RawMessage{},
						},
					},
				},
			},
		}
		if _, err := parsePluginConfig(r); err == nil {
			t.Error("expected error for missing plugin key")
		}
	})

	t.Run("malformed JSON returns error", func(t *testing.T) {
		if _, err := parsePluginConfig(makeRolloutWithRaw(json.RawMessage(`not-json`))); err == nil {
			t.Error("expected error for malformed JSON")
		}
	})
}
