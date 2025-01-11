package ir

import (
	"sort"

	"github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	netv1 "k8s.io/api/networking/v1"
)

// The IR (Intermediate Representation) package serves as a midway point when translating configuration resources into other types of config/resources

// OwningResource is a reference to a resource + kind to keep track of IR that was generated from other resources (like Ingress)
type OwningResource struct {
	Kind      string
	Name      string
	Namespace string
}

// Type alias to make it more apparent when using maps what the key represents
type IRHostname string

// IRVirtualHost represents a unique hostname and all of the rotues under that hostname
type IRVirtualHost struct {
	// The names of any resources (such as Ingress) that were used in the construction of this IRVirtualHost
	// Currently only used for debug/error logs, but can be added to generated resource statuses
	OwningResources []OwningResource
	Hostname        string

	// Keeps track of the namespace for this hostname. Since we do not allow multiple endpoints with the same hostname, we cannot support multiple ingresses
	// using the same hostname in different namespaces.
	Namespace string

	// This traffic policy will apply to all routes under this hostname
	TrafficPolicy *trafficpolicy.TrafficPolicy
	Routes        []*IRRoute

	// The following is used to support ingress default backends (currently only supported for endpoints and not edges)
	DefaultDestination *IRDestination
}

// IRRoute is a path match paired with a destination for requests with a matching path
type IRRoute struct {
	Path        string
	PathType    netv1.PathType
	Destination *IRDestination
}

// IRDestination determines what should be done with a request. One of upstream service or a traffic policy can be supplied
type IRDestination struct {
	Upstream      *IRUpstream
	TrafficPolicy *trafficpolicy.TrafficPolicy
}

// IRUpstream is a service upstream along with a list of the ingresses that route to that service
type IRUpstream struct {
	// The names of any resources (such as Ingress) that were used in the construction of this IRUpstream
	// Currently only used for debug/error logs, but can be added to generated resource statuses
	OwningResources []OwningResource
	Service         IRService
}

// IRService is an upstream service that we can route requests to
type IRService struct {
	UID       string // UID of the service so that we don't generate the exact same endpoints for the same service running in two different clusters
	Namespace string
	Name      string
	Port      int32
}

// sortRoutes sorts the routes for an IRVirtualHost.
//  1. We always generate the same ordering of route rules for any given set of ingresses
//  2. Since traffic policy rules are executed in-order, we need to order them in a way that results in best-match routing
func (h *IRVirtualHost) SortRoutes() {
	sort.SliceStable(h.Routes, func(i, j int) bool {
		// Exact matches before prefix matches
		if h.Routes[i].PathType != h.Routes[j].PathType {
			return h.Routes[i].PathType == netv1.PathTypeExact
		}

		// Then, longer paths before shorter paths
		if len(h.Routes[i].Path) != len(h.Routes[j].Path) {
			return len(h.Routes[i].Path) > len(h.Routes[j].Path)
		}

		// Finally, lexicographical order
		return h.Routes[i].Path < h.Routes[j].Path
	})
}

// addOwningIngress will append a new namespaced name to the list of owning ingresses for a virtual host
func (h *IRVirtualHost) AddOwningResource(new OwningResource) {
	for _, current := range h.OwningResources {
		if current == new {
			return
		}
	}
	h.OwningResources = append(h.OwningResources, new)
}

// addOwningIngress will append a new namespaced name to the list of owning ingresses for an upstream
func (h *IRUpstream) AddOwningResource(new OwningResource) {
	for _, current := range h.OwningResources {
		if current == new {
			return
		}
	}
	h.OwningResources = append(h.OwningResources, new)
}
