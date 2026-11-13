package ir

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSortRoutes(t *testing.T) {
	testCases := []struct {
		name          string
		routes        []*IRRoute
		expectedOrder []*IRRoute
	}{
		{
			name: "Path and PathType sorting: Exact vs Prefix",
			routes: []*IRRoute{
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Path:     new("/foo"),
						PathType: new(IRPathType_Prefix),
					},
				},
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Path:     new("/bar"),
						PathType: new(IRPathType_Exact),
					},
				},
			},
			expectedOrder: []*IRRoute{
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Path:     new("/bar"),
						PathType: new(IRPathType_Exact),
					},
				},
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Path:     new("/foo"),
						PathType: new(IRPathType_Prefix),
					},
				},
			},
		},
		{
			name: "Longer paths before shorter paths",
			routes: []*IRRoute{
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Path:     new("/longer"),
						PathType: new(IRPathType_Exact),
					},
				},
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Path:     new("/short"),
						PathType: new(IRPathType_Exact),
					},
				},
			},
			expectedOrder: []*IRRoute{
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Path:     new("/longer"),
						PathType: new(IRPathType_Exact),
					},
				},
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Path:     new("/short"),
						PathType: new(IRPathType_Exact),
					},
				},
			},
		},
		{
			name: "Lexicographical order for same path length and type",
			routes: []*IRRoute{
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Path:     new("/b"),
						PathType: new(IRPathType_Exact),
					},
				},
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Path:     new("/a"),
						PathType: new(IRPathType_Exact),
					},
				},
			},
			expectedOrder: []*IRRoute{
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Path:     new("/a"),
						PathType: new(IRPathType_Exact),
					},
				},
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Path:     new("/b"),
						PathType: new(IRPathType_Exact),
					},
				},
			},
		},
		{
			name: "Header specificity: routes with headers come before those without",
			routes: []*IRRoute{
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Headers: []IRHeaderMatch{
							{Name: "X-Test", Value: "foo", ValueType: IRStringValueType_Exact},
						},
					},
				},
				{
					HTTPMatchCriteria: &IRHTTPMatch{},
				},
			},
			expectedOrder: []*IRRoute{
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Headers: []IRHeaderMatch{
							{Name: "X-Test", Value: "foo", ValueType: IRStringValueType_Exact},
						},
					},
				},
				{
					HTTPMatchCriteria: &IRHTTPMatch{},
				},
			},
		},
		{
			name: "Query param specificity: routes with query params come before those without",
			routes: []*IRRoute{
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						QueryParams: []IRQueryParamMatch{
							{Name: "id", Value: "123", ValueType: IRStringValueType_Exact},
						},
					},
				},
				{
					HTTPMatchCriteria: &IRHTTPMatch{},
				},
			},
			expectedOrder: []*IRRoute{
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						QueryParams: []IRQueryParamMatch{
							{Name: "id", Value: "123", ValueType: IRStringValueType_Exact},
						},
					},
				},
				{
					HTTPMatchCriteria: &IRHTTPMatch{},
				},
			},
		},
		{
			name: "Method specificity: routes with method come before those without",
			routes: []*IRRoute{
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Method: new(IRMethodMatch_Get),
					},
				},
				{
					HTTPMatchCriteria: &IRHTTPMatch{},
				},
			},
			expectedOrder: []*IRRoute{
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Method: new(IRMethodMatch_Get),
					},
				},
				{
					HTTPMatchCriteria: &IRHTTPMatch{},
				},
			},
		},
		{
			name: "Combined criteria ordering",
			routes: []*IRRoute{
				// Route A: has path "/a", exact, no headers, no query, no method.
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Path:     new("/a"),
						PathType: new(IRPathType_Exact),
					},
				},
				// Route B: has path "/a", exact, with headers.
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Path:     new("/a"),
						PathType: new(IRPathType_Exact),
						Headers: []IRHeaderMatch{
							{Name: "X", Value: "1", ValueType: IRStringValueType_Exact},
						},
					},
				},
				// Route C: no path, with headers.
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Headers: []IRHeaderMatch{
							{Name: "Y", Value: "2", ValueType: IRStringValueType_Exact},
						},
					},
				},
				// Route D: no path, no headers, with method.
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Method: new(IRMethodMatch_Get),
					},
				},
				// Route E: no path, no headers, no method.
				{
					HTTPMatchCriteria: &IRHTTPMatch{},
				},
			},
			// Expected order:
			// - Routes with a path come before those without
			//   Among those with a path, the one with headers is more specific
			// - Among routes with no path, the one with headers comes first
			//   then the one with a method, then the one with nothing
			expectedOrder: []*IRRoute{
				// Route B: has path "/a", exact, with headers.
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Path:     new("/a"),
						PathType: new(IRPathType_Exact),
						Headers: []IRHeaderMatch{
							{Name: "X", Value: "1", ValueType: IRStringValueType_Exact},
						},
					},
				},
				// Route A: has path "/a", exact, no headers.
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Path:     new("/a"),
						PathType: new(IRPathType_Exact),
					},
				},
				// Then, among routes with no path, route with headers (C)
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Headers: []IRHeaderMatch{
							{Name: "Y", Value: "2", ValueType: IRStringValueType_Exact},
						},
					},
				},
				// Then, route with method (D)
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Method: new(IRMethodMatch_Get),
					},
				},
				// Then, route with nothing (E)
				{
					HTTPMatchCriteria: &IRHTTPMatch{},
				},
			},
		},
		{
			name: "Routes with no match criteria come last",
			routes: []*IRRoute{

				{
					HTTPMatchCriteria: nil,
				},
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Path:     new("/test"),
						PathType: new(IRPathType_Exact),
					},
				},
			},
			expectedOrder: []*IRRoute{
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Path:     new("/test"),
						PathType: new(IRPathType_Exact),
					},
				},
				{
					HTTPMatchCriteria: nil,
				},
			},
		},
		{
			name: "Routes with no match criteria come last (different initial order)",
			routes: []*IRRoute{
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Path:     new("/test"),
						PathType: new(IRPathType_Exact),
					},
				},
				{
					HTTPMatchCriteria: nil,
				},
			},
			expectedOrder: []*IRRoute{
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Path:     new("/test"),
						PathType: new(IRPathType_Exact),
					},
				},
				{
					HTTPMatchCriteria: nil,
				},
			},
		},
		{
			name: "Routes with no match criteria come last (multiple with no match criteria)",
			routes: []*IRRoute{
				{
					HTTPMatchCriteria: nil,
				},
				{
					HTTPMatchCriteria: nil,
				},
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Path:     new("/test"),
						PathType: new(IRPathType_Exact),
					},
				},
			},
			expectedOrder: []*IRRoute{
				{
					HTTPMatchCriteria: &IRHTTPMatch{
						Path:     new("/test"),
						PathType: new(IRPathType_Exact),
					},
				},
				{
					HTTPMatchCriteria: nil,
				},
				{
					HTTPMatchCriteria: nil,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			vhost := &IRVirtualHost{
				Routes: tc.routes,
			}
			vhost.SortRoutes()
			assert.Equal(t, tc.expectedOrder, vhost.Routes, "routes should be sorted correctly")
		})
	}
}

