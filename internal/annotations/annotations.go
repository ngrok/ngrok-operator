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
	ComputedURLAnnotation = "ngrok.com/computed-url"

	// DeniedKeyName name of the key that contains the reason to deny a location
	DeniedKeyName = "Denied"

	// This annotation can be used on ingress/gateway resources to control which ngrok resources (endpoints/edges) get created from it
	MappingStrategyAnnotation    = "ngrok.com/mapping-strategy"
	MappingStrategyAnnotationKey = "mapping-strategy"

	EndpointPoolingAnnotation    = "ngrok.com/pooling-enabled"
	EndpointPoolingAnnotationKey = "pooling-enabled"

	TrafficPolicyAnnotation    = "ngrok.com/traffic-policy"
	TrafficPolicyAnnotationKey = "traffic-policy"

	// This annotation controls where the endpoint created from this resource is
	// bound (its visibility), e.g. public, internal, or kubernetes.
	BindingsAnnotation    = "ngrok.com/bindings"
	BindingsAnnotationKey = "bindings"

	// This annotation can be used on a service to control whether the endpoint is a TCP or TLS endpoint.
	// Examples:
	//   * tcp://1.tcp.ngrok.io:12345
	//   * tls://my-domain.com
	//
	URLAnnotation = "ngrok.com/url"
	URLKey        = "url"

	// MetadataAnnotation allows setting ngrok metadata on the endpoint created from this resource.
	// The value must be a JSON object string, e.g. '{"env":"prod","team":"platform"}'.
	// This metadata is merged with the operator-level default metadata; keys in this annotation take precedence.
	// When multiple annotated resources share the same endpoint, the metadata from the
	// alphabetically-first resource (by namespace/name) takes precedence per key.
	MetadataAnnotation = "ngrok.com/metadata"
	MetadataKey        = "metadata"

	// DescriptionAnnotation sets a human-readable description on the endpoint created from this resource.
	// When multiple resources share the same endpoint, the description from the alphabetically-first
	// resource (by namespace/name) is used; if none is set, the operator default is used.
	DescriptionAnnotation = "ngrok.com/description"
	DescriptionKey        = "description"
)

// LEGACY-PREFIX-MIGRATION: BEGIN
// LegacyComputedURLAnnotation is the deprecated key for the operator-written
// computed-url annotation. Retained so ExtractComputedURL can read values
// stamped by a previous operator version, and so setComputedURLAnnotation
// can dual-write during the migration window. Cleanup happens in two steps:
// write-side cleanup drops the dual-write in setComputedURLAnnotation; the
// later read-side cleanup drops this const and the legacy branch in
// ExtractComputedURL.
const LegacyComputedURLAnnotation = "k8s.ngrok.com/computed-url"

// LEGACY-PREFIX-MIGRATION: END

type MappingStrategy string

const (
	// The default strategy when translating resources into AgentEndpoint / CloudEndpoint that prioritizes collapsing into a single public AgentEndpoint when possible
	MappingStrategy_EndpointsDefault MappingStrategy = "endpoints"

	// Alternative strategy when translating resources into AgentEndpoint / CloudEndpoint that always creates CloudEndpoints for hostnames and only internal AgentEndpoints for each unique upstream
	MappingStrategy_EndpointsVerbose MappingStrategy = "endpoints-verbose"
)

// Extracts a single traffic policy str from the annotation
// ngrok.com/traffic-policy: "module1"
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
// from the annotation "ngrok.com/pooling-enabled" if it is present.
// Returns nil if the annotation is not set, allowing the caller to distinguish
// between "not configured" and "explicitly disabled".
func ExtractUseEndpointPooling(obj client.Object) (*bool, error) {
	val, err := parser.GetStringAnnotation(EndpointPoolingAnnotationKey, obj)
	if err != nil {
		if errors.IsMissingAnnotations(err) {
			return nil, nil
		}
		return nil, err
	}

	result := strings.EqualFold(val, "true")
	return &result, nil
}

// Determines which traffic is allowed to reach an endpoint
// from the annotation "ngrok.com/bindings" if it is present. Otherwise, it defaults to public
func ExtractUseBindings(obj client.Object) ([]string, error) {
	bindings, err := parser.GetStringSliceAnnotation(BindingsAnnotationKey, obj)
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

// Retrieves the value of the annotation "ngrok.com/url" if it is present. Otherwise, it returns
// an error.
func ExtractURL(obj client.Object) (string, error) {
	return parser.GetStringAnnotation(URLKey, obj)
}

// ExtractComputedURL reads the operator-written computed-url annotation.
// During the legacy-prefix migration window it dual-reads: it prefers the new
// `ngrok.com/computed-url` key and falls back to `k8s.ngrok.com/computed-url`
// for values stamped by a previous operator version. The Service controller
// re-stamps the resolved value under the new key on its next reconcile (both
// the TLS path and the TCP happy path call setComputedURLAnnotation), so a
// legacy-only value gets migrated to the new key.
//
// This reads the annotation map directly rather than going through the parser
// helpers. This predates the parser's dual-read support and stays direct-read
// only to keep the operator-written key's behavior self-contained here rather
// than depending on the parser's generic fallback semantics. Reading the map
// directly also treats a present-but-empty value as missing; that's safe
// here because the value is operator-written (never user-authored, never empty
// — clearComputedURLAnnotation deletes the key rather than emptying it).
//
// LEGACY-PREFIX-MIGRATION (read-side cleanup): drop the LegacyComputedURLAnnotation
// read. The body collapses back to a single annotation lookup.
func ExtractComputedURL(obj client.Object) (string, error) {
	if obj == nil {
		return "", errors.ErrMissingAnnotations
	}
	a := obj.GetAnnotations()
	if len(a) == 0 {
		return "", errors.ErrMissingAnnotations
	}
	if v, ok := a[ComputedURLAnnotation]; ok && v != "" {
		return v, nil
	}
	// LEGACY-PREFIX-MIGRATION (read-side cleanup): drop this branch
	if v, ok := a[LegacyComputedURLAnnotation]; ok && v != "" {
		return v, nil
	}
	return "", errors.ErrMissingAnnotations
}

// ExtractMetadata extracts the ngrok metadata JSON string from the annotation "ngrok.com/metadata".
// Returns ("", nil) if the annotation is not set.
func ExtractMetadata(obj client.Object) (string, error) {
	val, err := parser.GetStringAnnotation(MetadataKey, obj)
	if err != nil {
		if errors.IsMissingAnnotations(err) {
			return "", nil
		}
		return "", err
	}
	return val, nil
}

// ExtractDescription extracts the description string from the annotation "ngrok.com/description".
// Returns ("", nil) if the annotation is not set.
func ExtractDescription(obj client.Object) (string, error) {
	val, err := parser.GetStringAnnotation(DescriptionKey, obj)
	if err != nil {
		if errors.IsMissingAnnotations(err) {
			return "", nil
		}
		return "", err
	}
	return val, nil
}
