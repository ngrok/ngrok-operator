package ngrok

// LEGACY-cloudendpoint-kind: delete this whole file at cleanup. The twin
// behavior is covered by the "CloudEndpoint migration" envtest specs; this
// covers the decision table, including the read errors envtest cannot produce.

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/cloudendpoint"
	"github.com/ngrok/ngrok-operator/internal/controller/labels"
)

func TestCloudEndpointTwinDecision(t *testing.T) {
	const id = "ep_123"
	gr := schema.GroupResource{Group: "ngrok.com", Resource: "cloudendpoints"}

	v1Twin := func(id string, deleting bool) *ngrokv1.CloudEndpoint {
		ep := &ngrokv1.CloudEndpoint{}
		ep.Status.ID = id
		if deleting {
			ep.DeletionTimestamp = new(metav1.Now())
		}
		return ep
	}
	legacyTwin := func(id string) *ngrokv1alpha1.CloudEndpoint {
		ep := &ngrokv1alpha1.CloudEndpoint{}
		ep.Status.ID = id
		return ep
	}

	tests := []struct {
		name     string
		err      error
		twin     cloudendpoint.Resource
		draining bool
		want     cloudEndpointTwinAction
	}{
		{name: "NotFound", err: apierrors.NewNotFound(gr, "clep"), twin: &ngrokv1.CloudEndpoint{}, want: twinDeleteRemote},
		{name: "NoMatch", err: &meta.NoKindMatchError{GroupKind: schema.GroupKind{Group: "ngrok.com", Kind: "CloudEndpoint"}}, twin: &ngrokv1.CloudEndpoint{}, want: twinDeleteRemote},
		{name: "Forbidden", err: apierrors.NewForbidden(gr, "clep", errors.New("rbac")), twin: &ngrokv1.CloudEndpoint{}, want: twinKeepRemote},
		{name: "Other error", err: errors.New("boom"), twin: &ngrokv1.CloudEndpoint{}, want: twinRequeue},
		{name: "Live twin, same ID", twin: v1Twin(id, false), want: twinKeepRemote},
		{name: "Live legacy twin, same ID", twin: legacyTwin(id), want: twinKeepRemote},
		{name: "Live twin, different ID", twin: v1Twin("ep_other", false), want: twinDeleteRemote},
		{name: "Live legacy twin, different ID", twin: legacyTwin("ep_other"), want: twinDeleteRemote},
		{name: "Legacy deleting, v1 twin has no ID, not draining", twin: v1Twin("", false), want: twinWait},
		{name: "Legacy deleting, v1 twin has no ID, draining", twin: v1Twin("", false), draining: true, want: twinDeleteRemote},
		{name: "v1 deleting, legacy twin has no ID", twin: legacyTwin(""), want: twinDeleteRemote},
		{name: "Twin being deleted, same ID", twin: v1Twin(id, true), want: twinDeleteRemote},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, cloudEndpointTwinDecision(tt.err, tt.twin, id, tt.draining))
		})
	}
}

func TestIsOperatorManagedCloudEndpoint(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		owner  string
		want   bool
	}{
		{name: "user-created", want: false},
		{name: "controller-name label", labels: map[string]string{labels.ControllerName: "ngrok-operator"}, want: true},
		{name: "legacy controller-name label", labels: map[string]string{labels.LegacyControllerName: "ngrok-operator"}, want: true},
		{name: "owned by a Service", owner: "Service", want: true},
		{name: "owned by something else", owner: "Deployment", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ep := &ngrokv1alpha1.CloudEndpoint{}
			ep.Labels = tt.labels
			if tt.owner != "" {
				ep.OwnerReferences = []metav1.OwnerReference{{Kind: tt.owner, Name: "x", Controller: new(true)}}
			}
			assert.Equal(t, tt.want, isOperatorManagedCloudEndpoint(ep))
		})
	}
}
