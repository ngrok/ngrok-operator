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
