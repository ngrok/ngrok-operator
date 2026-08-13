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

// LEGACY-enabledfeatures-format: BEGIN
//
// KubernetesOperatorEnabledFeatures decodes either the array this field now
// writes, or the comma-separated string written by pre-migration operator
// versions.
//
// Two-release, same-key type change (see docs/developer-guide/passivity-shims.md):
// write-side cleanup is done — the field marshals as a plain array again.
// Keep UnmarshalJSON a while longer — existing objects still carry the
// legacy string until their next reconcile — then delete it too (read-side
// cleanup) along with this type, switching the field to plain []string.
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

// LEGACY-enabledfeatures-format: END
