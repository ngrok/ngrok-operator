package util

import "encoding/json"

const (
	// KubernetesOperatorIDMetadataKey is the metadata key used to store the kubernetes operator ID
	KubernetesOperatorIDMetadataKey = "kubernetes-operator-id"
)

// InjectKubernetesOperatorID injects the kubernetes operator ID into the given
// ngrok API metadata JSON string. If operatorID is empty, metadata is returned
// unchanged.
func InjectKubernetesOperatorID(metadata string, operatorID string) string {
	if operatorID == "" {
		return metadata
	}

	m := make(map[string]interface{})
	if metadata != "" {
		_ = json.Unmarshal([]byte(metadata), &m)
	}

	m[KubernetesOperatorIDMetadataKey] = operatorID

	b, err := json.Marshal(m)
	if err != nil {
		return metadata
	}

	return string(b)
}
