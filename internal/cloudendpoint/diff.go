/*
MIT License

Copyright (c) 2026 ngrok, Inc.

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

package cloudendpoint

import (
	"encoding/json"
	"reflect"
	"slices"

	"github.com/ngrok/ngrok-api-go/v9"
)

// NeedsUpdate compares the remote endpoint from the ngrok API with the state
// r and its resolved traffic policy want. It reports whether an API update
// call is necessary.
func NeedsUpdate(remote *ngrok.Endpoint, r Resource, policy string) bool {
	if remote.URL != r.GetURL() {
		return true
	}
	if remote.Description != r.GetDescription() {
		return true
	}
	if !MetadataEqual(remote.Metadata, r.GetMetadataAPIString()) {
		return true
	}
	if remote.TrafficPolicy != policy {
		return true
	}
	if !slices.Equal(remote.Bindings, r.GetBindings()) {
		return true
	}
	if p := r.GetPoolingEnabled(); p != nil && remote.PoolingEnabled != *p {
		return true
	}
	return false
}

// MetadataEqual compares two metadata API strings semantically: when both are
// JSON objects their decoded contents are compared, so key order and
// whitespace don't matter. Anything else (legacy free text, empty) is compared
// byte for byte.
func MetadataEqual(a, b string) bool {
	if a == b {
		return true
	}
	var ma, mb map[string]any
	if json.Unmarshal([]byte(a), &ma) != nil || json.Unmarshal([]byte(b), &mb) != nil {
		return false
	}
	return reflect.DeepEqual(ma, mb)
}
