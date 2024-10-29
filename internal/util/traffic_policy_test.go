package util

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsLegacyPolicy(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		trafficPolicy    map[string][]RawRule
		expectedIsLegacy bool
	}{
		{
			name: "has legacy inbound",
			trafficPolicy: map[string][]RawRule{
				LegacyPhaseInbound: nil,
				"some-other-phase": nil,
			},
			expectedIsLegacy: true,
		},
		{
			name: "has legacy outbound",
			trafficPolicy: map[string][]RawRule{
				LegacyPhaseOutbound: nil,
				"some-other-phase":  nil,
			},
			expectedIsLegacy: true,
		},
		{
			name: "has both legacy fields",
			trafficPolicy: map[string][]RawRule{
				LegacyPhaseOutbound: nil,
				LegacyPhaseInbound:  nil,
			},
			expectedIsLegacy: true,
		},
		{
			name: "has no legacy names anywhere",
			trafficPolicy: map[string][]RawRule{
				PhaseOnHttpRequest:  nil,
				PhaseOnHttpResponse: nil,
			},
			expectedIsLegacy: false,
		},
		{
			name: "had legacy name, not as top level key",
			trafficPolicy: map[string][]RawRule{
				PhaseOnHttpRequest:  {[]byte(LegacyPhaseOutbound), []byte(LegacyPhaseOutbound)},
				PhaseOnHttpResponse: {[]byte(LegacyPhaseOutbound), []byte(LegacyPhaseOutbound)},
			},
			expectedIsLegacy: false,
		},
		{
			name:             "nil map",
			expectedIsLegacy: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tpImpl := trafficPolicyImpl{
				trafficPolicy: tc.trafficPolicy,
			}

			assert.Equal(t, tc.expectedIsLegacy, tpImpl.IsLegacyPolicy())
		})
	}

}

func OldTestIsLegacyPolicy(t *testing.T) {
	t.Helper()

	_ = []struct {
		name             string
		msg              json.RawMessage
		expectedIsLegacy bool
	}{
		{
			name:             "json has top-level 'inbound' legacy keys",
			msg:              []byte(`{"inbound":["some_val"], "on_http_request":[{"a": "b"}]}`),
			expectedIsLegacy: true,
		},
		{
			name:             "json has top-level 'outbound' legacy keys",
			msg:              []byte(`{"top_level_key":["some_val"],"outbound":[{"eleven":"twelve"}]}`),
			expectedIsLegacy: true,
		},
		{
			name:             "json only has phase-based naming top-level keys",
			msg:              []byte(`{"on_tcp_connect":"some_val","on_http_request":{"eleven":"twelve"},"on_http_response":"hello"}`),
			expectedIsLegacy: false,
		},
		{
			name:             "legacy key exists, but not at top level",
			msg:              []byte(`{"on_tcp_connect":"inbound"}`),
			expectedIsLegacy: false,
		},
	}

}

func TestConvertLegacyDirectionsToPhases(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name                  string
		trafficPolicy         *trafficPolicyImpl
		expectedTrafficPolicy map[string][]RawRule
	}{
		{
			name: "has inbound and outbound legacy phases",
			trafficPolicy: &trafficPolicyImpl{
				trafficPolicy: map[string][]RawRule{
					LegacyPhaseInbound:  {[]byte(`from inbound`)},
					LegacyPhaseOutbound: {[]byte(`from outbound`)},
					"some-other-phase":  {[]byte(`my-phase`)},
				},
			},
			expectedTrafficPolicy: map[string][]RawRule{
				PhaseOnHttpRequest:  {[]byte(`from inbound`)},
				PhaseOnHttpResponse: {[]byte(`from outbound`)},
				"some-other-phase":  {[]byte(`my-phase`)},
			},
		},
		{
			name: "inbound and outbound merged into existing phases",
			trafficPolicy: &trafficPolicyImpl{
				trafficPolicy: map[string][]RawRule{
					LegacyPhaseInbound:  {[]byte(`from inbound`)},
					LegacyPhaseOutbound: {[]byte(`from outbound`)},
					PhaseOnHttpRequest:  {[]byte(PhaseOnHttpRequest)},
					PhaseOnHttpResponse: {[]byte(PhaseOnHttpResponse)},
				},
			},
			expectedTrafficPolicy: map[string][]RawRule{
				PhaseOnHttpRequest:  {[]byte(`from inbound`), []byte(PhaseOnHttpRequest)},
				PhaseOnHttpResponse: {[]byte(`from outbound`), []byte(PhaseOnHttpResponse)},
			},
		},
		{
			name: "had no legacy phases",
			trafficPolicy: &trafficPolicyImpl{
				trafficPolicy: map[string][]RawRule{
					PhaseOnHttpRequest:  {[]byte(PhaseOnHttpRequest)},
					PhaseOnHttpResponse: {[]byte(PhaseOnHttpResponse)},
				},
			},
			expectedTrafficPolicy: map[string][]RawRule{
				PhaseOnHttpRequest:  {[]byte(PhaseOnHttpRequest)},
				PhaseOnHttpResponse: {[]byte(PhaseOnHttpResponse)},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tc.trafficPolicy.ConvertLegacyDirectionsToPhases()

			actualTrafficPolicy := tc.trafficPolicy.trafficPolicy
			// the impl iterates over map keys, which is not deterministic. We need to test somewhat more manually here instead
			// of directly comparing.
			assert.Equal(t, len(tc.expectedTrafficPolicy), len(actualTrafficPolicy))
			assert.ElementsMatch(t, tc.expectedTrafficPolicy[PhaseOnHttpRequest], actualTrafficPolicy[PhaseOnHttpRequest])
			assert.ElementsMatch(t, tc.expectedTrafficPolicy[PhaseOnHttpResponse], actualTrafficPolicy[PhaseOnHttpResponse])
		})
	}
}

