package trafficpolicy

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
)

func assertTrafficPolicyContent(t *testing.T, tp *TrafficPolicy, expected string) {
	content, err := json.Marshal(tp)
	assert.NoError(t, err)
	assert.JSONEq(t, expected, string(content))
}

func loadTestData(name string) string {
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func TestEmptyTrafficPolicy(t *testing.T) {
	tp := NewTrafficPolicy()
	assert.True(t, tp.IsEmpty())
	assertTrafficPolicyContent(t, tp, `{}`)
}

func TestTrafficPolicy(t *testing.T) {
	tp := NewTrafficPolicy()
	if tp == nil {
		t.Error("TrafficPolicy is nil")
	}

	tp.AddRuleOnHTTPRequest(
		Rule{
			Name: "test-name",
			Actions: []Action{
				NewWebhookVerificationAction("github", "secret"),
			},
		},
	)
	assertTrafficPolicyContent(t, tp, loadTestData("policy-1.json"))

	tp = NewTrafficPolicy()
	tp.AddRuleOnTCPConnect(
		Rule{
			Expressions: []string{"[1,2,3].all(x, x > 0)"},
			Actions: []Action{
				NewRestricIPsActionFromIPPolicies([]string{"ipp_123", "ipp_456"}),
				NewTerminateTLSAction(TLSTerminationConfig{MinVersion: ptr.To("1.2")}),
			},
		},
	)
	tp.AddRuleOnHTTPRequest(
		Rule{
			Actions: []Action{
				NewCircuitBreakerAction(0.10, nil, nil, ptr.To(2*time.Minute)),
			},
		},
	)

	tp.AddRuleOnHTTPResponse(
		Rule{
			Actions: []Action{
				NewAddHeadersAction(map[string]string{
					"X-Header-1": "value1",
					"X-Header-2": "value2",
				}),
				NewRemoveHeadersAction([]string{
					"X-Header-3",
					"X-Header-4",
				}),
				NewCompressResponseAction(nil),
			},
		},
	)

	assertTrafficPolicyContent(t, tp, loadTestData("policy-2.json"))
}
