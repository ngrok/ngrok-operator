package ir

import (
	"fmt"
	"sort"

	"github.com/ngrok/ngrok-operator/internal/trafficpolicy"
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

type IRProtocol string

const (
	IRProtocol_HTTPS IRProtocol = "HTTPS"
	IRProtocol_HTTP  IRProtocol = "HTTP"

	// Note: TCP/TLS/UDP not currently supported
)

type IRListener struct {
	// The hostname to listen on
	Hostname IRHostname

	// The port to listen on
	Port int32

	// The protocol to expect
	Protocol IRProtocol
}

// IRVirtualHost represents a unique hostname and all of the rotues under that hostname
type IRVirtualHost struct {
	// optional prefix for the name of generated cloud endpoints from this irVirutalHost.
	// When this is not set, the name will be created from the hostname. This enables having multiple
	// different endpoints for the same hostname such as from different Gateways that you want to make a pool of endpoints
	// for the same hostname without having conflicting names
	NamePrefix *string

	// Keeps track of the namespace for this hostname. Since we do not allow multiple endpoints with the same hostname, we cannot support multiple ingresses
	// using the same hostname in different namespaces.
	Namespace string

	// Defines how we should listen for incoming traffic
	Listener IRListener

	// Enables/disables endpoint pooling for endpoints created from this virtual host
	EndpointPoolingEnabled bool

	// This traffic policy will apply to all routes under this hostname
	TrafficPolicy *trafficpolicy.TrafficPolicy
	// Reference to the object that the above traffic policy config was loaded from
	TrafficPolicyObj *OwningResource

	// Routes define the various acceptance criteria for incoming traffic and what to do with it
	Routes []*IRRoute

	// The following is used to support ingress default backends (currently only supported for endpoints and not edges)
	DefaultDestination *IRDestination

	// Optional configuration for TLS termination, when nil, the default ngrok endpoint behaviour is used
	TLSTermination *IRTLSTermination

	// Optional list of references to secrets for client certificates to use when communicating with upstream services for this virtual host
	ClientCertRefs []IRObjectRef

	// Any key/value pairs in this map will be added to any resources created from this IRVirtualHost as labels
	LabelsToAdd map[string]string
	// Any key/value pairs in this map will be added to any resources created from this IRVirtualHost as annotations
	AnnotationsToAdd map[string]string

	// The names of any resources (such as Ingress) that were used in the construction of this IRVirtualHost
	// Currently only used for debug/error logs, but can be added to generated resource statuses
	OwningResources []OwningResource

	// Metadata to set on any created CloudEndpoints/AgentEndpoints
	Metadata string

	// Bindings to set on generated Endpoints
	Bindings []string
}

type IRTLSTermination struct {
	ExtendedOptions                 map[string]string
	ServerPrivateKey                *string
	ServerCertificate               *string
	MutualTLSCertificateAuthorities []string
}

type IRPathMatchType string

const (
	IRPathType_Prefix IRPathMatchType = "prefix"
	IRPathType_Exact  IRPathMatchType = "exact"
	IRPathType_Regex  IRPathMatchType = "regex"
)

type IRMethodMatch string

const (
	IRMethodMatch_Get     IRMethodMatch = "GET"
	IRMethodMatch_Head    IRMethodMatch = "HEAD"
	IRMethodMatch_Post    IRMethodMatch = "POST"
	IRMethodMatch_Put     IRMethodMatch = "PUT"
	IRMethodMatch_Delete  IRMethodMatch = "DELETE"
	IRMethodMatch_Connect IRMethodMatch = "CONNECT"
	IRMethodMatch_Options IRMethodMatch = "OPTIONS"
	IRMethodMatch_Trace   IRMethodMatch = "TRACE"
	IRMethodMatch_Patch   IRMethodMatch = "PATCH"
)

type IRHeaderMatch struct {
	Name      string
	Value     string
	ValueType IRStringValueType
}

type IRStringValueType string

const (
	IRStringValueType_Exact IRStringValueType = "exact"
	IRStringValueType_Regex IRStringValueType = "regex"
)

type IRQueryParamMatch struct {
	Name      string
	Value     string
	ValueType IRStringValueType
}

// IRRoute is a path match paired with a destination for requests with a matching path
type IRRoute struct {
	MatchCriteria IRHTTPMatch
	// These traffic policies will apply to the route regardless of which destination is chosen.
	// They are not intended to be terminating.
	// This supports the per-route use-case such as adding/removing headers, etc. before routing
	TrafficPolicies []*trafficpolicy.TrafficPolicy

	// A list of destinations for the route. A single destination will receive 100% of the traffic, otherwise
	// the destination for any given request will be chosen according to the weights of the destinations.
	Destinations []*IRDestination
}

type IRHTTPMatch struct {
	Path        *string
	PathType    *IRPathMatchType
	Headers     []IRHeaderMatch
	QueryParams []IRQueryParamMatch
	Method      *IRMethodMatch
}

// IRDestination determines what should be done with a request. One of upstream service or a traffic policy can be supplied
type IRDestination struct {
	// The weight of this destination, used to determine which destination will receive a request
	// When left out, the destination will receive 100% of the traffic.
	// Weight must be greater than 0 when supplied.
	Weight *int

	// The upstream service that will receive traffic
	Upstream *IRUpstream

	// Traffic policies for this destination. It is perfectly valid to have only traffic policies and no upstream
	TrafficPolicies []*trafficpolicy.TrafficPolicy
}

type IRUpstream struct {
	// The names of any resources (such as Ingress) that were used in the construction of this IRUpstream
	// Currently only used for debug/error logs, but can be added to generated resource statuses
	OwningResources []OwningResource
	Service         IRService
}

// IRService is an upstream service that we can route requests to
type IRService struct {
	UID            string // UID of the service so that we don't generate the exact same endpoints for the same service running in two different clusters
	Namespace      string
	Name           string
	Port           int32
	ClientCertRefs []IRObjectRef
	Scheme         IRScheme
}

type IRScheme string

const (
	IRScheme_HTTP  IRScheme = "http://"
	IRScheme_HTTPS IRScheme = "https://"
)

type IRServiceKey string

func (s IRService) Key() IRServiceKey {
	key := fmt.Sprintf("%s/%s/%s/%d", s.UID, s.Namespace, s.Name, s.Port)
	for _, clientCertRef := range s.ClientCertRefs {
		key += fmt.Sprintf("/%s.%s", clientCertRef.Name, clientCertRef.Namespace)
	}
	return IRServiceKey(key)
}

type IRObjectRef struct {
	Name      string
	Namespace string
}

// SortRoutes sorts the routes for an IRVirtualHost.
// The ordering is chosen so that the most specific (best-match) routes come first.
// The order of criteria is:
//  1. Path: Routes with a defined path come before those without.
//     For routes with paths, an exact match is preferred over prefix which is preferred over regex.
//     For the same type, longer paths are preferred, then lexicographical order.
//  2. Headers: Routes with headers come before those without. When both have headers,
//     a normalized, sorted representation is used for comparison.
//  3. Query Params: Similar to headers.
//  4. Method: Routes specifying a method come before those that don’t; otherwise, lex order.
func (h *IRVirtualHost) SortRoutes() {
	sort.SliceStable(h.Routes, func(i, j int) bool {
		mi := h.Routes[i].MatchCriteria
		mj := h.Routes[j].MatchCriteria

		// 1. Compare Path.
		// If only one route specifies a path, that route is more specific.
		if mi.Path != nil && mj.Path == nil {
			return true
		} else if mi.Path == nil && mj.Path != nil {
			return false
		} else if mi.Path != nil && mj.Path != nil {
			// Compare path type.
			orderI := pathTypeOrder(*mi.PathType)
			orderJ := pathTypeOrder(*mj.PathType)
			if orderI != orderJ {
				return orderI < orderJ
			}
			// For the same path type, longer paths are more specific.
			if len(*mi.Path) != len(*mj.Path) {
				return len(*mi.Path) > len(*mj.Path)
			}
			// If still tied, compare lexicographically.
			if *mi.Path != *mj.Path {
				return *mi.Path < *mj.Path
			}
		}

		// 2. Compare Headers.
		// Routes with header matchers are more specific than those without.
		if len(mi.Headers) > 0 && len(mj.Headers) == 0 {
			return true
		} else if len(mi.Headers) == 0 && len(mj.Headers) > 0 {
			return false
		} else if len(mi.Headers) > 0 && len(mj.Headers) > 0 {
			// Compare a normalized string representation.
			hStrI := headersToString(mi.Headers)
			hStrJ := headersToString(mj.Headers)
			if hStrI != hStrJ {
				return hStrI < hStrJ
			}
		}

		// 3. Compare Query Params.
		// Routes with query param matchers are more specific than those without.
		if len(mi.QueryParams) > 0 && len(mj.QueryParams) == 0 {
			return true
		} else if len(mi.QueryParams) == 0 && len(mj.QueryParams) > 0 {
			return false
		} else if len(mi.QueryParams) > 0 && len(mj.QueryParams) > 0 {
			qpStrI := queryParamsToString(mi.QueryParams)
			qpStrJ := queryParamsToString(mj.QueryParams)
			if qpStrI != qpStrJ {
				return qpStrI < qpStrJ
			}
		}

		// 4. Compare Method.
		// Routes that specify an HTTP method are more specific than those that do not.
		if mi.Method != nil && mj.Method == nil {
			return true
		} else if mi.Method == nil && mj.Method != nil {
			return false
		} else if mi.Method != nil && mj.Method != nil {
			if *mi.Method != *mj.Method {
				return *mi.Method < *mj.Method
			}
		}

		// Fallback: if all criteria are equal, preserve the original order.
		return false
	})
}

// pathTypeOrder returns an integer to order the path types.
// Lower values are considered more specific.
func pathTypeOrder(pt IRPathMatchType) int {
	switch pt {
	case IRPathType_Exact:
		return 0
	case IRPathType_Prefix:
		return 1
	case IRPathType_Regex:
		return 2
	default:
		return 3
	}
}

// headerValueTypeOrder returns an ordering for header or query parameter value types.
func headerValueTypeOrder(vt IRStringValueType) int {
	switch vt {
	case IRStringValueType_Exact:
		return 0
	case IRStringValueType_Regex:
		return 1
	default:
		return 2
	}
}

// headersToString produces a normalized string representation of header matchers.
// This helps compare two sets of header matchers deterministically.
func headersToString(headers []IRHeaderMatch) string {
	// Copy headers to avoid mutating the original slice.
	hCopy := make([]IRHeaderMatch, len(headers))
	copy(hCopy, headers)
	// Sort by name, then by value type (exact before regex), then by value.
	sort.SliceStable(hCopy, func(i, j int) bool {
		if hCopy[i].Name != hCopy[j].Name {
			return hCopy[i].Name < hCopy[j].Name
		}
		// Compare value types.
		vi := headerValueTypeOrder(hCopy[i].ValueType)
		vj := headerValueTypeOrder(hCopy[j].ValueType)
		if vi != vj {
			return vi < vj
		}
		return hCopy[i].Value < hCopy[j].Value
	})
	s := ""
	for _, h := range hCopy {
		s += h.Name + ":" + string(h.ValueType) + ":" + h.Value + ";"
	}
	return s
}

// queryParamsToString produces a normalized string representation for query parameter matchers.
func queryParamsToString(qps []IRQueryParamMatch) string {
	qpCopy := make([]IRQueryParamMatch, len(qps))
	copy(qpCopy, qps)
	sort.SliceStable(qpCopy, func(i, j int) bool {
		if qpCopy[i].Name != qpCopy[j].Name {
			return qpCopy[i].Name < qpCopy[j].Name
		}
		vi := headerValueTypeOrder(qpCopy[i].ValueType)
		vj := headerValueTypeOrder(qpCopy[j].ValueType)
		if vi != vj {
			return vi < vj
		}
		return qpCopy[i].Value < qpCopy[j].Value
	})
	s := ""
	for _, qp := range qpCopy {
		s += qp.Name + ":" + string(qp.ValueType) + ":" + qp.Value + ";"
	}
	return s
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