func TestToCRDJson(t *testing.T) {
	t.Parallel()

	trueVal := true

	testCases := []struct {
		name          string
		trafficPolicy *trafficPolicyImpl
		expectedJson  json.RawMessage
		expectedErr   error
	}{
		{
			name: "no enabled set, just trafficPolicy gets marshalled",
			trafficPolicy: &trafficPolicyImpl{
				trafficPolicy: map[string][]RawRule{
					PhaseOnHttpRequest: {[]byte(`{"a":"b"}`)},
				},
			},
			expectedJson: []byte(`{"on_http_request":[{"a":"b"}]}`),
		},
		{
			name: "enabled set, in json",
			trafficPolicy: &trafficPolicyImpl{
				trafficPolicy: map[string][]RawRule{
					PhaseOnHttpRequest: {[]byte(`{"a":"b"}`)},
				},
				enabled: &trueVal,
			},
			expectedJson: []byte(`{"enabled":true,"on_http_request":[{"a":"b"}]}`),
		},
		{
			name: "invalid json",
			trafficPolicy: &trafficPolicyImpl{
				trafficPolicy: map[string][]RawRule{
					PhaseOnHttpRequest: {[]byte(`ngrok is built to deliver applications and APIs with  zero networking configuration and zero hardware`)},
				},
			},
			expectedErr: fmt.Errorf(`json: error calling MarshalJSON for type json.RawMessage: invalid character 'g' in literal null (expecting 'u')`),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			crdJson, err := tc.trafficPolicy.ToCRDJson()

			assert.Equal(t, tc.expectedJson, crdJson)
			if tc.expectedErr == nil {
				assert.NoError(t, err)
			} else {
				// Can't compare the exact error as we don't have access to json SyntaxError underlying `msg` field`
				assert.Equal(t, tc.expectedErr.Error(), err.Error())
			}
		})
	}
}

func TestFilterEnabled(t *testing.T) {
	t.Parallel()

	expectedTrue := true
	expectedFalse := false

	testCases := []struct {
		name                  string
		msg                   json.RawMessage
		expectedReturnedMsg   json.RawMessage
		expectedSetEnabledVal *bool
		expectedErr           error
	}{
		{
			name:                  "message has enabled in top level field (true)",
			msg:                   []byte(`{"enabled":true,"on_tcp_connect":"some_val"}`),
			expectedReturnedMsg:   []byte(`{"on_tcp_connect":"some_val"}`),
			expectedSetEnabledVal: &expectedTrue,
		},
		{
			name:                  "message has enabled in top level field (false)",
			msg:                   []byte(`{"enabled":false,"on_tcp_connect":"some_val"}`),
			expectedReturnedMsg:   []byte(`{"on_tcp_connect":"some_val"}`),
			expectedSetEnabledVal: &expectedFalse,
		},
		{
			name:                  "message is valid, enabled isn't present whatsoever",
			msg:                   []byte(`{"on_tcp_connect":{"config":"yes"}}`),
			expectedReturnedMsg:   []byte(`{"on_tcp_connect":{"config":"yes"}}`),
			expectedSetEnabledVal: nil,
			expectedErr:           nil,
		},
		{
			name:                  "message is valid, enabled isn't top level",
			msg:                   []byte(`{"on_tcp_connect":{"enabled":true}}`),
			expectedReturnedMsg:   []byte(`{"on_tcp_connect":{"enabled":true}}`),
			expectedSetEnabledVal: nil,
			expectedErr:           nil,
		},
		{
			name:                  "message is entirely invalid",
			msg:                   []byte(`Industry leaders rely on ngrok`),
			expectedReturnedMsg:   nil,
			expectedSetEnabledVal: nil,
			expectedErr:           fmt.Errorf("invalid character 'I' looking for beginning of value"),
		},
		{
			name:                  "message is empty json",
			msg:                   []byte(""),
			expectedReturnedMsg:   nil,
			expectedSetEnabledVal: nil,
			expectedErr:           fmt.Errorf("unexpected end of JSON input"),
		},
		{
			name:                  "message is nil",
			msg:                   nil,
			expectedReturnedMsg:   nil,
			expectedSetEnabledVal: nil,
			expectedErr:           nil,
		},
		{
			name:                  "enabled present but doesn't map to anything meaningful",
			msg:                   []byte(`{"enabled":"howdidthisgethere","on_http_request":{"config":"yes"}}`),
			expectedReturnedMsg:   []byte(`{"on_http_request":{"config":"yes"}}`),
			expectedSetEnabledVal: nil,
			expectedErr:           nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			returnedMsg, setEnabledVal, err := filterEnabled(tc.msg)

			assert.Equal(t, tc.expectedReturnedMsg, returnedMsg)
			assert.Equal(t, tc.expectedSetEnabledVal, setEnabledVal)
			if tc.expectedErr == nil {
				assert.NoError(t, err)
			} else {
				// Can't compare the exact error as we don't have access to json SyntaxError underlying `msg` field`
				assert.Equal(t, tc.expectedErr.Error(), err.Error())
			}
		})
	}
}

