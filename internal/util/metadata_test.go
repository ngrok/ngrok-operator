package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInjectKubernetesOperatorID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		metadata   string
		operatorID string
		want       string
	}{
		{
			name:       "empty operator ID returns metadata unchanged",
			metadata:   `{"key":"value"}`,
			operatorID: "",
			want:       `{"key":"value"}`,
		},
		{
			name:       "empty metadata with valid operator ID",
			metadata:   "",
			operatorID: "op-123",
			want:       `{"kubernetes-operator-id":"op-123"}`,
		},
		{
			name:       "valid metadata with operator ID injection",
			metadata:   `{"existing":"data"}`,
			operatorID: "op-456",
			want:       `{"existing":"data","kubernetes-operator-id":"op-456"}`,
		},
		{
			name:       "invalid JSON metadata with valid operator ID",
			metadata:   "not-json",
			operatorID: "op-789",
			want:       `{"kubernetes-operator-id":"op-789"}`,
		},
		{
			name:       "metadata already has the key overwrites",
			metadata:   `{"kubernetes-operator-id":"old-id"}`,
			operatorID: "new-id",
			want:       `{"kubernetes-operator-id":"new-id"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := InjectKubernetesOperatorID(tt.metadata, tt.operatorID)
			assert.JSONEq(t, tt.want, got)
		})
	}
}
