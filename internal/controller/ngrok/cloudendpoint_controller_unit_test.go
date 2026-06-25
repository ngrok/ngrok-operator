package ngrok

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ngrok/ngrok-api-go/v7"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller/labels"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
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

			r.normalizeLegacyTrafficPolicy(tt.clep)

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
