package ir

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- Helper functions to create pointers ---
func stringPtr(s string) *string {
	return &s
}

func pathTypePtr(pt IRPathMatchType) *IRPathMatchType {
	return &pt
}

func methodPtr(m IRMethodMatch) *IRMethodMatch {
	return &m
}
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
					MatchCriteria: IRHTTPMatch{
						Path:     stringPtr("/foo"),
						PathType: pathTypePtr(IRPathType_Prefix),
					},
				},
				{
					MatchCriteria: IRHTTPMatch{
						Path:     stringPtr("/bar"),
						PathType: pathTypePtr(IRPathType_Exact),
					},
				},
			},
			expectedOrder: []*IRRoute{
				{
					MatchCriteria: IRHTTPMatch{
						Path:     stringPtr("/bar"),
						PathType: pathTypePtr(IRPathType_Exact),
					},
				},
				{
					MatchCriteria: IRHTTPMatch{
						Path:     stringPtr("/foo"),
						PathType: pathTypePtr(IRPathType_Prefix),
					},
				},
			},
		},
		{
			name: "Longer paths before shorter paths",
			routes: []*IRRoute{
				{
					MatchCriteria: IRHTTPMatch{
						Path:     stringPtr("/longer"),
						PathType: pathTypePtr(IRPathType_Exact),
					},
				},
				{
					MatchCriteria: IRHTTPMatch{
						Path:     stringPtr("/short"),
						PathType: pathTypePtr(IRPathType_Exact),
					},
				},
			},
			expectedOrder: []*IRRoute{
				{
					MatchCriteria: IRHTTPMatch{
						Path:     stringPtr("/longer"),
						PathType: pathTypePtr(IRPathType_Exact),
					},
				},
				{
					MatchCriteria: IRHTTPMatch{
						Path:     stringPtr("/short"),
						PathType: pathTypePtr(IRPathType_Exact),
					},
				},
			},
		},
		{
			name: "Lexicographical order for same path length and type",
			routes: []*IRRoute{
				{
					MatchCriteria: IRHTTPMatch{
						Path:     stringPtr("/b"),
						PathType: pathTypePtr(IRPathType_Exact),
					},
				},
				{
					MatchCriteria: IRHTTPMatch{
						Path:     stringPtr("/a"),
						PathType: pathTypePtr(IRPathType_Exact),
					},
				},
			},
			expectedOrder: []*IRRoute{
				{
					MatchCriteria: IRHTTPMatch{
						Path:     stringPtr("/a"),
						PathType: pathTypePtr(IRPathType_Exact),
					},
				},
				{
					MatchCriteria: IRHTTPMatch{
						Path:     stringPtr("/b"),
						PathType: pathTypePtr(IRPathType_Exact),
					},
				},
			},
		},
		{
			name: "Header specificity: routes with headers come before those without",
			routes: []*IRRoute{
				{
					MatchCriteria: IRHTTPMatch{
						Headers: []IRHeaderMatch{
							{Name: "X-Test", Value: "foo", ValueType: IRStringValueType_Exact},
						},
					},
				},
				{
					MatchCriteria: IRHTTPMatch{},
				},
			},
			expectedOrder: []*IRRoute{
				{
					MatchCriteria: IRHTTPMatch{
						Headers: []IRHeaderMatch{
							{Name: "X-Test", Value: "foo", ValueType: IRStringValueType_Exact},
						},
					},
				},
				{
					MatchCriteria: IRHTTPMatch{},
				},
			},
		},
		{
			name: "Query param specificity: routes with query params come before those without",
			routes: []*IRRoute{
				{
					MatchCriteria: IRHTTPMatch{
						QueryParams: []IRQueryParamMatch{
							{Name: "id", Value: "123", ValueType: IRStringValueType_Exact},
						},
					},
				},
				{
					MatchCriteria: IRHTTPMatch{},
				},
			},
			expectedOrder: []*IRRoute{
				{
					MatchCriteria: IRHTTPMatch{
						QueryParams: []IRQueryParamMatch{
							{Name: "id", Value: "123", ValueType: IRStringValueType_Exact},
						},
					},
				},
				{
					MatchCriteria: IRHTTPMatch{},
				},
			},
		},
		{
			name: "Method specificity: routes with method come before those without",
			routes: []*IRRoute{
				{
					MatchCriteria: IRHTTPMatch{
						Method: methodPtr(IRMethodMatch_Get),
					},
				},
				{
					MatchCriteria: IRHTTPMatch{},
				},
			},
			expectedOrder: []*IRRoute{
				{
					MatchCriteria: IRHTTPMatch{
						Method: methodPtr(IRMethodMatch_Get),
					},
				},
				{
					MatchCriteria: IRHTTPMatch{},
				},
			},
		},
		{
			name: "Combined criteria ordering",
			routes: []*IRRoute{
				// Route A: has path "/a", exact, no headers, no query, no method.
				{
					MatchCriteria: IRHTTPMatch{
						Path:     stringPtr("/a"),
						PathType: pathTypePtr(IRPathType_Exact),
					},
				},
				// Route B: has path "/a", exact, with headers.
				{
					MatchCriteria: IRHTTPMatch{
						Path:     stringPtr("/a"),
						PathType: pathTypePtr(IRPathType_Exact),
						Headers: []IRHeaderMatch{
							{Name: "X", Value: "1", ValueType: IRStringValueType_Exact},
						},
					},
				},
				// Route C: no path, with headers.
				{
					MatchCriteria: IRHTTPMatch{
						Headers: []IRHeaderMatch{
							{Name: "Y", Value: "2", ValueType: IRStringValueType_Exact},
						},
					},
				},
				// Route D: no path, no headers, with method.
				{
					MatchCriteria: IRHTTPMatch{
						Method: methodPtr(IRMethodMatch_Get),
					},
				},
				// Route E: no path, no headers, no method.
				{
					MatchCriteria: IRHTTPMatch{},
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
					MatchCriteria: IRHTTPMatch{
						Path:     stringPtr("/a"),
						PathType: pathTypePtr(IRPathType_Exact),
						Headers: []IRHeaderMatch{
							{Name: "X", Value: "1", ValueType: IRStringValueType_Exact},
						},
					},
				},
				// Route A: has path "/a", exact, no headers.
				{
					MatchCriteria: IRHTTPMatch{
						Path:     stringPtr("/a"),
						PathType: pathTypePtr(IRPathType_Exact),
					},
				},
				// Then, among routes with no path, route with headers (C)
				{
					MatchCriteria: IRHTTPMatch{
						Headers: []IRHeaderMatch{
							{Name: "Y", Value: "2", ValueType: IRStringValueType_Exact},
						},
					},
				},
				// Then, route with method (D)
				{
					MatchCriteria: IRHTTPMatch{
						Method: methodPtr(IRMethodMatch_Get),
					},
				},
				// Then, route with nothing (E)
				{
					MatchCriteria: IRHTTPMatch{},
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
		tc := tc
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