func TestMerge(t *testing.T) {
	t.Parallel()

	// used so we can get pointers to these values
	trueVal := true
	falseVal := false

	testCases := []struct {
		name                        string
		addedTrafficPolicy          trafficPolicyImpl
		baseTrafficPolicyEnabled    *bool
		expectedMergedTrafficPolicy trafficPolicyImpl
	}{
		{
			name: "added traffic policy, existing and new phases",
			addedTrafficPolicy: trafficPolicyImpl{
				trafficPolicy: map[string][]RawRule{
					PhaseOnHttpRequest: {
						[]byte(`b`),
					},
					PhaseOnHttpResponse: {
						[]byte(`c`),
					},
				}},
			expectedMergedTrafficPolicy: trafficPolicyImpl{
				trafficPolicy: map[string][]RawRule{
					PhaseOnHttpRequest: {
						[]byte(`a`),
						[]byte(`b`),
					},
					PhaseOnHttpResponse: {
						[]byte(`c`),
					},
				}},
		},
		{
			name:                        "base traffic policy has enabled set, added doesn't",
			baseTrafficPolicyEnabled:    &trueVal,
			expectedMergedTrafficPolicy: *newBaseTrafficPolicy(t, &trueVal),
		},
		{
			name: "base traffic policy has no enabled set, added does",
			addedTrafficPolicy: trafficPolicyImpl{
				enabled: &trueVal,
			},
			expectedMergedTrafficPolicy: *newBaseTrafficPolicy(t, &trueVal),
		},
		{
			name:                     "both have enabled set, base is false",
			baseTrafficPolicyEnabled: &falseVal,
			addedTrafficPolicy: trafficPolicyImpl{
				enabled: &trueVal,
			},
			expectedMergedTrafficPolicy: *newBaseTrafficPolicy(t, &trueVal),
		},
		{
			name:                     "both have enabled set, added is false",
			baseTrafficPolicyEnabled: &trueVal,
			addedTrafficPolicy: trafficPolicyImpl{
				enabled: &falseVal,
			},
			expectedMergedTrafficPolicy: *newBaseTrafficPolicy(t, &trueVal),
		},
		{
			name:                     "both have enabled set, both false",
			baseTrafficPolicyEnabled: &falseVal,
			addedTrafficPolicy: trafficPolicyImpl{
				enabled: &falseVal,
			},
			expectedMergedTrafficPolicy: *newBaseTrafficPolicy(t, &falseVal),
		},
		{
			name:                     "both have enabled set, both true",
			baseTrafficPolicyEnabled: &trueVal,
			addedTrafficPolicy: trafficPolicyImpl{
				enabled: &trueVal,
			},
			expectedMergedTrafficPolicy: *newBaseTrafficPolicy(t, &trueVal),
		},
		{
			name: "empty added map",
			addedTrafficPolicy: trafficPolicyImpl{
				trafficPolicy: map[string][]RawRule{},
			},
			expectedMergedTrafficPolicy: *newBaseTrafficPolicy(t, nil),
		},
		{
			name:                        "nil added map",
			addedTrafficPolicy:          trafficPolicyImpl{},
			expectedMergedTrafficPolicy: *newBaseTrafficPolicy(t, nil),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			baseTrafficPolicy := newBaseTrafficPolicy(t, tc.baseTrafficPolicyEnabled)

			baseTrafficPolicy.Merge(&tc.addedTrafficPolicy)

			assert.Equal(t, tc.expectedMergedTrafficPolicy, *baseTrafficPolicy)
		})
	}
}

