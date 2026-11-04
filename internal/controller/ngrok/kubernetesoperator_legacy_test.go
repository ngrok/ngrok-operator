package ngrok

// LEGACY-kubernetesoperator-kind: delete this whole file at cleanup, together
// with kubernetesoperator_legacy.go. The twin behavior is covered by envtest in
// kubernetesoperator_controller_test.go; this covers only the read errors
// envtest cannot produce.

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestLegacyTwinReadSkipsDrain(t *testing.T) {
	legacyGR := schema.GroupResource{Group: "ngrok.k8s.ngrok.com", Resource: "kubernetesoperators"}

	tests := []struct {
		name     string
		err      error
		wantSkip bool
		wantErr  bool
	}{
		{name: "legacy twin present skips", wantSkip: true},
		{name: "legacy read forbidden skips", err: apierrors.NewForbidden(legacyGR, "ko", errors.New("rbac")), wantSkip: true},
		{name: "no legacy twin drains", err: apierrors.NewNotFound(legacyGR, "ko")},
		{name: "legacy kind not installed drains", err: &meta.NoKindMatchError{GroupKind: schema.GroupKind{Group: "ngrok.k8s.ngrok.com", Kind: "KubernetesOperator"}}},
		{name: "other error requeues", err: errors.New("boom"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skip, err := legacyTwinReadSkipsDrain(tt.err)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantSkip, skip)
		})
	}
}
