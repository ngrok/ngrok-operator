package plugin

import (
	"encoding/json"
	"fmt"
)

// tpRule is the wire format for a single traffic policy rule.
type tpRule struct {
	Name        string                   `json:"name,omitempty"`
	Expressions []string                 `json:"expressions,omitempty"`
	Actions     []map[string]interface{} `json:"actions"`
}

func canaryForwardRule(desiredWeight int32, canaryURL string) tpRule {
	threshold := float64(desiredWeight) / 100.0
	return tpRule{
		Name:        "ngrok-rollout-canary",
		Expressions: []string{fmt.Sprintf("rand.double() <= %.4f", threshold)},
		Actions: []map[string]interface{}{{
			"type":   "forward-internal",
			"config": map[string]interface{}{"url": canaryURL},
		}},
	}
}

func unconditionalForwardRule(name, url string) tpRule {
	return tpRule{
		Name: name,
		Actions: []map[string]interface{}{{
			"type":   "forward-internal",
			"config": map[string]interface{}{"url": url},
		}},
	}
}

func marshalPolicy(rules []tpRule) json.RawMessage {
	type policy struct {
		OnHTTPRequest []tpRule `json:"on_http_request"`
	}
	b, _ := json.Marshal(policy{OnHTTPRequest: rules})
	return json.RawMessage(b)
}

// extractCanaryThreshold finds the rand.double() threshold from the "ngrok-rollout-canary"
// rule in the given policy JSON. Returns (threshold, true, nil) if found.
func extractCanaryThreshold(raw json.RawMessage) (threshold float64, found bool, err error) {
	if len(raw) == 0 {
		return 0, false, nil
	}
	var policy struct {
		OnHTTPRequest []struct {
			Name        string   `json:"name"`
			Expressions []string `json:"expressions"`
		} `json:"on_http_request"`
	}
	if err = json.Unmarshal(raw, &policy); err != nil {
		return 0, false, err
	}
	for _, rule := range policy.OnHTTPRequest {
		if rule.Name != "ngrok-rollout-canary" {
			continue
		}
		if len(rule.Expressions) == 0 {
			return 1.0, true, nil // desiredWeight == 100, unconditional
		}
		var t float64
		if _, scanErr := fmt.Sscanf(rule.Expressions[0], "rand.double() <= %f", &t); scanErr == nil {
			return t, true, nil
		}
	}
	return 0, false, nil
}
