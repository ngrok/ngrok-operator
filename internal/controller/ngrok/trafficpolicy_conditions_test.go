package ngrok

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	trafficpolicypkg "github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	"github.com/ngrok/ngrok-operator/internal/util"
)

// Both instantiations of the generic reconciler must satisfy the
// controller-runtime contract. Asserting it here fails at compile time with a
// clear message rather than deep inside SetupWithManager.
var (
	_ reconcile.Reconciler = (*TrafficPolicyReconciler)(nil)
	// LEGACY-trafficpolicy-kind: drop this assertion at cleanup.
	_ reconcile.Reconciler = (*NgrokTrafficPolicyReconciler)(nil)
)

// trafficPolicyKind lets every test below run unchanged against each served
// kind. The condition logic is shared through TrafficPolicyResource, so
// exercising only one kind would leave the other untested — and, before these
// files were merged, the two kinds had separate copy-pasted tests that were
// free to drift apart.
type trafficPolicyKind struct {
	name string
	// new builds a policy resource of this kind at the given generation.
	new func(generation int64, policy string) trafficpolicypkg.TrafficPolicyResource
	// setPolicy rewrites spec.policy in place. spec is not part of the
	// interface (nothing in production needs to write it), so each kind
	// supplies its own setter.
	setPolicy func(tp trafficpolicypkg.TrafficPolicyResource, policy string)
}

var trafficPolicyKinds = []trafficPolicyKind{
	{
		name: "ngrok.com/v1 TrafficPolicy",
		new: func(generation int64, policy string) trafficpolicypkg.TrafficPolicyResource {
			return &ngrokv1.TrafficPolicy{
				Generation: generation,
				Spec:       ngrokv1.TrafficPolicySpec{Policy: json.RawMessage(policy)},
			}
		},
		setPolicy: func(tp trafficpolicypkg.TrafficPolicyResource, policy string) {
			tp.(*ngrokv1.TrafficPolicy).Spec.Policy = json.RawMessage(policy)
		},
	},
	{
		// LEGACY-trafficpolicy-kind: drop this entry at cleanup. The table
		// above keeps working with a single kind.
		name: "ngrok.k8s.ngrok.com/v1alpha1 NgrokTrafficPolicy",
		new: func(generation int64, policy string) trafficpolicypkg.TrafficPolicyResource {
			return &ngrokv1alpha1.NgrokTrafficPolicy{
				Generation: generation,
				Spec:       ngrokv1alpha1.NgrokTrafficPolicySpec{Policy: json.RawMessage(policy)},
			}
		},
		setPolicy: func(tp trafficpolicypkg.TrafficPolicyResource, policy string) {
			tp.(*ngrokv1alpha1.NgrokTrafficPolicy).Spec.Policy = json.RawMessage(policy)
		},
	},
}

func TestSetTrafficPolicyConditions(t *testing.T) {
	tests := []struct {
		name            string
		policy          string
		expectedStatus  metav1.ConditionStatus
		expectedReason  string
		messageContains string
	}{
		{
			name:            "valid policy",
			policy:          `{"on_http_request":[{"actions":[{"type":"deny"}]}]}`,
			expectedStatus:  metav1.ConditionTrue,
			expectedReason:  ReasonTrafficPolicyValid,
			messageContains: "valid",
		},
		{
			name:            "invalid policy JSON",
			policy:          `{"on_http_request":`,
			expectedStatus:  metav1.ConditionFalse,
			expectedReason:  ReasonTrafficPolicyParseFailed,
			messageContains: "Failed to parse",
		},
		{
			name:            "legacy directions",
			policy:          `{"inbound":[{"actions":[{"type":"deny"}]}]}`,
			expectedStatus:  metav1.ConditionTrue,
			expectedReason:  ReasonLegacyPolicyFormat,
			messageContains: "legacy directions",
		},
		{
			name:            "enabled field set",
			policy:          `{"enabled":true,"on_http_request":[{"actions":[{"type":"deny"}]}]}`,
			expectedStatus:  metav1.ConditionTrue,
			expectedReason:  ReasonEnabledDeprecated,
			messageContains: "'enabled' set",
		},
	}

	for _, kind := range trafficPolicyKinds {
		t.Run(kind.name, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					tp := kind.new(3, tt.policy)

					parsed, parseErr := util.NewTrafficPolicyFromJson(tp.GetPolicy())
					setTrafficPolicyConditions(tp, parsed, parseErr)

					for _, condType := range []string{ConditionTrafficPolicyReady, ConditionTrafficPolicyValid} {
						cond := meta.FindStatusCondition(*tp.GetConditions(), condType)
						require.NotNil(t, cond, "condition %s should be set", condType)
						assert.Equal(t, tt.expectedStatus, cond.Status)
						assert.Equal(t, tt.expectedReason, cond.Reason)
						assert.Contains(t, cond.Message, tt.messageContains)
						assert.Equal(t, int64(3), cond.ObservedGeneration)
					}
				})
			}
		})
	}
}

