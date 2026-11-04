package agent

// LEGACY-agentendpoint-kind: delete this whole file at cleanup. The twin
// behavior is covered by envtest in agent_endpoint_migration_test.go; this
// covers only the read errors envtest cannot produce.

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
)

func TestKeepTunnelForTwinRead(t *testing.T) {
	gr := schema.GroupResource{Group: "ngrok.com", Resource: "agentendpoints"}
	live := &ngrokv1.AgentEndpoint{}
	deleting := &ngrokv1.AgentEndpoint{}
	deleting.DeletionTimestamp = new(metav1.Now())

	tests := []struct {
		name     string
		twin     *ngrokv1.AgentEndpoint
		err      error
		wantKeep bool
		wantErr  bool
	}{
		{name: "live twin keeps", twin: live, wantKeep: true},
		{name: "deleting twin closes", twin: deleting},
		{name: "no twin closes", twin: live, err: apierrors.NewNotFound(gr, "aep")},
		{name: "twin kind not served closes", twin: live, err: &meta.NoKindMatchError{GroupKind: schema.GroupKind{Group: "ngrok.com", Kind: "AgentEndpoint"}}},
		{name: "forbidden read keeps", twin: live, err: apierrors.NewForbidden(gr, "aep", errors.New("rbac")), wantKeep: true},
		{name: "other error requeues", twin: live, err: errors.New("boom"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keep, err := keepTunnelForTwinRead(tt.twin, tt.err)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantKeep, keep)
		})
	}
}

// A legacy AgentEndpoint and its v1 twin must send the same metadata, or the
// take-over reconnects the endpoint.
func TestTwinMetadataMatches(t *testing.T) {
	tests := []struct {
		name   string
		legacy string // spec.metadata as JSON
		v1     map[string]string
	}{
		{name: "spaced string", legacy: `"{\"team\": \"edge\", \"app\": \"x\"}"`, v1: map[string]string{"team": "edge", "app": "x"}},
		{name: "unsorted string", legacy: `"{\"b\":\"2\",\"a\":\"1\"}"`, v1: map[string]string{"a": "1", "b": "2"}},
		{name: "object form", legacy: `{"team":"edge"}`, v1: map[string]string{"team": "edge"}},
		{name: "CRD default", legacy: `"{\"owned-by\":\"ngrok-operator\"}"`, v1: map[string]string{"owned-by": "ngrok-operator"}},
		{name: "absent", legacy: ``, v1: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			legacy := &ngrokv1alpha1.AgentEndpoint{}
			legacy.Spec.Metadata = json.RawMessage(tt.legacy)
			v1 := &ngrokv1.AgentEndpoint{}
			v1.Spec.Metadata = tt.v1
			assert.Equal(t, partsOf(v1).driverSpec.Metadata, partsOf(legacy).driverSpec.Metadata)
		})
	}
}

func TestLegacyMetadataStringKeepsOtherStrings(t *testing.T) {
	for _, s := range []string{`"hello"`, `"{\"n\": 3}"`, `"{not json"`} {
		var want string
		require.NoError(t, json.Unmarshal([]byte(s), &want))
		assert.Equal(t, want, legacyMetadataString(json.RawMessage(s)), s)
	}
}