func TestAddOwningResource(t *testing.T) {
	testCases := []struct {
		name          string
		initial       []OwningResource
		newResource   OwningResource
		expectedFinal []OwningResource
	}{
		{
			name:    "Add unique resource to empty list",
			initial: []OwningResource{},
			newResource: OwningResource{
				Kind:      "Ingress",
				Name:      "test-ingress",
				Namespace: "default",
			},
			expectedFinal: []OwningResource{
				{
					Kind:      "Ingress",
					Name:      "test-ingress",
					Namespace: "default",
				},
			},
		},
		{
			name: "Add unique resource to non-empty list",
			initial: []OwningResource{
				{
					Kind:      "Ingress",
					Name:      "existing-ingress",
					Namespace: "default",
				},
			},
			newResource: OwningResource{
				Kind:      "Ingress",
				Name:      "test-ingress",
				Namespace: "default",
			},
			expectedFinal: []OwningResource{
				{
					Kind:      "Ingress",
					Name:      "existing-ingress",
					Namespace: "default",
				},
				{
					Kind:      "Ingress",
					Name:      "test-ingress",
					Namespace: "default",
				},
			},
		},
		{
			name: "Do not add duplicate resource",
			initial: []OwningResource{
				{
					Kind:      "Ingress",
					Name:      "test-ingress",
					Namespace: "default",
				},
			},
			newResource: OwningResource{
				Kind:      "Ingress",
				Name:      "test-ingress",
				Namespace: "default",
			},
			expectedFinal: []OwningResource{
				{
					Kind:      "Ingress",
					Name:      "test-ingress",
					Namespace: "default",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run("IRVirtualHost/"+tc.name, func(t *testing.T) {
			host := &IRVirtualHost{OwningResources: tc.initial}
			host.AddOwningResource(tc.newResource)
			assert.Equal(t, tc.expectedFinal, host.OwningResources, "unexpected result in test case: %s", tc.name)
		})

		t.Run("IRUpstream/"+tc.name, func(t *testing.T) {
			upstream := &IRUpstream{OwningResources: tc.initial}
			upstream.AddOwningResource(tc.newResource)
			assert.Equal(t, tc.expectedFinal, upstream.OwningResources, "unexpected result in test case: %s", tc.name)
		})
	}
}

func TestMergeMetadata(t *testing.T) {
	testCases := []struct {
		name     string
		base     string
		override string
		wantKeys map[string]any // expected keys in merged result
		wantRaw  string         // when we want exact raw equality (empty string cases)
	}{
		{
			name:     "empty override returns base unchanged",
			base:     `{"owned-by":"ngrok-operator"}`,
			override: "",
			wantKeys: map[string]any{"owned-by": "ngrok-operator"},
		},
		{
			name:     "empty base returns override",
			base:     "",
			override: `{"env":"prod"}`,
			wantKeys: map[string]any{"env": "prod"},
		},
		{
			name:     "both empty returns empty string",
			base:     "",
			override: "",
			wantRaw:  "",
		},
		{
			name:     "override keys take precedence over base",
			base:     `{"owned-by":"ngrok-operator","env":"dev"}`,
			override: `{"env":"prod"}`,
			wantKeys: map[string]any{"owned-by": "ngrok-operator", "env": "prod"},
		},
		{
			name:     "base and override are merged - no overlap",
			base:     `{"owned-by":"ngrok-operator"}`,
			override: `{"team":"platform","region":"us-east-1"}`,
			wantKeys: map[string]any{"owned-by": "ngrok-operator", "team": "platform", "region": "us-east-1"},
		},
		{
			name:     "invalid override JSON returns base unchanged",
			base:     `{"owned-by":"ngrok-operator"}`,
			override: `not-valid-json`,
			wantKeys: map[string]any{"owned-by": "ngrok-operator"},
		},
		{
			name:     "invalid base JSON is treated as empty object",
			base:     `not-valid-json`,
			override: `{"env":"prod"}`,
			wantKeys: map[string]any{"env": "prod"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := MergeMetadata(tc.base, tc.override)

			if tc.wantRaw != "" || (tc.base == "" && tc.override == "") {
				assert.Equal(t, tc.wantRaw, result)
				return
			}

			var got map[string]any
			require.NoError(t, json.Unmarshal([]byte(result), &got), "result should be valid JSON: %s", result)
			assert.Equal(t, tc.wantKeys, got)
		})
	}
}

func TestSortRoutesGatewayPrecedence(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	route := func(name string, p IRRoutePrecedence, m IRHTTPMatch) *IRRoute {
		p.Route = name
		if p.CreatedAt.IsZero() {
			p.CreatedAt = t0
		}
		return &IRRoute{HTTPMatchCriteria: &m, Precedence: &p}
	}
	prefix := func(path string) IRHTTPMatch {
		return IRHTTPMatch{Path: new(path), PathType: new(IRPathType_Prefix)}
	}
	exact := func(path string) IRHTTPMatch {
		return IRHTTPMatch{Path: new(path), PathType: new(IRPathType_Exact)}
	}
	withHeaders := func(m IRHTTPMatch, n int) IRHTTPMatch {
		for i := range n {
			m.Headers = append(m.Headers, IRHeaderMatch{Name: string(rune('a' + i)), Value: "v", ValueType: IRStringValueType_Exact})
		}
		return m
	}
	withQuery := func(m IRHTTPMatch, n int) IRHTTPMatch {
		for i := range n {
			m.QueryParams = append(m.QueryParams, IRQueryParamMatch{Name: string(rune('a' + i)), Value: "v", ValueType: IRStringValueType_Exact})
		}
		return m
	}
	withMethod := func(m IRHTTPMatch) IRHTTPMatch {
		m.Method = new(IRMethodMatch_Get)
		return m
	}

	tests := []struct {
		name   string
		routes []*IRRoute
		want   []string
	}{
		{
			name: "exact hostname beats wildcard, which beats none",
			routes: []*IRRoute{
				route("none", IRRoutePrecedence{}, prefix("/")),
				route("wildcard", IRRoutePrecedence{Hostname: "*.example.com"}, prefix("/")),
				route("exact", IRRoutePrecedence{Hostname: "foo.example.com"}, prefix("/")),
			},
			want: []string{"exact/0/0", "wildcard/0/0", "none/0/0"},
		},
		{
			name: "hostname outranks a more specific path",
			routes: []*IRRoute{
				route("long-path", IRRoutePrecedence{Hostname: "*.example.com"}, exact("/a/very/long/path")),
				route("host", IRRoutePrecedence{Hostname: "foo.example.com"}, prefix("/")),
			},
			want: []string{"host/0/0", "long-path/0/0"},
		},
		{
			name: "exact path beats a longer prefix",
			routes: []*IRRoute{
				route("prefix", IRRoutePrecedence{}, prefix("/foo/bar")),
				route("exact", IRRoutePrecedence{}, exact("/foo")),
			},
			want: []string{"exact/0/0", "prefix/0/0"},
		},
		{
			name: "method beats headers, more headers beat fewer, then query params",
			routes: []*IRRoute{
				route("one-header", IRRoutePrecedence{}, withHeaders(prefix("/"), 1)),
				route("two-query", IRRoutePrecedence{}, withQuery(withHeaders(prefix("/"), 1), 2)),
				route("two-headers", IRRoutePrecedence{}, withHeaders(prefix("/"), 2)),
				route("method", IRRoutePrecedence{}, withMethod(prefix("/"))),
			},
			want: []string{"method/0/0", "two-headers/0/0", "two-query/0/0", "one-header/0/0"},
		},
		{
			name: "ties go to the oldest route, then the name, then rule and match order",
			routes: []*IRRoute{
				route("b", IRRoutePrecedence{RuleIndex: 0}, prefix("/")),
				route("a", IRRoutePrecedence{RuleIndex: 1}, prefix("/")),
				route("a", IRRoutePrecedence{RuleIndex: 0, MatchIndex: 1}, prefix("/")),
				route("newer", IRRoutePrecedence{CreatedAt: t0.Add(-time.Hour)}, prefix("/")),
			},
			want: []string{"newer/0/0", "a/0/1", "a/1/0", "b/0/0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vh := &IRVirtualHost{Routes: tt.routes}
			vh.SortRoutes()
			got := []string{}
			for _, r := range vh.Routes {
				got = append(got, fmt.Sprintf("%s/%d/%d", r.Precedence.Route, r.Precedence.RuleIndex, r.Precedence.MatchIndex))
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
