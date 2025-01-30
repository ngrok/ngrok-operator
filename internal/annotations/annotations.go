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

	"github.com/imdario/mergo"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/annotations/compression"
	"github.com/ngrok/ngrok-operator/internal/annotations/headers"
	"github.com/ngrok/ngrok-operator/internal/annotations/ip_policies"
	"github.com/ngrok/ngrok-operator/internal/annotations/parser"
	"github.com/ngrok/ngrok-operator/internal/annotations/tls"
	"github.com/ngrok/ngrok-operator/internal/annotations/webhook_verification"
	"github.com/ngrok/ngrok-operator/internal/errors"
	networking "k8s.io/api/networking/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// DeniedKeyName name of the key that contains the reason to deny a location
	DeniedKeyName = "Denied"

	// This annotation can be used on ingress/gateway resources to control which ngrok resources (endpoints/edges) get created from it
	MappingStrategyAnnotation    = "k8s.ngrok.com/mapping-strategy"
	MappingStrategyAnnotationKey = "mapping-strategy"
	MappingStrategy_Endpoints    = "endpoints"
	MappingStrategy_Edges        = "edges"

	EndpointPoolingAnnotation    = "k8s.ngrok.com/pooling-enabled"
	EndpointPoolingAnnotationKey = "pooling-enabled"
)

type RouteModules struct {
	Compression         *ingressv1alpha1.EndpointCompression
	Headers             *ingressv1alpha1.EndpointHeaders
	IPRestriction       *ingressv1alpha1.EndpointIPPolicy
	TLSTermination      *ingressv1alpha1.EndpointTLSTerminationAtEdge
	WebhookVerification *ingressv1alpha1.EndpointWebhookVerification
}

type Extractor struct {
	annotations map[string]parser.Annotation
}

func NewAnnotationsExtractor() Extractor {
	return Extractor{
		annotations: map[string]parser.Annotation{
			"Compression":         compression.NewParser(),
			"Headers":             headers.NewParser(),
			"IPRestriction":       ip_policies.NewParser(),
			"TLSTermination":      tls.NewParser(),
			"WebhookVerification": webhook_verification.NewParser(),
		},
	}
}

// Extract extracts the annotations from an Ingress
func (e Extractor) Extract(ing *networking.Ingress) *RouteModules {
	pia := &RouteModules{}

	data := make(map[string]interface{})
	for name, annotationParser := range e.annotations {
		val, err := annotationParser.Parse(ing)
		klog.V(5).InfoS("Parsing Ingress annotation", "name", name, "ingress", klog.KObj(ing), "value", val)
		if err != nil {
			if errors.IsMissingAnnotations(err) {
				continue
			}

			if !errors.IsLocationDenied(err) {
				continue
			}

			_, alreadyDenied := data[DeniedKeyName]
			if !alreadyDenied {
				errString := err.Error()
				data[DeniedKeyName] = &errString
				klog.ErrorS(err, "error reading Ingress annotation", "name", name, "ingress", klog.KObj(ing))
				continue
			}

			klog.V(5).ErrorS(err, "error reading Ingress annotation", "name", name, "ingress", klog.KObj(ing))
		}

		if val != nil {
			data[name] = val
		}
	}

	err := mergo.MapWithOverwrite(pia, data)
	if err != nil {
		klog.ErrorS(err, "unexpected error merging extracted annotations")
	}

	return pia
}

// Extracts a list of module set names from the annotation
// k8s.ngrok.com/modules: "module1,module2"
func ExtractNgrokModuleSetsFromAnnotations(obj client.Object) ([]string, error) {
	return parser.GetStringSliceAnnotation("modules", obj)
}

// Extracts a single traffic policy str from the annotation
// k8s.ngrok.com/traffic-policy: "module1"
func ExtractNgrokTrafficPolicyFromAnnotations(obj client.Object) (string, error) {
	policies, err := parser.GetStringSliceAnnotation("traffic-policy", obj)

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

// Whether or not we should use endpoints in building the ngrok model for resources. Extracts the value
// from the annotation "k8s.ngrok.com/mapping-strategy" if it is present. Otherwise, it defaults to false
func ExtractUseEndpoints(obj client.Object) (bool, error) {
	val, err := parser.GetStringAnnotation(MappingStrategyAnnotationKey, obj)
	if err != nil {
		if errors.IsMissingAnnotations(err) {
			return false, nil
		}
		return false, err
	}
	return strings.EqualFold(val, MappingStrategy_Endpoints), nil
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
