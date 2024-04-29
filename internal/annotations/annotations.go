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

	"github.com/imdario/mergo"
	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/ingress/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/compression"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/headers"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/ip_policies"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/parser"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/tls"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/webhook_verification"
	"github.com/ngrok/kubernetes-ingress-controller/internal/errors"
	networking "k8s.io/api/networking/v1"
	"k8s.io/klog/v2"
)

// DeniedKeyName name of the key that contains the reason to deny a location
const DeniedKeyName = "Denied"

type RouteModules struct {
	Compression         *ingressv1alpha1.EndpointCompression
	Headers             *ingressv1alpha1.EndpointHeaders
	IPRestriction       *ingressv1alpha1.EndpointIPPolicy
	TLSTermination      *ingressv1alpha1.EndpointTLSTerminationAtEdge
	WebhookVerification *ingressv1alpha1.EndpointWebhookVerification
}

type Extractor struct {
	annotations map[string]parser.IngressAnnotation
}

func NewAnnotationsExtractor() Extractor {
	return Extractor{
		annotations: map[string]parser.IngressAnnotation{
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
func ExtractNgrokModuleSetsFromAnnotations(ing *networking.Ingress) ([]string, error) {
	return parser.GetStringSliceAnnotation("modules", ing)
}

// Extracts a single traffic policy str from the annotation
// k8s.ngrok.com/traffic-policy: "module1"
func ExtractNgrokTrafficPolicyFromAnnotations(ing *networking.Ingress) (string, error) {
	policies, err := parser.GetStringSliceAnnotation("traffic-policy", ing)

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
