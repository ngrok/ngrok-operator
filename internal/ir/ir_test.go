package ir

import (
	"testing"

	"github.com/stretchr/testify/assert"
	netv1 "k8s.io/api/networking/v1"
)

func TestSortRoutes(t *testing.T) {
	tests := []struct {
		name          string
		routes        []*IRRoute
		expectedOrder []*IRRoute
	}{
		{
			name: "Exact matches before prefix matches",
			routes: []*IRRoute{
				{Path: "/foo", PathType: netv1.PathTypePrefix},
				{Path: "/bar", PathType: netv1.PathTypeExact},
			},
			expectedOrder: []*IRRoute{
				{Path: "/bar", PathType: netv1.PathTypeExact},
				{Path: "/foo", PathType: netv1.PathTypePrefix},
			},
		},
		{
			name: "Longer paths before shorter paths",
			routes: []*IRRoute{
				{Path: "/longer", PathType: netv1.PathTypeExact},
				{Path: "/short", PathType: netv1.PathTypeExact},
			},
			expectedOrder: []*IRRoute{
				{Path: "/longer", PathType: netv1.PathTypeExact},
				{Path: "/short", PathType: netv1.PathTypeExact},
			},
		},
		{
			name: "Lexicographical order for same path length and type",
			routes: []*IRRoute{
				{Path: "/b", PathType: netv1.PathTypeExact},
				{Path: "/a", PathType: netv1.PathTypeExact},
			},
			expectedOrder: []*IRRoute{
				{Path: "/a", PathType: netv1.PathTypeExact},
				{Path: "/b", PathType: netv1.PathTypeExact},
			},
		},
		{
			name: "Mixed criteria",
			routes: []*IRRoute{
				{Path: "/foo", PathType: netv1.PathTypeExact},
				{Path: "/foooo", PathType: netv1.PathTypePrefix},
				{Path: "/bar", PathType: netv1.PathTypePrefix},
				{Path: "/baz", PathType: netv1.PathTypeExact},
			},
			expectedOrder: []*IRRoute{
				{Path: "/baz", PathType: netv1.PathTypeExact},
				{Path: "/foo", PathType: netv1.PathTypeExact},
				{Path: "/foooo", PathType: netv1.PathTypePrefix},
				{Path: "/bar", PathType: netv1.PathTypePrefix},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vhost := &IRVirtualHost{
				Routes: tt.routes,
			}

			vhost.SortRoutes()

			assert.Equal(t, tt.expectedOrder, vhost.Routes, "routes should be sorted correctly")
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
