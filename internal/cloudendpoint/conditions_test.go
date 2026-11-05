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

package cloudendpoint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
)

func TestSetConditions(t *testing.T) {
	setters := []struct {
		name          string
		set           func(Resource, bool, string, string)
		conditionType string
	}{
		{"Ready", SetReadyCondition, ConditionCloudEndpointReady},
		{"Created", SetCreatedCondition, ConditionCloudEndpointCreated},
	}
	for kind := range kinds(spec{}) {
		for _, s := range setters {
			t.Run(kind+"/"+s.name, func(t *testing.T) {
				r := kinds(spec{})[kind]

				s.set(r, true, "Good", "it worked")
				c := meta.FindStatusCondition(*r.GetConditions(), s.conditionType)
				require.NotNil(t, c)
				assert.Equal(t, metav1.ConditionTrue, c.Status)
				assert.Equal(t, "Good", c.Reason)
				assert.Equal(t, "it worked", c.Message)
				assert.Equal(t, int64(1), c.ObservedGeneration)

				s.set(r, false, "Bad", "it failed")
				c = meta.FindStatusCondition(*r.GetConditions(), s.conditionType)
				require.NotNil(t, c)
				assert.Equal(t, metav1.ConditionFalse, c.Status)
				assert.Equal(t, "Bad", c.Reason)
				assert.Equal(t, "it failed", c.Message)
			})
		}
	}
}

func TestCalculateReadyCondition(t *testing.T) {
	created := metav1.Condition{Type: ConditionCloudEndpointCreated, Status: metav1.ConditionTrue, Reason: ReasonCloudEndpointCreated}
	createFailed := metav1.Condition{Type: ConditionCloudEndpointCreated, Status: metav1.ConditionFalse, Reason: ReasonCloudEndpointCreationFailed, Message: "Failed to create CloudEndpoint"}
	tpFailed := metav1.Condition{Type: ConditionTrafficPolicy, Status: metav1.ConditionFalse, Reason: "TrafficPolicyNotFound", Message: "policy missing"}
	tpApplied := metav1.Condition{Type: ConditionTrafficPolicy, Status: metav1.ConditionTrue, Reason: "TrafficPolicyApplied"}
	domainReady := &domainpkg.DomainResult{IsReady: true, ReadyReason: "DomainActive", ReadyMessage: "Domain is ready"}
	domainProvisioning := &domainpkg.DomainResult{IsReady: false, ReadyReason: "ProvisioningError", ReadyMessage: "Certificate provisioning in progress"}

	tests := []struct {
		name        string
		conditions  []metav1.Condition
		domain      *domainpkg.DomainResult
		wantStatus  metav1.ConditionStatus
		wantReason  string
		wantMessage string
	}{
		{
			name:        "all ready",
			conditions:  []metav1.Condition{created},
			domain:      domainReady,
			wantStatus:  metav1.ConditionTrue,
			wantReason:  ReasonCloudEndpointActive,
			wantMessage: "CloudEndpoint is active and ready",
		},
		{
			name:        "all ready with traffic policy applied",
			conditions:  []metav1.Condition{created, tpApplied},
			domain:      domainReady,
			wantStatus:  metav1.ConditionTrue,
			wantReason:  ReasonCloudEndpointActive,
			wantMessage: "CloudEndpoint is active and ready",
		},
		{
			name:        "domain not ready uses domain reason",
			conditions:  []metav1.Condition{created},
			domain:      domainProvisioning,
			wantStatus:  metav1.ConditionFalse,
			wantReason:  "ProvisioningError",
			wantMessage: "Certificate provisioning in progress",
		},
		{
			name:        "domain not ready without reason",
			conditions:  []metav1.Condition{created},
			domain:      &domainpkg.DomainResult{IsReady: false},
			wantStatus:  metav1.ConditionFalse,
			wantReason:  ReasonDomainNotReady,
			wantMessage: "Domain is not ready",
		},
		{
			name:        "nil domain result is not ready",
			conditions:  []metav1.Condition{created},
			domain:      nil,
			wantStatus:  metav1.ConditionFalse,
			wantReason:  ReasonDomainNotReady,
			wantMessage: "Domain is not ready",
		},
		{
			name:        "creation failed",
			conditions:  []metav1.Condition{createFailed},
			domain:      domainReady,
			wantStatus:  metav1.ConditionFalse,
			wantReason:  ReasonCloudEndpointCreationFailed,
			wantMessage: "Failed to create CloudEndpoint",
		},
		{
			name:        "no created condition is pending",
			conditions:  nil,
			domain:      domainReady,
			wantStatus:  metav1.ConditionFalse,
			wantReason:  ReasonPending,
			wantMessage: "Waiting for CloudEndpoint to be ready",
		},
		{
			name:        "domain not ready takes precedence over creation failure",
			conditions:  []metav1.Condition{createFailed},
			domain:      domainProvisioning,
			wantStatus:  metav1.ConditionFalse,
			wantReason:  "ProvisioningError",
			wantMessage: "Certificate provisioning in progress",
		},
		{
			name:        "traffic policy failure takes precedence over pending",
			conditions:  []metav1.Condition{tpFailed},
			domain:      domainReady,
			wantStatus:  metav1.ConditionFalse,
			wantReason:  "TrafficPolicyNotFound",
			wantMessage: "policy missing",
		},
	}

	for _, tt := range tests {
		for kind, r := range kinds(spec{}) {
			t.Run(kind+"/"+tt.name, func(t *testing.T) {
				*r.GetConditions() = append([]metav1.Condition(nil), tt.conditions...)

				CalculateReadyCondition(r, tt.domain)

				c := meta.FindStatusCondition(*r.GetConditions(), ConditionCloudEndpointReady)
				require.NotNil(t, c)
				assert.Equal(t, tt.wantStatus, c.Status)
				assert.Equal(t, tt.wantReason, c.Reason)
				assert.Equal(t, tt.wantMessage, c.Message)
			})
		}
	}
}
