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

// This release still writes the legacy string form so a rollback to a
// pre-migration operator can decode it; see MarshalJSON in
// kubernetesoperator_status_compat.go.
func TestKubernetesOperatorEnabledFeatures_MarshalJSON(t *testing.T) {
	var ko KubernetesOperator
	require.NoError(t, json.Unmarshal([]byte(`{
		"apiVersion":"ngrok.k8s.ngrok.com/v1alpha1",
		"kind":"KubernetesOperator",
		"status":{"enabledFeatures":["ingress","bindings"]}
	}`), &ko))

	encoded, err := json.Marshal(&ko)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"enabledFeatures":"ingress,bindings"`)
}

// LEGACY-enabledfeatures-format: END
