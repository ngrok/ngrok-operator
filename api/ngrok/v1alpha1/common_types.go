/*
MIT License

Copyright (c) 2024 ngrok, Inc.

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

package v1alpha1

import (
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
)

// The shared endpoint types moved to ngrok.com/v1. These aliases keep the
// v1alpha1 names, so both groups use one Go type and one schema.
//
// LEGACY-agentendpoint-kind: delete with the last v1alpha1 endpoint kind.
type (
	K8sObjectRef                  = ngrokv1.K8sObjectRef
	K8sObjectRefOptionalNamespace = ngrokv1.K8sObjectRefOptionalNamespace
	TrafficPolicyCfgType          = ngrokv1.TrafficPolicyCfgType
	TrafficPolicyCfg              = ngrokv1.TrafficPolicyCfg
	EndpointWithDomain            = ngrokv1.EndpointWithDomain
	EndpointWithTrafficPolicy     = ngrokv1.EndpointWithTrafficPolicy
)

const (
	TrafficPolicyCfgType_K8sRef = ngrokv1.TrafficPolicyCfgType_K8sRef
	TrafficPolicyCfgType_Inline = ngrokv1.TrafficPolicyCfgType_Inline
)
