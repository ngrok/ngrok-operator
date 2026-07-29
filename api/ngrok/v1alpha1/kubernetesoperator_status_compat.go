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
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// LEGACY-STATUS-MIGRATION: BEGIN
//
// KubernetesOperatorEnabledFeatures is named only so its decoder can passively
// read the comma-separated status value written by older operator versions.
// This compatibility is read-only: current versions always write an array.
//
// Remove this type and use []string directly once upgrades from versions that
// wrote the string representation are outside the supported upgrade window.
type KubernetesOperatorEnabledFeatures []string

func (features *KubernetesOperatorEnabledFeatures) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		*features = nil
		return nil
	}

	var values []string
	if err := json.Unmarshal(data, &values); err == nil {
		*features = values
		return nil
	}

	var legacy string
	if err := json.Unmarshal(data, &legacy); err != nil {
		return fmt.Errorf("enabledFeatures must be an array or a legacy comma-separated string: %w", err)
	}
	if legacy == "" {
		*features = nil
		return nil
	}

	*features = strings.Split(legacy, ",")
	return nil
}

// LEGACY-STATUS-MIGRATION: END