// TestStatusChangeDetection guards PolicyReconciler.Reconcile's
// skip-write-if-unchanged logic by driving the exact snapshot/compare pair the
// reconciler uses. meta.SetStatusCondition mutates an existing condition's
// fields in place, so a snapshot that shared the backing array would always
// compare equal to the post-mutation value and the reconciler would never
// persist a status update.
func TestStatusChangeDetection(t *testing.T) {
	const validPolicy = `{"on_http_request":[{"actions":[{"type":"deny"}]}]}`
	const legacyPolicy = `{"inbound":[{"actions":[{"type":"deny"}]}]}`

	// reconcileStatus mirrors the status half of Reconcile and reports
	// whether the change would have been written.
	reconcileStatus := func(tp trafficpolicypkg.TrafficPolicyResource) bool {
		prev := snapshotPolicyStatus(tp)
		parsed, parseErr := util.NewTrafficPolicyFromJson(tp.GetPolicy())
		setTrafficPolicyConditions(tp, parsed, parseErr)
		tp.SetObservedGeneration(tp.GetGeneration())
		return prev.differsFrom(tp)
	}

	for _, kind := range trafficPolicyKinds {
		t.Run(kind.name, func(t *testing.T) {
			tp := kind.new(1, validPolicy)

			// First reconcile: conditions don't exist yet, must report changed.
			assert.True(t, reconcileStatus(tp), "initial condition set must be detected as a change")

			// Second reconcile with identical inputs: no real change, must be a no-op.
			assert.False(t, reconcileStatus(tp), "unchanged inputs must not be reported as a change")

			// Third reconcile with a legacy policy: reason/message change while
			// Status stays True — must still be detected (this is the case a
			// LastTransitionTime-based check would miss).
			kind.setPolicy(tp, legacyPolicy)
			assert.True(t, reconcileStatus(tp), "reason/message change without a status flip must be detected")
			assert.Equal(t, metav1.ConditionTrue, meta.FindStatusCondition(*tp.GetConditions(), ConditionTrafficPolicyReady).Status,
				"Ready should still be true for a legacy-format policy")
		})
	}
}

// TestObservedGenerationChangeDetection covers the other half of the status
// snapshot: a bumped spec generation with byte-identical conditions must still
// be persisted, so status.observedGeneration catches up to metadata.generation.
func TestObservedGenerationChangeDetection(t *testing.T) {
	const validPolicy = `{"on_http_request":[{"actions":[{"type":"deny"}]}]}`

	for _, kind := range trafficPolicyKinds {
		t.Run(kind.name, func(t *testing.T) {
			tp := kind.new(1, validPolicy)

			parsed, parseErr := util.NewTrafficPolicyFromJson(tp.GetPolicy())
			setTrafficPolicyConditions(tp, parsed, parseErr)
			tp.SetObservedGeneration(tp.GetGeneration())

			// Nothing about the policy changed, only the generation counter.
			tp.SetGeneration(2)

			prev := snapshotPolicyStatus(tp)
			parsed, parseErr = util.NewTrafficPolicyFromJson(tp.GetPolicy())
			setTrafficPolicyConditions(tp, parsed, parseErr)
			tp.SetObservedGeneration(tp.GetGeneration())

			assert.True(t, prev.differsFrom(tp), "a generation bump must be persisted")
			assert.Equal(t, int64(2), tp.GetObservedGeneration())
		})
	}
}

// TestReconcilerControllerNamesAreDistinct guards the one hazard the shared
// generic reconciler introduces: controller-runtime derives a controller's
// name from the lowercased Kind of the object passed to For(), and registering
// two controllers under the same name makes SetupWithManager fail at startup.
// Because both reconcilers are now instantiations of the same generic type,
// nothing about their Go types keeps those names apart — only the distinct
// Kinds behind T do. This asserts that directly, via the same lookup the
// builder performs.
func TestReconcilerControllerNamesAreDistinct(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, ngrokv1.AddToScheme(scheme))
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))

	controllerName := func(obj client.Object) string {
		gvk, err := apiutil.GVKForObject(obj, scheme)
		require.NoError(t, err)
		return strings.ToLower(gvk.Kind)
	}

	canonical := controllerName(&ngrokv1.TrafficPolicy{})
	legacy := controllerName(&ngrokv1alpha1.NgrokTrafficPolicy{})

	assert.Equal(t, "trafficpolicy", canonical)
	assert.Equal(t, "ngroktrafficpolicy", legacy)
	assert.NotEqual(t, canonical, legacy, "both reconcilers would register under the same controller name")
}
