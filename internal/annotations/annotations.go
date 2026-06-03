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

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-operator/internal/annotations/parser"
	"github.com/ngrok/ngrok-operator/internal/deprecation"
	"github.com/ngrok/ngrok-operator/internal/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// deprecationCallback returns a parser.LegacyHitFunc that routes legacy-key
// hits to the deprecation helper, which structured-logs and (if recorder is
// non-nil) fires a Warning event.
func deprecationCallback(log logr.Logger, recorder deprecation.EventRecorder, obj client.Object) parser.LegacyHitFunc {
	return func(legacyKey, newKey string) {
		deprecation.Annotation(log, recorder, obj, legacyKey, newKey)
	}
}

// The canonical annotation key for each user-facing knob uses the new
// `ngrok.com/` prefix. The matching `Legacy*Annotation` const a few lines
// down is read-side compatibility for the migration window and is deleted in
// the cleanup PR — see `internal/deprecation` for the marker convention used
// to find every site.
//
// When adding a new user-facing annotation key, also update
// `deprecation.userFacingAnnotationSuffixes` so the reconcile path emits a
// LegacyAnnotation event when a user is still on the legacy prefix.
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
	MappingStrategyAnnotation    = "ngrok.com/mapping-strategy"
	MappingStrategyAnnotationKey = "mapping-strategy"

	EndpointPoolingAnnotation    = "ngrok.com/pooling-enabled"
	EndpointPoolingAnnotationKey = "pooling-enabled"

	TrafficPolicyAnnotation    = "ngrok.com/traffic-policy"
	TrafficPolicyAnnotationKey = "traffic-policy"

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

	// BindingsAnnotation/Key controls per-endpoint binding visibility (public/internal/kubernetes).
	BindingsAnnotation    = "ngrok.com/bindings"
	BindingsAnnotationKey = "bindings"
)

// LEGACY-PREFIX-MIGRATION: BEGIN
// Legacy `k8s.ngrok.com/`-prefixed keys retained for read-side compatibility
// during the ngrok.com prefix migration. Delete this entire const block in
// the release immediately before ngrok-operator 1.0.
// See docs/developer-guide/passivity-shims.md and internal/deprecation for the convention.
const (
	LegacyMappingStrategyAnnotation = "k8s.ngrok.com/mapping-strategy"
	LegacyEndpointPoolingAnnotation = "k8s.ngrok.com/pooling-enabled"
	LegacyTrafficPolicyAnnotation   = "k8s.ngrok.com/traffic-policy"
	LegacyURLAnnotation             = "k8s.ngrok.com/url"
	LegacyMetadataAnnotation        = "k8s.ngrok.com/metadata"
	LegacyDescriptionAnnotation     = "k8s.ngrok.com/description"
	LegacyBindingsAnnotation        = "k8s.ngrok.com/bindings"
)

// LEGACY-PREFIX-MIGRATION: END

type MappingStrategy string

const (
	// The default strategy when translating resources into AgentEndpoint / CloudEndpoint that prioritizes collapsing into a single public AgentEndpoint when possible
	MappingStrategy_EndpointsDefault MappingStrategy = "endpoints"

	// Alternative strategy when translating resources into AgentEndpoint / CloudEndpoint that always creates CloudEndpoints for hostnames and only internal AgentEndpoints for each unique upstream
	MappingStrategy_EndpointsVerbose MappingStrategy = "endpoints-verbose"
)

// ExtractNgrokTrafficPolicyFromAnnotations reads the traffic-policy annotation
// from obj. During the legacy-prefix migration window it also accepts
// `k8s.ngrok.com/traffic-policy` and emits a deprecation signal on legacy hits.
// recorder may be nil on translator hot paths.
func ExtractNgrokTrafficPolicyFromAnnotations(log logr.Logger, recorder deprecation.EventRecorder, obj client.Object) (string, error) {
	policies, err := parser.GetStringSliceAnnotationWithFallback(TrafficPolicyAnnotationKey, obj, deprecationCallback(log, recorder, obj))
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

// ExtractUseEndpointPooling reads the pooling-enabled annotation. Returns nil
// if unset, so callers can distinguish "unset" from "explicitly disabled".
// The legacy-prefix form is accepted during the migration window.
func ExtractUseEndpointPooling(log logr.Logger, recorder deprecation.EventRecorder, obj client.Object) (*bool, error) {
	val, err := parser.GetStringAnnotationWithFallback(EndpointPoolingAnnotationKey, obj, deprecationCallback(log, recorder, obj))
	if err != nil {
		if errors.IsMissingAnnotations(err) {
			return nil, nil
		}
		return nil, err
	}

	result := strings.EqualFold(val, "true")
	return &result, nil
}

// ExtractUseBindings reads the bindings annotation. The legacy-prefix form is
// accepted during the migration window.
func ExtractUseBindings(log logr.Logger, recorder deprecation.EventRecorder, obj client.Object) ([]string, error) {
	bindings, err := parser.GetStringSliceAnnotationWithFallback(BindingsAnnotationKey, obj, deprecationCallback(log, recorder, obj))
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

// ExtractURL reads the url annotation. The legacy-prefix form is accepted
// during the migration window.
func ExtractURL(log logr.Logger, recorder deprecation.EventRecorder, obj client.Object) (string, error) {
	return parser.GetStringAnnotationWithFallback(URLKey, obj, deprecationCallback(log, recorder, obj))
}

// ExtractComputedURL extracts the computed URL from the annotation "k8s.ngrok.com/computed-url" if it is present. Otherwise, it returns
// an error.
func ExtractComputedURL(obj client.Object) (string, error) {
	return parser.GetStringAnnotation(ComputedURLKey, obj)
}

// ExtractMetadata reads the metadata annotation. Returns ("", nil) if unset.
// The legacy-prefix form is accepted during the migration window.
func ExtractMetadata(log logr.Logger, recorder deprecation.EventRecorder, obj client.Object) (string, error) {
	val, err := parser.GetStringAnnotationWithFallback(MetadataKey, obj, deprecationCallback(log, recorder, obj))
	if err != nil {
		if errors.IsMissingAnnotations(err) {
			return "", nil
		}
		return "", err
	}
	return val, nil
}

// ExtractDescription reads the description annotation. Returns ("", nil) if
// unset. The legacy-prefix form is accepted during the migration window.
func ExtractDescription(log logr.Logger, recorder deprecation.EventRecorder, obj client.Object) (string, error) {
	val, err := parser.GetStringAnnotationWithFallback(DescriptionKey, obj, deprecationCallback(log, recorder, obj))
	if err != nil {
		if errors.IsMissingAnnotations(err) {
			return "", nil
		}
		return "", err
	}
	return val, nil
}
