package ngrok

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ngrok/ngrok-api-go/v7"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/controller/labels"
	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
	trafficpolicypkg "github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEndpointNeedsUpdate(t *testing.T) {
	baseEndpoint := func() *ngrok.Endpoint {
		return &ngrok.Endpoint{
			URL:            "https://example.ngrok.app",
			Description:    "Created by the ngrok-operator",
			Metadata:       `{"owned-by":"ngrok-operator"}`,
			TrafficPolicy:  `{"on_http_request":[]}`,
			Bindings:       []string{"public"},
			PoolingEnabled: false,
		}
	}

	baseSpec := func() ngrokv1alpha1.CloudEndpointSpec {
		return ngrokv1alpha1.CloudEndpointSpec{
			URL:            "https://example.ngrok.app",
			Description:    "Created by the ngrok-operator",
			Metadata:       `{"owned-by":"ngrok-operator"}`,
			Bindings:       []string{"public"},
			PoolingEnabled: new(false),
		}
	}

	basePolicy := `{"on_http_request":[]}`

	tests := []struct {
		name     string
		endpoint *ngrok.Endpoint
		spec     ngrokv1alpha1.CloudEndpointSpec
		policy   string
		want     bool
	}{
		{
			name:     "no update needed when everything matches",
			endpoint: baseEndpoint(),
			spec:     baseSpec(),
			policy:   basePolicy,
			want:     false,
		},
		{
			name:     "update needed when URL changes",
			endpoint: baseEndpoint(),
			spec: func() ngrokv1alpha1.CloudEndpointSpec {
				s := baseSpec()
				s.URL = "https://other.ngrok.app"
				return s
			}(),
			policy: basePolicy,
			want:   true,
		},
		{
			name:     "update needed when description changes",
			endpoint: baseEndpoint(),
			spec: func() ngrokv1alpha1.CloudEndpointSpec {
				s := baseSpec()
				s.Description = "new description"
				return s
			}(),
			policy: basePolicy,
			want:   true,
		},
		{
			name:     "update needed when metadata changes",
			endpoint: baseEndpoint(),
			spec: func() ngrokv1alpha1.CloudEndpointSpec {
				s := baseSpec()
				s.Metadata = `{"owned-by":"someone-else"}`
				return s
			}(),
			policy: basePolicy,
			want:   true,
		},
		{
			name:     "update needed when traffic policy changes",
			endpoint: baseEndpoint(),
			spec:     baseSpec(),
			policy:   `{"on_http_request":[{"actions":[]}]}`,
			want:     true,
		},
		{
			name:     "update needed when bindings change",
			endpoint: baseEndpoint(),
			spec: func() ngrokv1alpha1.CloudEndpointSpec {
				s := baseSpec()
				s.Bindings = []string{"internal"}
				return s
			}(),
			policy: basePolicy,
			want:   true,
		},
		{
			name:     "update needed when pooling enabled changes",
			endpoint: baseEndpoint(),
			spec: func() ngrokv1alpha1.CloudEndpointSpec {
				s := baseSpec()
				s.PoolingEnabled = new(true)
				return s
			}(),
			policy: basePolicy,
			want:   true,
		},
		{
			name:     "nil pooling enabled in spec skips comparison",
			endpoint: baseEndpoint(),
			spec: func() ngrokv1alpha1.CloudEndpointSpec {
				s := baseSpec()
				s.PoolingEnabled = nil
				return s
			}(),
			policy: basePolicy,
			want:   false,
		},
		{
			name: "nil and empty bindings are equal",
			endpoint: func() *ngrok.Endpoint {
				e := baseEndpoint()
				e.Bindings = nil
				return e
			}(),
			spec: func() ngrokv1alpha1.CloudEndpointSpec {
				s := baseSpec()
				s.Bindings = nil
				return s
			}(),
			policy: basePolicy,
			want:   false,
		},
		{
			name: "empty slice and nil bindings are equal",
			endpoint: func() *ngrok.Endpoint {
				e := baseEndpoint()
				e.Bindings = []string{}
				return e
			}(),
			spec: func() ngrokv1alpha1.CloudEndpointSpec {
				s := baseSpec()
				s.Bindings = nil
				return s
			}(),
			policy: basePolicy,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := endpointNeedsUpdate(tt.endpoint, tt.spec, tt.policy)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestIndexCloudEndpointTrafficPolicyRefs covers the legacy-vs-canonical
// fallback gating. The key bug to guard against: when a user has both
// `spec.trafficPolicy.inline` (or `.policy`) AND `spec.trafficPolicyName`
// set, the canonical inline wins and the legacy name field must NOT
// produce an index entry — otherwise updates to a TrafficPolicy named
// `trafficPolicyName` would stale-requeue this endpoint that never
// resolves the named ref.
func TestIndexCloudEndpointTrafficPolicyRefs(t *testing.T) {
	tests := []struct {
		name string
		clep *ngrokv1alpha1.CloudEndpoint
		want []string
	}{
		{
			name: "non-CloudEndpoint object returns nil",
			clep: nil,
			want: nil,
		},
		{
			name: "no policy configured returns nil",
			clep: &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
			},
			want: nil,
		},
		{
			name: "canonical targetRef indexes namespace/name in the endpoint's namespace",
			clep: &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					TrafficPolicy: &ngrokv1alpha1.CloudEndpointTrafficPolicyCfg{
						Reference: &ngrokv1alpha1.K8sObjectRef{Name: "policy"},
					},
				},
			},
			want: []string{"ns/policy"},
		},
		{
			name: "canonical inline returns nil (no external ref to watch)",
			clep: &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					TrafficPolicy: &ngrokv1alpha1.CloudEndpointTrafficPolicyCfg{
						Inline: json.RawMessage(`{"on_http_request":[]}`),
					},
				},
			},
			want: nil,
		},
		{
			name: "canonical inline + legacy trafficPolicyName returns nil — canonical wins, do not index legacy",
			clep: &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					TrafficPolicyName: "legacy-ignored", //nolint:staticcheck // test of deprecated field
					TrafficPolicy: &ngrokv1alpha1.CloudEndpointTrafficPolicyCfg{
						Inline: json.RawMessage(`{"on_http_request":[]}`),
					},
				},
			},
			want: nil,
		},
		{
			name: "legacy nested policy + legacy trafficPolicyName returns nil — canonical-equivalent wins",
			clep: &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					TrafficPolicyName: "legacy-ignored", //nolint:staticcheck // test of deprecated field
					TrafficPolicy: &ngrokv1alpha1.CloudEndpointTrafficPolicyCfg{
						Policy: json.RawMessage(`{"on_http_request":[]}`), //nolint:staticcheck // test of deprecated field
					},
				},
			},
			want: nil,
		},
		{
			name: "canonical targetRef + legacy trafficPolicyName indexes only the canonical ref",
			clep: &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					TrafficPolicyName: "legacy-ignored", //nolint:staticcheck // test of deprecated field
					TrafficPolicy: &ngrokv1alpha1.CloudEndpointTrafficPolicyCfg{
						Reference: &ngrokv1alpha1.K8sObjectRef{Name: "canonical"},
					},
				},
			},
			want: []string{"ns/canonical"},
		},
		{
			name: "empty trafficPolicy{} alongside legacy trafficPolicyName falls back to legacy",
			clep: &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					TrafficPolicyName: "legacy-name", //nolint:staticcheck // test of deprecated field
					TrafficPolicy:     &ngrokv1alpha1.CloudEndpointTrafficPolicyCfg{},
				},
			},
			want: []string{"ns/legacy-name"},
		},
		{
			name: "legacy trafficPolicyName only (no canonical) indexes legacy",
			clep: &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					TrafficPolicyName: "legacy-name", //nolint:staticcheck // test of deprecated field
				},
			},
			want: []string{"ns/legacy-name"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.clep == nil {
				assert.Nil(t, indexCloudEndpointTrafficPolicyRefs(&ngrokv1alpha1.NgrokTrafficPolicy{}))
				return
			}
			got := indexCloudEndpointTrafficPolicyRefs(tt.clep)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestNormalizeLegacyTrafficPolicy_EventSuppression covers the
// DeprecatedField event suppression on operator-managed CloudEndpoints.
// The dual-write at translator.go / service controller.go deliberately
// triggers `HasDeprecatedPolicy()` on every reconcile, so suppression is
// load-bearing.
func TestNormalizeLegacyTrafficPolicy_EventSuppression(t *testing.T) {
	const (
		operatorNamespace = "ngrok-operator-system"
		operatorName      = "ngrok-operator"
	)

	tests := []struct {
		name        string
		clep        *ngrokv1alpha1.CloudEndpoint
		expectEvent bool
	}{
		{
			name: "operator-managed via controller OwnerReference (Service path) — suppressed",
			clep: &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "service-owned",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "v1",
							Kind:       "Service",
							Name:       "my-service",
							UID:        "uid-123",
							Controller: new(true),
						},
					},
				},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL: "https://service-owned.internal",
					TrafficPolicy: &ngrokv1alpha1.CloudEndpointTrafficPolicyCfg{
						Inline: json.RawMessage(`{"on_http_request":[]}`),
						Policy: json.RawMessage(`{"on_http_request":[]}`), //nolint:staticcheck // dual-write
					},
				},
			},
			expectEvent: false,
		},
		{
			name: "operator-managed via controller labels (managerdriver path) — suppressed",
			clep: &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "managerdriver-owned",
					Labels: map[string]string{
						labels.ControllerName:      operatorName,
						labels.ControllerNamespace: operatorNamespace,
					},
				},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL: "https://managerdriver-owned.internal",
					TrafficPolicy: &ngrokv1alpha1.CloudEndpointTrafficPolicyCfg{
						Inline: json.RawMessage(`{"on_http_request":[]}`),
						Policy: json.RawMessage(`{"on_http_request":[]}`), //nolint:staticcheck // dual-write
					},
				},
			},
			expectEvent: false,
		},
		{
			name: "user-authored (no controller ref, no operator labels) — event emitted",
			clep: &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "user-authored",
				},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL: "https://user-authored.internal",
					TrafficPolicy: &ngrokv1alpha1.CloudEndpointTrafficPolicyCfg{
						Inline: json.RawMessage(`{"on_http_request":[]}`),
						Policy: json.RawMessage(`{"on_http_request":[]}`), //nolint:staticcheck // intentional user dual-set
					},
				},
			},
			expectEvent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := events.NewFakeRecorder(10)
			r := &CloudEndpointReconciler{Recorder: recorder}

			r.normalizeLegacyTrafficPolicy(tt.clep, true)

			gotEvent := false
			for {
				select {
				case ev := <-recorder.Events:
					if strings.Contains(ev, "DeprecatedField") {
						gotEvent = true
					}
				default:
					if tt.expectEvent {
						assert.True(t, gotEvent, "expected a DeprecatedField event to be emitted")
					} else {
						assert.False(t, gotEvent, "DeprecatedField event should be suppressed on operator-managed CloudEndpoints")
					}
					return
				}
			}
		})
	}
}

