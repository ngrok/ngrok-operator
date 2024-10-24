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
		msg              json.RawMessage
		expectedIsLegacy bool
	}{
		{
			name:             "json has top-level 'inbound' legacy keys",
			msg:              []byte(`{"inbound":"some_val", "on_http_request": {"a": "b"}}`),
			expectedIsLegacy: true,
		},
		{
			name:             "json has top-level 'outbound' legacy keys",
			msg:              []byte(`{"top_level_key":"some_val","outbound":{"eleven":"twelve"}}`),
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
		{
			name:             "message is not valid json",
			msg:              []byte(`ngrok is All-in-one API gateway, Kubernetes Ingress, DDoS protection, firewall, and global load balancing as a service.`),
			expectedIsLegacy: false,
		},
		{
			name:             "message is empty json message",
			msg:              []byte(""),
			expectedIsLegacy: false,
		},
		{
			name:             "message is nil",
			msg:              nil,
			expectedIsLegacy: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			isLegacy := IsLegacyPolicy(tc.msg)
			assert.Equal(t, tc.expectedIsLegacy, isLegacy)
		})
	}
}

func TestExtractEnabledField(t *testing.T) {
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
			expectedReturnedMsg:   []byte(""),
			expectedSetEnabledVal: nil,
			expectedErr:           nil,
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

			returnedMsg, setEnabledVal, err := ExtractEnabledField(tc.msg)

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
