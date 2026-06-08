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

	"github.com/ngrok/ngrok-operator/internal/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CanonicalAnnotationsPrefix is the unified annotation prefix for ngrok-operator 1.0.
const CanonicalAnnotationsPrefix = "ngrok.com"

// LEGACY-PREFIX-MIGRATION: BEGIN
// Legacy `k8s.ngrok.com` annotation prefix and the mutable global it backs.
// The whole `*WithFallback` family further down also gets deleted in the
// cleanup PR; remove this block and that family together.
const LegacyAnnotationsPrefix = "k8s.ngrok.com"

var (
	// AnnotationsPrefix is the mutable attribute that the controller explicitly refers to.
	// During the migration window it defaults to the legacy prefix to keep
	// `Get*Annotation` (non-fallback) reading legacy-prefixed keys; the cleanup
	// PR replaces all uses with CanonicalAnnotationsPrefix.
	AnnotationsPrefix = LegacyAnnotationsPrefix
)

// LEGACY-PREFIX-MIGRATION: END

// Annotation has a method to parse annotations located in client.Object
type Annotation interface {
	Parse(obj client.Object) (any, error)
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
		for v := range strings.SplitSeq(s, ",") {
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

// LEGACY-PREFIX-MIGRATION: BEGIN
// Everything from here to the LEGACY-PREFIX-MIGRATION: END marker below is
// read-side compatibility scaffolding for the k8s.ngrok.com → ngrok.com prefix
// migration. In the cleanup PR:
//
//   - Delete the `*WithFallback` functions outright. Their callers should
//     migrate to the plain `Get*Annotation` variants further up, after those
//     are updated to read from CanonicalAnnotationsPrefix.
//   - Delete the LegacyHitFunc type and keysForFallback helper.
//
// See docs/developer-guide/passivity-shims.md and internal/deprecation for the
// marker convention used to find every site.

// LegacyHitFunc is invoked when a *WithFallback helper finds a value under the
// legacy k8s.ngrok.com prefix. legacyKey and newKey are the fully-qualified
// annotation keys (prefix + "/" + suffix).
type LegacyHitFunc func(legacyKey, newKey string)

// keysForFallback returns the canonical and legacy keys for the given suffix.
func keysForFallback(suffix string) (newKey, legacyKey string) {
	return CanonicalAnnotationsPrefix + "/" + suffix, LegacyAnnotationsPrefix + "/" + suffix
}

// GetStringAnnotationWithFallback reads suffix as a string annotation, trying
// the new prefix first then the legacy prefix. If the legacy prefix is used
// and onLegacyHit is non-nil, it is invoked with the legacy and new keys.
func GetStringAnnotationWithFallback(suffix string, obj client.Object, onLegacyHit LegacyHitFunc) (string, error) {
	newKey, legacyKey := keysForFallback(suffix)
	if err := checkAnnotation(newKey, obj); err == nil {
		if v, err := annotations(obj.GetAnnotations()).parseString(newKey); err == nil {
			return v, nil
		} else if !errors.IsMissingAnnotations(err) {
			return "", err
		}
	}
	if err := checkAnnotation(legacyKey, obj); err != nil {
		return "", err
	}
	v, err := annotations(obj.GetAnnotations()).parseString(legacyKey)
	if err != nil {
		return "", err
	}
	if onLegacyHit != nil {
		onLegacyHit(legacyKey, newKey)
	}
	return v, nil
}

// GetStringSliceAnnotationWithFallback reads suffix as a comma-separated slice,
// trying the new prefix first then the legacy prefix.
func GetStringSliceAnnotationWithFallback(suffix string, obj client.Object, onLegacyHit LegacyHitFunc) ([]string, error) {
	newKey, legacyKey := keysForFallback(suffix)
	if err := checkAnnotation(newKey, obj); err == nil {
		if v, err := annotations(obj.GetAnnotations()).parseStringSlice(newKey); err == nil {
			return v, nil
		} else if !errors.IsMissingAnnotations(err) {
			return []string{}, err
		}
	}
	if err := checkAnnotation(legacyKey, obj); err != nil {
		return []string{}, err
	}
	v, err := annotations(obj.GetAnnotations()).parseStringSlice(legacyKey)
	if err != nil {
		return []string{}, err
	}
	if onLegacyHit != nil {
		onLegacyHit(legacyKey, newKey)
	}
	return v, nil
}

// GetBoolAnnotationWithFallback reads suffix as a bool, trying the new prefix
// first then the legacy prefix.
//
// Intentional placeholder: no production caller yet — kept for symmetry with the
// other *WithFallback helpers as more annotations migrate onto the dual-prefix path.
func GetBoolAnnotationWithFallback(suffix string, obj client.Object, onLegacyHit LegacyHitFunc) (bool, error) {
	newKey, legacyKey := keysForFallback(suffix)
	if err := checkAnnotation(newKey, obj); err == nil {
		if v, err := annotations(obj.GetAnnotations()).parseBool(newKey); err == nil {
			return v, nil
		} else if !errors.IsMissingAnnotations(err) {
			return false, err
		}
	}
	if err := checkAnnotation(legacyKey, obj); err != nil {
		return false, err
	}
	v, err := annotations(obj.GetAnnotations()).parseBool(legacyKey)
	if err != nil {
		return false, err
	}
	if onLegacyHit != nil {
		onLegacyHit(legacyKey, newKey)
	}
	return v, nil
}

// GetStringMapAnnotationWithFallback reads suffix as a JSON-encoded map,
// trying the new prefix first then the legacy prefix.
//
// Intentional placeholder: no production caller yet — kept for symmetry with the
// other *WithFallback helpers as more annotations migrate onto the dual-prefix path.
func GetStringMapAnnotationWithFallback(suffix string, obj client.Object, onLegacyHit LegacyHitFunc) (map[string]string, error) {
	newKey, legacyKey := keysForFallback(suffix)
	if err := checkAnnotation(newKey, obj); err == nil {
		if v, err := annotations(obj.GetAnnotations()).parseStringMap(newKey); err == nil {
			return v, nil
		} else if !errors.IsMissingAnnotations(err) {
			return nil, err
		}
	}
	if err := checkAnnotation(legacyKey, obj); err != nil {
		return nil, err
	}
	v, err := annotations(obj.GetAnnotations()).parseStringMap(legacyKey)
	if err != nil {
		return nil, err
	}
	if onLegacyHit != nil {
		onLegacyHit(legacyKey, newKey)
	}
	return v, nil
}

// LEGACY-PREFIX-MIGRATION: END

func normalizeString(input string) string {
	trimmedContent := []string{}
	for line := range strings.SplitSeq(input, "\n") {
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

	switch {
	case parsedURL.Scheme == "":
		return nil, errors.New("url scheme is empty")
	case parsedURL.Host == "":
		return nil, errors.New("url host is empty")
	case strings.Contains(parsedURL.Host, ".."):
		return nil, errors.New("invalid url host")
	}

	return parsedURL, nil
}
