/*
MIT License

Copyright (c) 2026 ngrok, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package ippolicy

import (
	"encoding/json"
	"testing"

	"github.com/ngrok/ngrok-api-go/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
)

// syncerKinds builds one policy of every kind with the same description and
// rules, so each Syncer test runs against both.
func syncerKinds() map[string]IPPolicyResource {
	return map[string]IPPolicyResource{
		"canonical": &ngrokv1.IPPolicy{
			Name: "p", Namespace: "default", Generation: 1,
			Spec: ngrokv1.IPPolicySpec{
				Description: "desc",
				Rules: []ngrokv1.IPPolicyRule{
					{CIDR: "10.0.0.0/8", Action: "allow"},
					{CIDR: "192.168.1.0/24", Action: "deny"},
				},
			},
		},
		// LEGACY-ippolicy-kind: delete this entry at cleanup.
		"legacy": &ingressv1alpha1.IPPolicy{
			Name: "p", Namespace: "default", Generation: 1,
			Spec: ingressv1alpha1.IPPolicySpec{
				Description: "desc",
				Metadata:    json.RawMessage(`"free text"`),
				Rules: []ingressv1alpha1.IPPolicyRule{
					{CIDR: "10.0.0.0/8", Action: "allow"},
					{CIDR: "192.168.1.0/24", Action: "deny"},
				},
			},
		},
	}
}

func newTestSyncer() (*Syncer, *nmockapi.IPPolicyClient, *nmockapi.IPPolicyRuleClient) {
	policies := nmockapi.NewIPPolicyClient()
	rules := nmockapi.NewIPPolicyRuleClient(policies)
	return &Syncer{IPPolicies: policies, Rules: rules, Recorder: events.NewFakeRecorder(100)}, policies, rules
}

func conditionStatus(p IPPolicyResource, conditionType string) metav1.ConditionStatus {
	c := meta.FindStatusCondition(*p.GetConditions(), conditionType)
	if c == nil {
		return ""
	}
	return c.Status
}

func TestSyncer_CreatesPolicyAndRules(t *testing.T) {
	for kind, p := range syncerKinds() {
		t.Run(kind, func(t *testing.T) {
			s, policies, rules := newTestSyncer()

			require.NoError(t, s.Sync(t.Context(), p))

			require.NotEmpty(t, p.GetID())
			remote, err := policies.Get(t.Context(), p.GetID())
			require.NoError(t, err)
			assert.Equal(t, "desc", remote.Description)
			assert.Equal(t, p.GetMetadataAPIString(), remote.Metadata)
			assert.Len(t, rules.Items(), 2)
			assert.Equal(t, metav1.ConditionTrue, conditionStatus(p, ConditionIPPolicyReady))
		})
	}
}

func TestSyncer_UpdatesExistingPolicy(t *testing.T) {
	for kind, p := range syncerKinds() {
		t.Run(kind, func(t *testing.T) {
			s, policies, _ := newTestSyncer()
			require.NoError(t, s.Sync(t.Context(), p))
			id := p.GetID()

			switch obj := p.(type) {
			case *ngrokv1.IPPolicy:
				obj.Spec.Description = "changed"
			case *ingressv1alpha1.IPPolicy:
				obj.Spec.Description = "changed"
			}
			require.NoError(t, s.Sync(t.Context(), p))

			assert.Equal(t, id, p.GetID(), "an update must keep the remote ID")
			remote, err := policies.Get(t.Context(), id)
			require.NoError(t, err)
			assert.Equal(t, "changed", remote.Description)
		})
	}
}

func TestSyncer_ResetsStatusWhenRemoteIsGone(t *testing.T) {
	for kind, p := range syncerKinds() {
		t.Run(kind, func(t *testing.T) {
			s, policies, _ := newTestSyncer()
			require.NoError(t, s.Sync(t.Context(), p))
			policies.Reset()

			require.Error(t, s.Sync(t.Context(), p))
			assert.Empty(t, p.GetID(), "status must be reset so the next sync recreates the policy")
			assert.Empty(t, *p.GetConditions())
		})
	}
}

func TestSyncer_DeleteClearsIDAndToleratesMissingRemote(t *testing.T) {
	for kind, p := range syncerKinds() {
		t.Run(kind, func(t *testing.T) {
			s, policies, _ := newTestSyncer()
			require.NoError(t, s.Sync(t.Context(), p))
			id := p.GetID()

			require.NoError(t, s.Delete(t.Context(), p))
			assert.Empty(t, p.GetID())
			_, err := policies.Get(t.Context(), id)
			require.Error(t, err)

			p.SetID(id)
			err = s.Delete(t.Context(), p)
			assert.True(t, ngrok.IsNotFound(err), "callers treat NotFound as deleted")
			assert.Empty(t, p.GetID(), "an already-deleted remote still clears the ID")
		})
	}
}