func TestMergeEndpointRule(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                        string
		addedRule                   EndpointRule
		addedPhase                  string
		expectedMergedTrafficPolicy trafficPolicyImpl
		expectedErr                 error
	}{
		{
			name: "rule added to existing phase",
			addedRule: EndpointRule{
				Name: "test-rule",
				Actions: []RawAction{
					[]byte(`{"c":"d"}`),
				},
			},
			addedPhase: PhaseOnHttpRequest,
			expectedMergedTrafficPolicy: trafficPolicyImpl{
				trafficPolicy: map[string][]RawRule{
					PhaseOnHttpRequest: {
						[]byte(`a`),
						[]byte(`{"name":"test-rule","actions":[{"c":"d"}]}`),
					},
				},
			},
		},
		{
			name: "rule added to new phase",
			addedRule: EndpointRule{
				Name: "test-rule",
				Actions: []RawAction{
					[]byte(`{"c":"d"}`),
				},
			},
			addedPhase: PhaseOnHttpResponse,
			expectedMergedTrafficPolicy: trafficPolicyImpl{
				trafficPolicy: map[string][]RawRule{
					PhaseOnHttpRequest: {
						[]byte(`a`),
					},
					PhaseOnHttpResponse: {
						[]byte(`{"name":"test-rule","actions":[{"c":"d"}]}`),
					},
				}},
		},
		{
			name: "malformed json",
			addedRule: EndpointRule{
				Name: "test-rule",
				Actions: []RawAction{
					[]byte(`invalid-json`),
				},
			},
			addedPhase: PhaseOnHttpRequest,
			// original traffic policy should be unaffected
			expectedMergedTrafficPolicy: *newBaseTrafficPolicy(t, nil),
			expectedErr:                 fmt.Errorf("json: error calling MarshalJSON for type json.RawMessage: invalid character 'i' looking for beginning of value"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			baseTrafficPolicy := newBaseTrafficPolicy(t, nil)

			err := baseTrafficPolicy.MergeEndpointRule(tc.addedRule, tc.addedPhase)

			assert.Equal(t, tc.expectedMergedTrafficPolicy, *baseTrafficPolicy)
			if tc.expectedErr == nil {
				assert.NoError(t, err)
			} else {
				// Can't compare the exact error as we don't have access to json SyntaxError underlying `msg` field`
				assert.Equal(t, tc.expectedErr.Error(), err.Error())
			}
		})
	}
}

func TestMergeSinglePhase(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                        string
		addedRules                  []RawRule
		addedPhase                  string
		expectedMergedTrafficPolicy map[string][]RawRule
	}{
		{
			name: "rules merged into existing phase",
			addedRules: []RawRule{
				[]byte(`b`),
				[]byte(`c`),
			},
			addedPhase: PhaseOnHttpRequest,
			expectedMergedTrafficPolicy: map[string][]RawRule{
				PhaseOnHttpRequest: {
					[]byte(`a`),
					[]byte(`b`),
					[]byte(`c`),
				},
			},
		},
		{
			name: "rules merged into non-existing phase",
			addedRules: []RawRule{
				[]byte(`b`),
				[]byte(`c`),
			},
			addedPhase: PhaseOnHttpResponse,
			expectedMergedTrafficPolicy: map[string][]RawRule{
				PhaseOnHttpRequest: {
					[]byte(`a`),
				},
				PhaseOnHttpResponse: {
					[]byte(`b`),
					[]byte(`c`),
				},
			},
		},
		{
			name:                        "empty added rules",
			addedRules:                  []RawRule{},
			addedPhase:                  PhaseOnHttpRequest,
			expectedMergedTrafficPolicy: newBaseTrafficPolicy(t, nil).Deconstruct(),
		},
		{
			name:                        "nil added rules",
			addedRules:                  nil,
			addedPhase:                  PhaseOnHttpResponse,
			expectedMergedTrafficPolicy: newBaseTrafficPolicy(t, nil).Deconstruct(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			baseTrafficPolicy := newBaseTrafficPolicy(t, nil).Deconstruct()

			mergeSinglePhase(baseTrafficPolicy, tc.addedRules, tc.addedPhase)

			assert.Equal(t, tc.expectedMergedTrafficPolicy, baseTrafficPolicy)
		})
	}
}

// newBaseTrafficPolicy gives a simple base that the "merge" functions can use for testing.
func newBaseTrafficPolicy(t *testing.T, enabled *bool) *trafficPolicyImpl {
	t.Helper()

	return &trafficPolicyImpl{
		trafficPolicy: map[string][]RawRule{
			PhaseOnHttpRequest: {
				[]byte(`a`),
			},
		},
		enabled: enabled,
	}

}
