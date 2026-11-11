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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ngrok/ngrok-operator/internal/controller/conditions"
	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
	trafficpolicypkg "github.com/ngrok/ngrok-operator/internal/trafficpolicy"
)

const (
	// condition types for CloudEndpoint
	ConditionCloudEndpointReady   = "Ready"
	ConditionCloudEndpointCreated = "CloudEndpointCreated"
	// ConditionTrafficPolicy is sourced from the shared trafficpolicy package
	// so both endpoint controllers report the same condition type.
	ConditionTrafficPolicy = trafficpolicypkg.ConditionTrafficPolicy

	// condition reasons for CloudEndpoint
	ReasonCloudEndpointActive         = "CloudEndpointActive"
	ReasonCloudEndpointCreated        = "CloudEndpointCreated"
	ReasonCloudEndpointCreationFailed = "CloudEndpointCreationFailed"
	ReasonDomainNotReady              = "DomainNotReady"
	ReasonPending                     = "Pending"
	ReasonUnknown                     = "Unknown"

	// ReasonSupersededByV1 marks a legacy ngrok.k8s.ngrok.com CloudEndpoint
	// whose ngrok.com/v1 twin now manages the remote endpoint.
	// LEGACY-cloudendpoint-kind: delete at cleanup.
	ReasonSupersededByV1 = "SupersededByV1"
)

// SetReadyCondition sets the Ready condition
func SetReadyCondition(r Resource, ready bool, reason, message string) {
	conditions.Set(r.GetConditions(), r.GetGeneration(), ConditionCloudEndpointReady, ready, reason, message)
}

// SetCreatedCondition sets the CloudEndpointCreated condition
func SetCreatedCondition(r Resource, created bool, reason, message string) {
	conditions.Set(r.GetConditions(), r.GetGeneration(), ConditionCloudEndpointCreated, created, reason, message)
}

// CalculateReadyCondition calculates the overall Ready condition based on the
// per-sub-component conditions (CloudEndpointCreated, TrafficPolicyApplied)
// and the domain status.
func CalculateReadyCondition(r Resource, domainResult *domainpkg.DomainResult) {
	// Check CloudEndpoint created condition
	cloudEndpointCreated := false
	createdCondition := meta.FindStatusCondition(*r.GetConditions(), ConditionCloudEndpointCreated)
	if createdCondition != nil && createdCondition.Status == metav1.ConditionTrue {
		cloudEndpointCreated = true
	}

	// TrafficPolicy condition is optional — only blocks Ready when it exists
	// and is False (matches the AgentEndpoint logic).
	trafficPolicyCondition := meta.FindStatusCondition(*r.GetConditions(), ConditionTrafficPolicy)
	trafficPolicyReady := true
	if trafficPolicyCondition != nil && trafficPolicyCondition.Status == metav1.ConditionFalse {
		trafficPolicyReady = false
	}

	// Check if domain is ready (default to false for safety)
	domainReady := false
	if domainResult != nil {
		domainReady = domainResult.IsReady
	}

	// Overall ready status — all required sub-conditions must be true
	ready := cloudEndpointCreated && trafficPolicyReady && domainReady

	// Determine reason and message based on state
	var reason, message string
	switch {
	case ready:
		reason = ReasonCloudEndpointActive
		message = "CloudEndpoint is active and ready"
	case !domainReady:
		// Use the domain's Ready condition reason/message for better context
		if domainResult != nil && domainResult.ReadyReason != "" {
			reason = domainResult.ReadyReason
			message = domainResult.ReadyMessage
		} else {
			reason = ReasonDomainNotReady
			message = "Domain is not ready"
		}
	case !trafficPolicyReady:
		// A failed traffic-policy resolution (e.g. TrafficPolicyNotFound) is the
		// actionable root cause and is surfaced before the generic "not yet
		// created" pending state, since policy resolution gates creation.
		reason = trafficPolicyCondition.Reason
		message = trafficPolicyCondition.Message
	case !cloudEndpointCreated:
		// If CloudEndpointCreated condition exists and is False, use its reason/message
		if createdCondition != nil && createdCondition.Status == metav1.ConditionFalse {
			reason = createdCondition.Reason
			message = createdCondition.Message
		} else {
			reason = ReasonPending
			message = "Waiting for CloudEndpoint to be ready"
		}
	default:
		reason = ReasonUnknown
		message = "CloudEndpoint is not ready"
	}

	SetReadyCondition(r, ready, reason, message)
}
