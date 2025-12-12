/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package annotations

import (
	"fmt"
	"strings"

	"github.com/ngrok/ngrok-operator/internal/annotations/parser"
	"github.com/ngrok/ngrok-operator/internal/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ComputedURLAnnotation is the annotation key for the computed URL of an endpoint.
	// This is temporarily used by the Service controller to store reserved TCP addresses,
	// while we work to add support for assigning TCP addresses to Cloud/Agent Endpoints
	// when their URL is specified as 'tcp://', for example.
	ComputedURLAnnotation = "k8s.ngrok.com/computed-url"
	ComputedURLKey        = "computed-url"

	// DeniedKeyName name of the key that contains the reason to deny a location
	DeniedKeyName = "Denied"

	// This annotation can be used on ingress/gateway resources to control which ngrok resources (endpoints/edges) get created from it
	MappingStrategyAnnotation    = "k8s.ngrok.com/mapping-strategy"
	MappingStrategyAnnotationKey = "mapping-strategy"

	EndpointPoolingAnnotation    = "k8s.ngrok.com/pooling-enabled"
	EndpointPoolingAnnotationKey = "pooling-enabled"

	TrafficPolicyAnnotation    = "k8s.ngrok.com/traffic-policy"
	TrafficPolicyAnnotationKey = "traffic-policy"

	// This annotation can be used on a service to control whether the endpoint is a TCP or TLS endpoint.
	// Examples:
	//   * tcp://1.tcp.ngrok.io:12345
	//   * tls://my-domain.com
	//
	URLAnnotation = "k8s.ngrok.com/url"
	URLKey        = "url"
)

type MappingStrategy string

const (
	// The default strategy when translating resources into AgentEndpoint / CloudEndpoint that prioritizes collapsing into a single public AgentEndpoint when possible
	MappingStrategy_EndpointsDefault MappingStrategy = "endpoints"

	// Alternative strategy when translating resources into AgentEndpoint / CloudEndpoint that always creates CloudEndpoints for hostnames and only internal AgentEndpoints for each unique upstream
	MappingStrategy_EndpointsVerbose MappingStrategy = "endpoints-verbose"
)

// Extracts a single traffic policy str from the annotation
// k8s.ngrok.com/traffic-policy: "module1"
func ExtractNgrokTrafficPolicyFromAnnotations(obj client.Object) (string, error) {
	policies, err := parser.GetStringSliceAnnotation(TrafficPolicyAnnotationKey, obj)

	if err != nil {
		return "", err
	}

	if len(policies) > 1 {
		return "", fmt.Errorf("multiple traffic policies are not supported: %v", policies)
	}

	if len(policies) != 0 {
		return policies[0], nil
	}

	return "", nil
}

// Whether or not we should use endpoint pooling
// from the annotation "k8s.ngrok.com/pooling-enabled" if it is present. Otherwise, it defaults to false
func ExtractUseEndpointPooling(obj client.Object) (bool, error) {
	val, err := parser.GetStringAnnotation(EndpointPoolingAnnotationKey, obj)
	if err != nil {
		if errors.IsMissingAnnotations(err) {
			return false, nil
		}
		return false, err
	}

	return strings.EqualFold(val, "true"), nil
}

// Determines which traffic is allowed to reach an endpoint
// from the annotation "k8s.ngrok.com/bindings" if it is present. Otherwise, it defaults to public
func ExtractUseBindings(obj client.Object) ([]string, error) {
	bindings, err := parser.GetStringSliceAnnotation("bindings", obj)
	if err != nil {
		if errors.IsMissingAnnotations(err) {
			return nil, nil
		}
		return nil, err
	}

	n := len(bindings)
	switch {
	case n > 1:
		return nil, fmt.Errorf("multiple bindings are not supported: %v", bindings)
	case n == 1:
		return bindings, nil
	default:
		return nil, nil
	}
}

// Retrieves the value of the annotation "k8s.ngrok.com/url" if it is present. Otherwise, it returns
// an error.
func ExtractURL(obj client.Object) (string, error) {
	return parser.GetStringAnnotation(URLKey, obj)
}

// ExtractComputedURL extracts the computed URL from the annotation "k8s.ngrok.com/computed-url" if it is present. Otherwise, it returns
// an error.
func ExtractComputedURL(obj client.Object) (string, error) {
	return parser.GetStringAnnotation(ComputedURLKey, obj)
}
