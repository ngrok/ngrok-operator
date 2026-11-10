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
		readErr  error
		wantSkip bool
		wantErr  bool
	}{
		{name: "legacy twin present skips", readErr: nil, wantSkip: true, wantErr: false},
		{name: "legacy read forbidden skips", readErr: apierrors.NewForbidden(legacyGR, "ko", errors.New("rbac")), wantSkip: true, wantErr: false},
		{name: "no legacy twin drains", readErr: apierrors.NewNotFound(legacyGR, "ko"), wantSkip: false, wantErr: false},
		{name: "legacy kind not installed drains", readErr: &meta.NoKindMatchError{GroupKind: schema.GroupKind{Group: "ngrok.k8s.ngrok.com", Kind: "KubernetesOperator"}}, wantSkip: false, wantErr: false},
		{name: "other error requeues", readErr: errors.New("boom"), wantSkip: false, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skip, err := legacyTwinReadSkipsDrain(tt.readErr)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantSkip, skip)
		})
	}
}
