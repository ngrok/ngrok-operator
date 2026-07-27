package v1alpha1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// LEGACY-STATUS-MIGRATION: BEGIN

func TestKubernetesOperatorEnabledFeatures_LegacyStatusCompatibility(t *testing.T) {
	var ko KubernetesOperator
	require.NoError(t, json.Unmarshal([]byte(`{
		"apiVersion":"ngrok.k8s.ngrok.com/v1alpha1",
		"kind":"KubernetesOperator",
		"status":{"enabledFeatures":"ingress,bindings"}
	}`), &ko))
	assert.Equal(t,
		KubernetesOperatorEnabledFeatures{"ingress", "bindings"},
		ko.Status.EnabledFeatures,
	)

	encoded, err := json.Marshal(&ko)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"enabledFeatures":["ingress","bindings"]`)
}

// LEGACY-STATUS-MIGRATION: END
