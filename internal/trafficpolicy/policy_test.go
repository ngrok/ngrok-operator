package trafficpolicy

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			Expressions: []string{"req.url.path == '/example'"},
			Actions: []Action{
				NewCustomResponseAction(404, "Not Found", nil),
			},
		},
	)
	tp.AddRuleOnHTTPRequest(
		Rule{
			Actions: []Action{
				NewCircuitBreakerAction(0.10, nil, nil, ptr.To(2*time.Minute)),
				NewOAuthAction(OAuthConfig{
					Provider: "google",
				}),
				NewForwardInternalAction("http://test.internal:8080"),
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

func TestContainsAction(t *testing.T) {
	tp := NewTrafficPolicy()

	for _, actionType := range ActionTypes() {
		assert.False(t, tp.ContainsAction(actionType))
	}

	tp.AddRuleOnHTTPRequest(
		Rule{
			Actions: []Action{
				NewWebhookVerificationAction("github", "secret"),
			},
		},
	)

	tp.AddRuleOnHTTPResponse(
		Rule{
			Actions: []Action{
				NewAddHeadersAction(map[string]string{
					"X-Header-1": "value1",
				}),
			},
		},
	)

	tp.AddRuleOnTCPConnect(
		Rule{
			Actions: []Action{
				NewRestricIPsActionFromIPPolicies([]string{"ipp_123", "ipp_456"}),
			},
		},
	)

	assert.True(t, tp.ContainsAction(ActionType_VerifyWebhook))
	assert.True(t, tp.ContainsAction(ActionType_AddHeaders))
	assert.True(t, tp.ContainsAction(ActionType_RestrictIPs))

	assert.False(t, tp.ContainsAction(ActionType_TerminateTLS))
	assert.False(t, tp.ContainsAction(ActionType_RemoveHeaders))
	assert.False(t, tp.ContainsAction(ActionType_CompressResponse))
	assert.False(t, tp.ContainsAction(ActionType_Log))
}

func TestTrafficPolicyDeepCopy(t *testing.T) {
	testCases := []struct {
		name          string
		original      *TrafficPolicy
		modifyCopy    func(copy *TrafficPolicy)
		expectedEqual bool
	}{
		{
			name: "DeepCopy with no modifications",
			original: &TrafficPolicy{
				OnHTTPRequest: []Rule{
					{
						Name: "Rule1",
						Expressions: []string{
							"req.url.path == \"/example\"",
						},
						Actions: []Action{
							NewAddHeadersAction(map[string]string{
								"X-Custom-Header": "Value",
							}),
						},
					},
				},
			},
			modifyCopy:    nil,
			expectedEqual: true,
		},
		{
			name: "DeepCopy with empty TrafficPolicy",
			original: &TrafficPolicy{
				OnHTTPRequest:  []Rule{},
				OnHTTPResponse: []Rule{},
				OnTCPConnect:   []Rule{},
			},
			modifyCopy:    nil,
			expectedEqual: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			copy, err := tc.original.DeepCopy()
			require.NoError(t, err, "DeepCopy should not return an error")
			require.NotNil(t, copy, "DeepCopy result should not be nil")

			if tc.modifyCopy != nil {
				tc.modifyCopy(copy)
			}

			if tc.expectedEqual {
				// Use JSONEq to compare the original and the copy for deep equality.
				originalJSON, err := json.Marshal(tc.original)
				require.NoError(t, err, "Failed to marshal original TrafficPolicy")

				copyJSON, err := json.Marshal(copy)
				require.NoError(t, err, "Failed to marshal copied TrafficPolicy")

				assert.JSONEq(t, string(originalJSON), string(copyJSON), "Original and copy should be equal")
			} else {
				assert.NotEqual(t, tc.original, copy, "Original and copy should not be equal after modification")
			}
		})
	}
}
