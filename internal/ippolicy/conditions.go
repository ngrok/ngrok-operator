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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ngrok/ngrok-operator/internal/controller/conditions"
)

const (
	// condition types for IPPolicy
	ConditionIPPolicyReady           = "Ready"
	ConditionIPPolicyCreated         = "IPPolicyCreated"
	ConditionIPPolicyRulesConfigured = "IPPolicyRulesConfigured"

	// condition reasons for IPPolicy
	ReasonIPPolicyActive                  = "IPPolicyActive"
	ReasonIPPolicyCreated                 = "IPPolicyCreated"
	ReasonIPPolicyRulesConfigured         = "IPPolicyRulesConfigured"
	ReasonIPPolicyRulesConfigurationError = "IPPolicyRulesConfigurationError"
	ReasonIPPolicyInvalidCIDR             = "IPPolicyInvalidCIDR"
	ReasonIPPolicyCreationFailed          = "IPPolicyCreationFailed"
)

// SetReadyCondition sets the Ready condition based on the overall IP policy state
func SetReadyCondition(p IPPolicyResource, ready bool, reason, message string) {
	conditions.Set(p.GetConditions(), p.GetGeneration(), ConditionIPPolicyReady, ready, reason, message)
}

// SetCreatedCondition sets the IPPolicyCreated condition
func SetCreatedCondition(p IPPolicyResource, created bool, reason, message string) {
	conditions.Set(p.GetConditions(), p.GetGeneration(), ConditionIPPolicyCreated, created, reason, message)
}

// SetRulesConfiguredCondition sets the RulesConfigured condition
func SetRulesConfiguredCondition(p IPPolicyResource, configured bool, reason, message string) {
	conditions.Set(p.GetConditions(), p.GetGeneration(), ConditionIPPolicyRulesConfigured, configured, reason, message)
}

// CalculateReadyCondition calculates the overall Ready condition based on other conditions
func CalculateReadyCondition(p IPPolicyResource) {
	// Check IP Policy created condition
	ipPolicyCreated := hasCreatedCondition(p)

	// Check rules configured condition
	ipPolicyRulesConfigured := false
	rulesConfiguredCondition := meta.FindStatusCondition(*p.GetConditions(), ConditionIPPolicyRulesConfigured)
	if rulesConfiguredCondition != nil && rulesConfiguredCondition.Status == metav1.ConditionTrue {
		ipPolicyRulesConfigured = true
	}

	switch {
	case ipPolicyCreated && ipPolicyRulesConfigured:
		SetReadyCondition(p, true, ReasonIPPolicyActive, "IP Policy is active")
	case ipPolicyCreated && !ipPolicyRulesConfigured:
		SetReadyCondition(p, false, ReasonIPPolicyRulesConfigurationError, "IP Policy rules are not configured")
	default:
		SetReadyCondition(p, false, ReasonIPPolicyCreationFailed, "IP Policy is not ready")
	}
}

func hasCreatedCondition(p IPPolicyResource) bool {
	if p == nil {
		return false
	}
	createdCondition := meta.FindStatusCondition(*p.GetConditions(), ConditionIPPolicyCreated)
	return createdCondition != nil && createdCondition.Status == metav1.ConditionTrue
}
