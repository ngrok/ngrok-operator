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
	"testing"

	"github.com/ngrok/ngrok-api-go/v9"
	"github.com/stretchr/testify/assert"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
)

// spec is the kind-neutral desired state a test case builds both kinds from.
// Metadata is given per kind because the two kinds store it differently.
type spec struct {
	url        string
	desc       string
	legacyMeta string
	v1Meta     map[string]string
	bindings   []string
	pooling    *bool
}

// kinds builds one CloudEndpoint of every kind from s, so each test runs
// against both and the kinds cannot drift.
func kinds(s spec) map[string]Resource {
	var legacyMeta json.RawMessage
	if s.legacyMeta != "" {
		legacyMeta = json.RawMessage(s.legacyMeta)
	}
	return map[string]Resource{
		"v1": &ngrokv1.CloudEndpoint{
			Name: "clep", Namespace: "default", Generation: 1,
			Spec: ngrokv1.CloudEndpointSpec{
				URL: s.url, Description: s.desc, Metadata: s.v1Meta,
				Bindings: s.bindings, PoolingEnabled: s.pooling,
			},
		},
		// LEGACY-cloudendpoint-kind: delete this entry at cleanup.
		"legacy": &ngrokv1alpha1.CloudEndpoint{
			Name: "clep", Namespace: "default", Generation: 1,
			Spec: ngrokv1alpha1.CloudEndpointSpec{
				URL: s.url, Description: s.desc, Metadata: legacyMeta,
				Bindings: s.bindings, PoolingEnabled: s.pooling,
			},
		},
	}
}

func TestNeedsUpdate(t *testing.T) {
	baseRemote := func() *ngrok.Endpoint {
		return &ngrok.Endpoint{
			URL:            "https://example.ngrok.app",
			Description:    "Created by the ngrok-operator",
			Metadata:       `{"owned-by":"ngrok-operator"}`,
			TrafficPolicy:  `{"on_http_request":[]}`,
			Bindings:       []string{"public"},
			PoolingEnabled: false,
		}
	}
	baseSpec := func() spec {
		return spec{
			url:        "https://example.ngrok.app",
			desc:       "Created by the ngrok-operator",
			legacyMeta: `{"owned-by":"ngrok-operator"}`,
			v1Meta:     map[string]string{"owned-by": "ngrok-operator"},
			bindings:   []string{"public"},
			pooling:    new(false),
		}
	}
	basePolicy := `{"on_http_request":[]}`

	tests := []struct {
		name   string
		remote func(*ngrok.Endpoint)
		spec   func(*spec)
		policy string
		want   bool
	}{
		{
			name: "no update needed when everything matches",
		},
		{
			name: "update needed when URL changes",
			spec: func(s *spec) { s.url = "https://other.ngrok.app" },
			want: true,
		},
		{
			name: "update needed when description changes",
			spec: func(s *spec) { s.desc = "new description" },
			want: true,
		},
		{
			name: "update needed when metadata changes",
			spec: func(s *spec) {
				s.legacyMeta = `{"owned-by":"someone-else"}`
				s.v1Meta = map[string]string{"owned-by": "someone-else"}
			},
			want: true,
		},
		{
			name: "metadata key order does not matter (remote sorted)",
			remote: func(e *ngrok.Endpoint) {
				e.Metadata = `{"a":"1","b":"2"}`
			},
			spec: func(s *spec) {
				s.legacyMeta = `{"b":"2","a":"1"}`
				s.v1Meta = map[string]string{"a": "1", "b": "2"}
			},
		},
		{
			name: "metadata key order does not matter (remote unsorted)",
			remote: func(e *ngrok.Endpoint) {
				e.Metadata = `{"b":"2","a":"1"}`
			},
			spec: func(s *spec) {
				s.legacyMeta = `{"b":"2","a":"1"}`
				s.v1Meta = map[string]string{"a": "1", "b": "2"}
			},
		},
		{
			name: "metadata value differs with keys reordered",
			remote: func(e *ngrok.Endpoint) {
				e.Metadata = `{"b":"2","a":"1"}`
			},
			spec: func(s *spec) {
				s.legacyMeta = `{"b":"3","a":"1"}`
				s.v1Meta = map[string]string{"a": "1", "b": "3"}
			},
			want: true,
		},
		{
			name:   "update needed when traffic policy changes",
			policy: `{"on_http_request":[{"actions":[]}]}`,
			want:   true,
		},
		{
			name: "update needed when bindings change",
			spec: func(s *spec) { s.bindings = []string{"internal"} },
			want: true,
		},
		{
			name: "update needed when pooling enabled changes",
			spec: func(s *spec) { s.pooling = new(true) },
			want: true,
		},
		{
			name: "nil pooling enabled in spec skips comparison",
			spec: func(s *spec) { s.pooling = nil },
		},
		{
			name:   "nil and empty bindings are equal",
			remote: func(e *ngrok.Endpoint) { e.Bindings = nil },
			spec:   func(s *spec) { s.bindings = nil },
		},
		{
			name:   "empty slice and nil bindings are equal",
			remote: func(e *ngrok.Endpoint) { e.Bindings = []string{} },
			spec:   func(s *spec) { s.bindings = nil },
		},
	}

	for _, tt := range tests {
		s := baseSpec()
		if tt.spec != nil {
			tt.spec(&s)
		}
		policy := basePolicy
		if tt.policy != "" {
			policy = tt.policy
		}
		for kind, r := range kinds(s) {
			t.Run(kind+"/"+tt.name, func(t *testing.T) {
				remote := baseRemote()
				if tt.remote != nil {
					tt.remote(remote)
				}
				assert.Equal(t, tt.want, NeedsUpdate(remote, r, policy))
			})
		}
	}
}

func TestMetadataEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{"both empty", "", "", true},
		{"identical objects", `{"a":"1"}`, `{"a":"1"}`, true},
		{"reordered keys", `{"a":"1","b":"2"}`, `{"b":"2","a":"1"}`, true},
		{"whitespace differs", `{"a":"1"}`, `{ "a": "1" }`, true},
		{"different value", `{"a":"1"}`, `{"a":"2"}`, false},
		{"extra key", `{"a":"1"}`, `{"a":"1","b":"2"}`, false},
		{"empty vs object", "", `{"a":"1"}`, false},
		{"identical free text", "free text", "free text", true},
		{"different free text", "free text", "other text", false},
		{"free text vs object", "free text", `{"a":"1"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, MetadataEqual(tt.a, tt.b))
			assert.Equal(t, tt.want, MetadataEqual(tt.b, tt.a))
		})
	}
}