// TestUpdate_RecreateOn404_DoesNotDoubleNormalizeLegacyPolicy is a regression
// test for the update() -> Get() 404 -> create() fallback. That fallback
// used to call r.create(ctx, clep), which re-ran resolveTrafficPolicy (and
// therefore normalizeLegacyTrafficPolicy) a second time on the very same
// object in the same reconcile — a wasted extra TrafficPolicy CR fetch plus
// a duplicate "DeprecatedField" event on every recreate. (The second call
// does *not* misresolve the policy: the ReconcileStatus call the fallback
// makes first, to clear the stale Status.ID before recreating, resets
// clep's in-memory Spec back to the API server's stored copy, so the second
// normalizeLegacyTrafficPolicy call sees the same legacy-only shape as the
// first and just redundantly repeats it — confirmed empirically.)
//
// createWithPolicy now lets the fallback reuse the policy update() already
// resolved instead of re-resolving, so resolveTrafficPolicy — and the
// DeprecatedField event it can emit — must run exactly once per reconcile.
// A real k8s Event API can't distinguish "emitted once" from "emitted twice
// with identical text" (it aggregates repeated (reason, regarding) events
// within a short window into a single object), so this exercises update()
// directly with a FakeRecorder instead of going through envtest.
//
// This also regression-tests a second bug in the same path: the
// ReconcileStatus call used to clear the stale Status.ID decodes the API
// server's un-normalized Spec back into clep, so by the time
// recordWriteSuccess calls MarkApplied, clep.GetTrafficPolicyCfg() saw nil
// for a trafficPolicyName-only endpoint and silently skipped setting
// TrafficPolicyApplied=True — even though the policy was correctly applied
// to the real ngrok endpoint. Fixed by re-normalizing (without re-emitting
// the event) right after the reset.
func TestUpdate_RecreateOn404_DoesNotDoubleNormalizeLegacyPolicy(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))

	trafficPolicy := &ngrokv1alpha1.NgrokTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "my-policy", Namespace: "ns"},
		Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
			Policy: json.RawMessage(`{"on_http_request":[{"name":"legacy"}]}`),
		},
	}
	clep := &ngrokv1alpha1.CloudEndpoint{
		ObjectMeta: metav1.ObjectMeta{Name: "legacy-only", Namespace: "ns"},
		Spec: ngrokv1alpha1.CloudEndpointSpec{
			URL:               "https://legacy-only.internal",
			TrafficPolicyName: "my-policy",
		},
		// A Status.ID the mock ngrok clientset doesn't know about, so
		// Endpoints().Get returns 404 and update() falls through to create.
		Status: ngrokv1alpha1.CloudEndpointStatus{ID: "ep_does_not_exist"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(trafficPolicy, clep).
		WithStatusSubresource(&ngrokv1alpha1.CloudEndpoint{}).
		Build()

	recorder := events.NewFakeRecorder(50)
	dm, err := domainpkg.NewManager(fakeClient, recorder)
	require.NoError(t, err)

	r := &CloudEndpointReconciler{
		Client:               fakeClient,
		Recorder:             recorder,
		NgrokClientset:       nmockapi.NewClientset(),
		DomainManager:        dm,
		TrafficPolicyManager: trafficpolicypkg.NewManager(fakeClient, recorder),
	}
	r.controller = &controller.BaseController[*ngrokv1alpha1.CloudEndpoint]{
		Kube:     fakeClient,
		Recorder: recorder,
	}

	require.NoError(t, r.update(context.Background(), clep))

	cond := meta.FindStatusCondition(clep.Status.Conditions, trafficpolicypkg.ConditionTrafficPolicy)
	require.NotNil(t, cond, "TrafficPolicyApplied condition must be set after the 404-recreate path, even for a trafficPolicyName-only endpoint")
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, trafficpolicypkg.ReasonTrafficPolicyApplied, cond.Reason)

	close(recorder.Events)
	var deprecatedEvents []string
	for ev := range recorder.Events {
		if strings.Contains(ev, "DeprecatedField") {
			deprecatedEvents = append(deprecatedEvents, ev)
		}
	}

	assert.Len(t, deprecatedEvents, 1,
		"resolveTrafficPolicy must run exactly once per reconcile on the 404-recreate fallback, got events: %v", deprecatedEvents)
}
