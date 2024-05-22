/*
Copyright 2015 The Kubernetes Authors.

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

package parser

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/ngrok/kubernetes-ingress-controller/internal/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DefaultAnnotationsPrefix defines the common prefix used in the nginx ingress controller
const DefaultAnnotationsPrefix = "k8s.ngrok.com"

var (
	// AnnotationsPrefix is the mutable attribute that the controller explicitly refers to
	AnnotationsPrefix = DefaultAnnotationsPrefix
)

// Annotation has a method to parse annotations located in client.Object
type Annotation interface {
	Parse(obj client.Object) (interface{}, error)
}

type annotations map[string]string

func (a annotations) parseBool(name string) (bool, error) {
	val, ok := a[name]
	if ok {
		b, err := strconv.ParseBool(val)
		if err != nil {
			return false, errors.NewInvalidAnnotationContent(name, val)
		}
		return b, nil
	}
	return false, errors.ErrMissingAnnotations
}

func (a annotations) parseString(name string) (string, error) {
	val, ok := a[name]
	if ok {
		s := normalizeString(val)
		if len(s) == 0 {
			return "", errors.NewInvalidAnnotationContent(name, val)
		}

		return s, nil
	}
	return "", errors.ErrMissingAnnotations
}

func (a annotations) parseStringSlice(name string) ([]string, error) {
	val, ok := a[name]
	if ok {
		s := normalizeString(val)
		if len(s) == 0 {
			return []string{}, errors.NewInvalidAnnotationContent(name, val)
		}

		// Remove spaces around each element
		values := []string{}
		for _, v := range strings.Split(s, ",") {
			values = append(values, strings.TrimSpace(v))
		}

		return values, nil
	}
	return []string{}, errors.ErrMissingAnnotations
}

func (a annotations) parseStringMap(name string) (map[string]string, error) {
	val, ok := a[name]
	if !ok {
		return nil, errors.ErrMissingAnnotations
	}

	m := map[string]string{}
	err := json.Unmarshal([]byte(val), &m)
	if err != nil {
		return nil, errors.NewInvalidAnnotationContent(name, val)
	}

	return m, nil
}

func (a annotations) parseInt(name string) (int, error) {
	val, ok := a[name]
	if ok {
		i, err := strconv.Atoi(val)
		if err != nil {
			return 0, errors.NewInvalidAnnotationContent(name, val)
		}
		return i, nil
	}
	return 0, errors.ErrMissingAnnotations
}

func (a annotations) parseFloat32(name string) (float32, error) {
	val, ok := a[name]
	if ok {
		i, err := strconv.ParseFloat(val, 32)
		if err != nil {
			return 0, errors.NewInvalidAnnotationContent(name, val)
		}
		return float32(i), nil
	}
	return 0, errors.ErrMissingAnnotations
}

func checkAnnotation(name string, obj client.Object) error {
	if obj == nil || len(obj.GetAnnotations()) == 0 {
		return errors.ErrMissingAnnotations
	}
	if name == "" {
		return errors.ErrInvalidAnnotationName
	}

	return nil
}

// GetBoolAnnotation extracts a boolean from a client.Object annotation
func GetBoolAnnotation(name string, obj client.Object) (bool, error) {
	v := GetAnnotationWithPrefix(name)
	err := checkAnnotation(v, obj)
	if err != nil {
		return false, err
	}
	return annotations(obj.GetAnnotations()).parseBool(v)
}

// GetStringAnnotation extracts a string from an client.Object annotation
func GetStringAnnotation(name string, obj client.Object) (string, error) {
	v := GetAnnotationWithPrefix(name)
	err := checkAnnotation(v, obj)
	if err != nil {
		return "", err
	}

	return annotations(obj.GetAnnotations()).parseString(v)
}

func GetStringSliceAnnotation(name string, obj client.Object) ([]string, error) {
	v := GetAnnotationWithPrefix(name)
	err := checkAnnotation(v, obj)
	if err != nil {
		return []string{}, err
	}

	return annotations(obj.GetAnnotations()).parseStringSlice(v)
}

func GetStringMapAnnotation(name string, obj client.Object) (map[string]string, error) {
	v := GetAnnotationWithPrefix(name)
	err := checkAnnotation(v, obj)
	if err != nil {
		return nil, err
	}

	return annotations(obj.GetAnnotations()).parseStringMap(v)
}

// GetIntAnnotation extracts an int from a client.Object annotation
func GetIntAnnotation(name string, obj client.Object) (int, error) {
	v := GetAnnotationWithPrefix(name)
	err := checkAnnotation(v, obj)
	if err != nil {
		return 0, err
	}
	return annotations(obj.GetAnnotations()).parseInt(v)
}

// GetFloatAnnotation extracts a float32 from a client.Object annotation
func GetFloatAnnotation(name string, obj client.Object) (float32, error) {
	v := GetAnnotationWithPrefix(name)
	err := checkAnnotation(v, obj)
	if err != nil {
		return 0, err
	}
	return annotations(obj.GetAnnotations()).parseFloat32(v)
}

// GetAnnotationWithPrefix returns the annotation prefixed with the AnnotationsPrefix
func GetAnnotationWithPrefix(suffix string) string {
	return fmt.Sprintf("%v/%v", AnnotationsPrefix, suffix)
}

func normalizeString(input string) string {
	trimmedContent := []string{}
	for _, line := range strings.Split(input, "\n") {
		trimmedContent = append(trimmedContent, strings.TrimSpace(line))
	}

	return strings.Join(trimmedContent, "\n")
}

var configmapAnnotations = sets.NewString(
	"auth-proxy-set-header",
	"fastcgi-params-configmap",
)

// AnnotationsReferencesConfigmap checks if at least one annotation in the Ingress rule
// references a configmap.
func AnnotationsReferencesConfigmap(obj client.Object) bool {
	if obj == nil || len(obj.GetAnnotations()) == 0 {
		return false
	}

	for name := range obj.GetAnnotations() {
		if configmapAnnotations.Has(name) {
			return true
		}
	}

	return false
}

// StringToURL parses the provided string into URL and returns error
// message in case of failure
func StringToURL(input string) (*url.URL, error) {
	parsedURL, err := url.Parse(input)
	if err != nil {
		return nil, fmt.Errorf("%v is not a valid URL: %v", input, err)
	}

	if parsedURL.Scheme == "" {
		return nil, fmt.Errorf("url scheme is empty")
	} else if parsedURL.Host == "" {
		return nil, fmt.Errorf("url host is empty")
	} else if strings.Contains(parsedURL.Host, "..") {
		return nil, fmt.Errorf("invalid url host")
	}

	return parsedURL, nil
}
