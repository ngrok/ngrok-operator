package v1alpha1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// LEGACY-enabledfeatures-format: BEGIN

func TestKubernetesOperatorEnabledFeatures_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		json string
		want KubernetesOperatorEnabledFeatures
	}{
		{
			name: "legacy comma-separated string",
			json: `{"enabledFeatures":"ingress,bindings"}`,
			want: KubernetesOperatorEnabledFeatures{"ingress", "bindings"},
		},
		{
			name: "array",
			json: `{"enabledFeatures":["ingress","bindings"]}`,
			want: KubernetesOperatorEnabledFeatures{"ingress", "bindings"},
		},
		{
			name: "empty legacy string",
			json: `{"enabledFeatures":""}`,
			want: nil,
		},
		{
			name: "absent",
			json: `{}`,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var status KubernetesOperatorStatus
			require.NoError(t, json.Unmarshal([]byte(tt.json), &status))
			assert.Equal(t, tt.want, status.EnabledFeatures)
		})
	}
}

// The write-side cleanup landed, so the field marshals as a plain array. The
// previous release decodes that, which is what keeps a rollback to it safe.
func TestKubernetesOperatorEnabledFeatures_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		features KubernetesOperatorEnabledFeatures
		want     string
	}{
		{
			name:     "array",
			features: KubernetesOperatorEnabledFeatures{"ingress", "bindings"},
			want:     `{"enabledFeatures":["ingress","bindings"]}`,
		},
		{
			name:     "single feature",
			features: KubernetesOperatorEnabledFeatures{"ingress"},
			want:     `{"enabledFeatures":["ingress"]}`,
		},
		{
			name:     "empty",
			features: KubernetesOperatorEnabledFeatures{},
			want:     `{}`,
		},
		{
			name:     "nil",
			features: nil,
			want:     `{}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := json.Marshal(&KubernetesOperatorStatus{EnabledFeatures: tt.features})
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(encoded))
		})
	}
}

// An object last written by the previous release carries the legacy string.
// The operator recomputes status from the ngrok API and rewrites the whole
// field every reconcile, so decoding then re-encoding is the self-heal that
// converts stored objects to the array form.
func TestKubernetesOperatorEnabledFeatures_LegacyStringRoundTrip(t *testing.T) {
	var status KubernetesOperatorStatus
	require.NoError(t, json.Unmarshal([]byte(`{"enabledFeatures":"ingress,bindings"}`), &status))

	encoded, err := json.Marshal(&status)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"enabledFeatures":["ingress","bindings"]`)
}

// LEGACY-enabledfeatures-format: END
