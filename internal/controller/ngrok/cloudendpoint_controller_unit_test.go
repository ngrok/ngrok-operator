package ngrok

import (
	"encoding/json"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v9"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/stretchr/testify/assert"
)

// TestIndexCloudEndpointTrafficPolicyRefs covers indexCloudEndpointTrafficPolicyRefs:
// only a canonical targetRef produces an index entry, since that's the only
// shape with an external ref to watch.
func TestIndexCloudEndpointTrafficPolicyRefs(t *testing.T) {
	tests := []struct {
		name string
		clep client.Object
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
				Namespace: "ns",
			},
			want: nil,
		},
		{
			name: "canonical targetRef indexes namespace/name in the endpoint's namespace",
			clep: &ngrokv1alpha1.CloudEndpoint{
				Namespace: "ns",
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					TrafficPolicy: &ngrokv1alpha1.TrafficPolicyCfg{
						Reference: &ngrokv1alpha1.K8sObjectRef{Name: "policy"},
					},
				},
			},
			want: []string{"ns/policy"},
		},
		{
			name: "ngrok.com/v1 CloudEndpoint targetRef indexes namespace/name",
			clep: &ngrokv1.CloudEndpoint{
				Namespace: "ns",
				Spec: ngrokv1.CloudEndpointSpec{
					TrafficPolicy: &ngrokv1.TrafficPolicyCfg{
						Reference: &ngrokv1.K8sObjectRef{Name: "policy"},
					},
				},
			},
			want: []string{"ns/policy"},
		},
		{
			name: "canonical inline returns nil (no external ref to watch)",
			clep: &ngrokv1alpha1.CloudEndpoint{
				Namespace: "ns",
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					TrafficPolicy: &ngrokv1alpha1.TrafficPolicyCfg{
						Inline: json.RawMessage(`{"on_http_request":[]}`),
					},
				},
			},
			want: nil,
		},
		{
			name: "empty trafficPolicy{} returns nil",
			clep: &ngrokv1alpha1.CloudEndpoint{
				Namespace: "ns",
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					TrafficPolicy: &ngrokv1alpha1.TrafficPolicyCfg{},
				},
			},
			want: nil,
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

// TestErrResultURLConflict covers the retry for a URL another endpoint still holds: it requeues on a
// short fixed delay instead of returning the error, which would back off further on each attempt.
func TestErrResultURLConflict(t *testing.T) {
	r := &CloudEndpointReconciler{Log: logr.Discard()}
	ep := &ngrokv1.CloudEndpoint{}
	for _, code := range []string{"ERR_NGROK_18016", "ERR_NGROK_18017"} {
		t.Run(code, func(t *testing.T) {
			res, err := r.errResult(ep, &ngrok.Error{StatusCode: 400, ErrorCode: code})
			assert.NoError(t, err)
			assert.Equal(t, 2*time.Second, res.RequeueAfter)
		})
	}
}
