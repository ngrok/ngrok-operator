package managerdriver

import (
	"crypto/tls"
	"fmt"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// This file works out the status of Gateways and HTTPRoutes: whether each listener is valid, which
// routes attach to it, and whether each route is accepted by each of its parents and can reach its
// backends. It reads a snapshot of the cluster (gatewayAPIState) and does not talk to the API server,
// so the rules can be tested directly. The Driver writes the results at the end of every Sync.
//
// The rules follow the Gateway API spec and its conformance suite:
// https://gateway-api.sigs.k8s.io/reference/spec/

// ListenerReasonHostnameRequired is the reason an HTTP, HTTPS or TLS listener without a hostname is
// rejected when the Gateway has no addresses and no generated domain suffix is configured: ngrok routes
// by hostname, so the endpoint needs one.
const ListenerReasonHostnameRequired gatewayv1.ListenerConditionReason = "HostnameRequired"

const (
	gatewayAPIGroup = gatewayv1.GroupName
	kindGateway     = "Gateway"
	kindHTTPRoute   = "HTTPRoute"
	kindTLSRoute    = "TLSRoute"
	kindTCPRoute    = "TCPRoute"
	kindSecret      = "Secret"
	kindService     = "Service"
)

// gatewayAPIState is what the status rules read.
type gatewayAPIState struct {
	controllerName   gatewayv1.GatewayController
	hostnames        GatewayHostnames
	disableRefGrants bool
	tcpRoutesEnabled bool
	tlsRoutesEnabled bool

	referenceGrants []*gatewayv1beta1.ReferenceGrant
	getNamespace    func(name string) (*corev1.Namespace, error)
	getService      func(name, namespace string) (*corev1.Service, error)
	getSecret       func(name, namespace string) (*corev1.Secret, error)
	// gatewayOwned reports whether a Gateway exists and belongs to our GatewayClass, and returns it.
	getGateway func(key types.NamespacedName) (*gatewayv1.Gateway, bool)
	// addresses are the hostnames a Gateway is reachable at, from its Domains.
	addresses func(gw *gatewayv1.Gateway) []string
}

// listenerResult is a listener's validity, before routes are attached.
type listenerResult struct {
	accepted       metav1.Condition
	resolvedRefs   metav1.Condition
	supportedKinds []gatewayv1.RouteGroupKind
	// attachedRoutes is filled in by attachRoutes.
	attachedRoutes int32
}

// routeParentResult is a route's status for one of its parentRefs.
type routeParentResult struct {
	parentRef gatewayv1.ParentReference
	accepted  metav1.Condition
	// listeners the route attached to, by name. Empty unless accepted.
	listeners []gatewayv1.SectionName
}

func condition(t, reason string, status metav1.ConditionStatus, generation int64, msg string) metav1.Condition {
	return metav1.Condition{Type: t, Status: status, Reason: reason, Message: msg, ObservedGeneration: generation}
}

// listenerResults checks each listener of a Gateway on its own: protocol and port, hostname, route kinds
// and TLS certificates.
func (s *gatewayAPIState) listenerResults(gw *gatewayv1.Gateway) map[gatewayv1.SectionName]*listenerResult {
	results := make(map[gatewayv1.SectionName]*listenerResult, len(gw.Spec.Listeners))
	for _, l := range gw.Spec.Listeners {
		results[l.Name] = s.listenerResult(gw, l)
	}
	return results
}

func (s *gatewayAPIState) listenerResult(gw *gatewayv1.Gateway, l gatewayv1.Listener) *listenerResult {
	gen := gw.Generation
	res := &listenerResult{
		accepted:     condition(string(gatewayv1.ListenerConditionAccepted), string(gatewayv1.ListenerReasonAccepted), metav1.ConditionTrue, gen, "listener accepted by the ngrok operator"),
		resolvedRefs: condition(string(gatewayv1.ListenerConditionResolvedRefs), string(gatewayv1.ListenerReasonResolvedRefs), metav1.ConditionTrue, gen, "all references resolved"),
	}

	routeKinds := s.routeKindsForProtocol(l.Protocol)
	if routeKinds == nil {
		res.accepted = condition(string(gatewayv1.ListenerConditionAccepted), string(gatewayv1.ListenerReasonUnsupportedProtocol), metav1.ConditionFalse, gen,
			fmt.Sprintf("ngrok does not support the %q protocol", l.Protocol))
		res.supportedKinds = []gatewayv1.RouteGroupKind{}
		return res
	}

	switch {
	case l.Protocol == gatewayv1.HTTPProtocolType && l.Port != 80:
		res.accepted = condition(string(gatewayv1.ListenerConditionAccepted), string(gatewayv1.ListenerReasonPortUnavailable), metav1.ConditionFalse, gen, "ngrok only supports HTTP on port 80")
	case l.Protocol == gatewayv1.HTTPSProtocolType && l.Port != 443:
		res.accepted = condition(string(gatewayv1.ListenerConditionAccepted), string(gatewayv1.ListenerReasonPortUnavailable), metav1.ConditionFalse, gen, "ngrok only supports HTTPS on port 443")
	case (l.Hostname == nil || *l.Hostname == "") && len(gw.Spec.Addresses) == 0 && listenerUsesGeneratedDomain(l) && s.hostnames.GeneratedSuffix == "":
		res.accepted = condition(string(gatewayv1.ListenerConditionAccepted), string(ListenerReasonHostnameRequired), metav1.ConditionFalse, gen,
			"ngrok routes by hostname: set a hostname on the listener, set Gateway spec.addresses, or configure features.gateway.generatedDomainSuffix")
	}

	// Supported kinds are the requested kinds this listener can serve; any it cannot is an error.
	if l.AllowedRoutes == nil || len(l.AllowedRoutes.Kinds) == 0 {
		res.supportedKinds = routeKinds
	} else {
		res.supportedKinds = []gatewayv1.RouteGroupKind{}
		invalid := []string{}
		for _, k := range l.AllowedRoutes.Kinds {
			group := gatewayv1.Group(gatewayAPIGroup)
			if k.Group != nil {
				group = *k.Group
			}
			if slices.ContainsFunc(routeKinds, func(rk gatewayv1.RouteGroupKind) bool { return rk.Kind == k.Kind && *rk.Group == group }) {
				res.supportedKinds = append(res.supportedKinds, gatewayv1.RouteGroupKind{Group: new(group), Kind: k.Kind})
			} else {
				invalid = append(invalid, string(k.Kind))
			}
		}
		if len(invalid) > 0 {
			res.resolvedRefs = condition(string(gatewayv1.ListenerConditionResolvedRefs), string(gatewayv1.ListenerReasonInvalidRouteKinds), metav1.ConditionFalse, gen,
				fmt.Sprintf("unsupported route kinds: %s", strings.Join(invalid, ", ")))
			return res
		}
	}

	if reason, msg, ok := s.checkCertificateRefs(gw, l); !ok {
		res.resolvedRefs = condition(string(gatewayv1.ListenerConditionResolvedRefs), string(reason), metav1.ConditionFalse, gen, msg)
	}
	return res
}

// routeKindsForProtocol lists the route kinds a listener protocol carries, or nil when ngrok can't
// serve the protocol.
func (s *gatewayAPIState) routeKindsForProtocol(p gatewayv1.ProtocolType) []gatewayv1.RouteGroupKind {
	kind := func(k string) []gatewayv1.RouteGroupKind {
		return []gatewayv1.RouteGroupKind{{Group: ptr.To(gatewayv1.Group(gatewayAPIGroup)), Kind: gatewayv1.Kind(k)}}
	}
	switch p {
	case gatewayv1.HTTPProtocolType, gatewayv1.HTTPSProtocolType:
		return kind(kindHTTPRoute)
	case gatewayv1.TLSProtocolType:
		if s.tlsRoutesEnabled {
			return kind(kindTLSRoute)
		}
		return []gatewayv1.RouteGroupKind{}
	case gatewayv1.TCPProtocolType:
		if s.tcpRoutesEnabled {
			return kind(kindTCPRoute)
		}
		return []gatewayv1.RouteGroupKind{}
	}
	return nil
}

// checkCertificateRefs validates an HTTPS or terminating TLS listener's certificates: each must be a
// core Secret, allowed by a ReferenceGrant when in another namespace, and hold a usable key pair.
func (s *gatewayAPIState) checkCertificateRefs(gw *gatewayv1.Gateway, l gatewayv1.Listener) (gatewayv1.ListenerConditionReason, string, bool) {
	if l.TLS == nil || (l.TLS.Mode != nil && *l.TLS.Mode == gatewayv1.TLSModePassthrough) {
		return "", "", true
	}
	for _, ref := range l.TLS.CertificateRefs {
		group := ""
		if ref.Group != nil {
			group = string(*ref.Group)
		}
		kind := kindSecret
		if ref.Kind != nil {
			kind = string(*ref.Kind)
		}
		if group != "" || kind != kindSecret {
			return gatewayv1.ListenerReasonInvalidCertificateRef, fmt.Sprintf("certificateRef %s/%s is not a Secret", group, kind), false
		}
		ns := gw.Namespace
		if ref.Namespace != nil && *ref.Namespace != "" {
			ns = string(*ref.Namespace)
		}
		// Permission is decided before existence, so a reference that isn't allowed never reveals
		// whether the Secret exists.
		if !s.refAllowed(gw.Namespace, kindGateway, string(ref.Name), ns, "", kindSecret) {
			return gatewayv1.ListenerReasonRefNotPermitted, fmt.Sprintf("a ReferenceGrant is required to reference Secret %s/%s", ns, ref.Name), false
		}
		secret, err := s.getSecret(string(ref.Name), ns)
		if err != nil || secret == nil {
			return gatewayv1.ListenerReasonInvalidCertificateRef, fmt.Sprintf("Secret %s/%s not found", ns, ref.Name), false
		}
		if _, err := tls.X509KeyPair(secret.Data[corev1.TLSCertKey], secret.Data[corev1.TLSPrivateKeyKey]); err != nil {
			return gatewayv1.ListenerReasonInvalidCertificateRef, fmt.Sprintf("Secret %s/%s does not hold a valid certificate and key: %v", ns, ref.Name, err), false
		}
	}
	return "", "", true
}

func (s *gatewayAPIState) refAllowed(fromNamespace, fromKind, toName, toNamespace, toGroup, toKind string) bool {
	if s.disableRefGrants {
		return true
	}
	return referenceGrantAllows(s.referenceGrants, fromNamespace, gatewayAPIGroup, fromKind, toName, toNamespace, toGroup, toKind)
}

// referenceGrantAllows reports whether an object in fromNamespace may reference one in toNamespace.
// References within a namespace are always allowed.
func referenceGrantAllows(grants []*gatewayv1beta1.ReferenceGrant, fromNamespace, fromGroup, fromKind, toName, toNamespace, toGroup, toKind string) bool {
	if fromNamespace == toNamespace {
		return true
	}
	for _, grant := range grants {
		if grant.Namespace != toNamespace {
			continue
		}
		to := slices.ContainsFunc(grant.Spec.To, func(t gatewayv1beta1.ReferenceGrantTo) bool {
			return strings.EqualFold(string(t.Group), toGroup) && strings.EqualFold(string(t.Kind), toKind) &&
				(t.Name == nil || *t.Name == "" || string(*t.Name) == toName)
		})
		from := slices.ContainsFunc(grant.Spec.From, func(f gatewayv1beta1.ReferenceGrantFrom) bool {
			return strings.EqualFold(string(f.Group), fromGroup) && strings.EqualFold(string(f.Kind), fromKind) &&
				string(f.Namespace) == fromNamespace
		})
		if to && from {
			return true
		}
	}
	return false
}

// routeParents works out a route's status for each parentRef that names one of our Gateways, and
// counts it on the listeners it attaches to. hostnames are the route's spec hostnames.
func (s *gatewayAPIState) routeParents(
	routeNamespace string,
	routeKind string,
	generation int64,
	parentRefs []gatewayv1.ParentReference,
	hostnames []gatewayv1.Hostname,
	listeners map[types.NamespacedName]map[gatewayv1.SectionName]*listenerResult,
) []routeParentResult {
	results := []routeParentResult{}
	for _, ref := range parentRefs {
		if !parentRefIsGateway(ref) {
			continue
		}
		key := types.NamespacedName{Name: string(ref.Name), Namespace: routeNamespace}
		if ref.Namespace != nil && *ref.Namespace != "" {
			key.Namespace = string(*ref.Namespace)
		}
		gw, ok := s.getGateway(key)
		if !ok {
			// Not our Gateway, or it doesn't exist: the parent's status isn't ours to write.
			continue
		}

		res := routeParentResult{parentRef: normalizeParentRef(ref, key.Namespace)}
		res.accepted, res.listeners = s.attachRoute(gw, listeners[key], routeNamespace, routeKind, ref, hostnames, generation)
		for _, name := range res.listeners {
			listeners[key][name].attachedRoutes++
		}
		results = append(results, res)
	}
	return results
}

// attachRoute decides whether a route may attach to a Gateway through one parentRef, returning the
// Accepted condition and the listeners it attached to. The checks narrow the Gateway's listeners in the
// order the reasons are defined: those the parentRef selects (NoMatchingParent), those that allow the
// route (NotAllowedByListeners), then those sharing a hostname with it (NoMatchingListenerHostname).
func (s *gatewayAPIState) attachRoute(
	gw *gatewayv1.Gateway,
	listenerResults map[gatewayv1.SectionName]*listenerResult,
	routeNamespace, routeKind string,
	ref gatewayv1.ParentReference,
	hostnames []gatewayv1.Hostname,
	generation int64,
) (metav1.Condition, []gatewayv1.SectionName) {
	reject := func(reason gatewayv1.RouteConditionReason, msg string) (metav1.Condition, []gatewayv1.SectionName) {
		return condition(string(gatewayv1.RouteConditionAccepted), string(reason), metav1.ConditionFalse, generation, msg), nil
	}

	selected := []gatewayv1.Listener{}
	for _, l := range gw.Spec.Listeners {
		if ref.SectionName != nil && *ref.SectionName != l.Name {
			continue
		}
		if ref.Port != nil && *ref.Port != l.Port {
			continue
		}
		selected = append(selected, l)
	}
	if len(selected) == 0 {
		return reject(gatewayv1.RouteReasonNoMatchingParent, "no listener matches the parentRef's sectionName and port")
	}

	allowed := []gatewayv1.Listener{}
	for _, l := range selected {
		res := listenerResults[l.Name]
		if res == nil || res.accepted.Status != metav1.ConditionTrue {
			continue
		}
		if !slices.ContainsFunc(res.supportedKinds, func(k gatewayv1.RouteGroupKind) bool { return string(k.Kind) == routeKind }) {
			continue
		}
		if !s.namespaceAllowed(gw, l, routeNamespace) {
			continue
		}
		allowed = append(allowed, l)
	}
	if len(allowed) == 0 {
		return reject(gatewayv1.RouteReasonNotAllowedByListeners, "no listener the parentRef selects allows this route")
	}

	attached := []gatewayv1.SectionName{}
	for _, l := range allowed {
		if listenerMatchesRouteHostnames(l, hostnameStrings(hostnames)) {
			attached = append(attached, l.Name)
		}
	}
	if len(attached) == 0 {
		return reject(gatewayv1.RouteReasonNoMatchingListenerHostname, "none of the route's hostnames match a listener")
	}

	return condition(string(gatewayv1.RouteConditionAccepted), string(gatewayv1.RouteReasonAccepted), metav1.ConditionTrue, generation, "route accepted by the ngrok operator"), attached
}

func (s *gatewayAPIState) namespaceAllowed(gw *gatewayv1.Gateway, l gatewayv1.Listener, routeNamespace string) bool {
	from := gatewayv1.NamespacesFromSame
	if l.AllowedRoutes != nil && l.AllowedRoutes.Namespaces != nil && l.AllowedRoutes.Namespaces.From != nil {
		from = *l.AllowedRoutes.Namespaces.From
	}
	switch from {
	case gatewayv1.NamespacesFromAll:
		return true
	case gatewayv1.NamespacesFromSelector:
		if l.AllowedRoutes.Namespaces.Selector == nil {
			return false
		}
		selector, err := metav1.LabelSelectorAsSelector(l.AllowedRoutes.Namespaces.Selector)
		if err != nil {
			return false
		}
		ns, err := s.getNamespace(routeNamespace)
		if err != nil || ns == nil {
			return false
		}
		return selector.Matches(labels.Set(ns.Labels))
	default:
		return gw.Namespace == routeNamespace
	}
}

// httpRouteResolvedRefs checks every backendRef of an HTTPRoute: it must be a core Service, allowed by
// a ReferenceGrant when in another namespace, and exist. A rule without backendRefs is fine.
func (s *gatewayAPIState) httpRouteResolvedRefs(route *gatewayv1.HTTPRoute) metav1.Condition {
	gen := route.Generation
	for _, rule := range route.Spec.Rules {
		for _, ref := range rule.BackendRefs {
			if reason, msg, ok := s.checkBackendRef(route.Namespace, kindHTTPRoute, ref.BackendObjectReference); !ok {
				return condition(string(gatewayv1.RouteConditionResolvedRefs), string(reason), metav1.ConditionFalse, gen, msg)
			}
		}
	}
	return condition(string(gatewayv1.RouteConditionResolvedRefs), string(gatewayv1.RouteReasonResolvedRefs), metav1.ConditionTrue, gen, "all references resolved")
}

// checkBackendRef validates one backendRef. The same rules decide which backends the translator
// replaces with a 500 response.
func (s *gatewayAPIState) checkBackendRef(routeNamespace, routeKind string, ref gatewayv1.BackendObjectReference) (gatewayv1.RouteConditionReason, string, bool) {
	group := ""
	if ref.Group != nil {
		group = string(*ref.Group)
	}
	kind := kindService
	if ref.Kind != nil {
		kind = string(*ref.Kind)
	}
	if group != "" || kind != kindService {
		return gatewayv1.RouteReasonInvalidKind, fmt.Sprintf("backendRef %s/%s %q is not a Service", group, kind, ref.Name), false
	}
	ns := routeNamespace
	if ref.Namespace != nil && *ref.Namespace != "" {
		ns = string(*ref.Namespace)
	}
	if !s.refAllowed(routeNamespace, routeKind, string(ref.Name), ns, "", kindService) {
		return gatewayv1.RouteReasonRefNotPermitted, fmt.Sprintf("a ReferenceGrant is required to reference Service %s/%s", ns, ref.Name), false
	}
	if svc, err := s.getService(string(ref.Name), ns); err != nil || svc == nil {
		return gatewayv1.RouteReasonBackendNotFound, fmt.Sprintf("Service %s/%s not found", ns, ref.Name), false
	}
	return "", "", true
}

// gatewayStatus builds a Gateway's status from its listener results, keeping conditions other
// controllers or users wrote. existing is the Gateway's current status.
func (s *gatewayAPIState) gatewayStatus(gw *gatewayv1.Gateway, listeners map[gatewayv1.SectionName]*listenerResult) gatewayv1.GatewayStatus {
	gen := gw.Generation
	existing := gw.Status.DeepCopy()

	status := gatewayv1.GatewayStatus{
		Conditions: existing.Conditions,
		Listeners:  make([]gatewayv1.ListenerStatus, 0, len(gw.Spec.Listeners)),
	}

	previous := map[gatewayv1.SectionName]gatewayv1.ListenerStatus{}
	for _, l := range existing.Listeners {
		previous[l.Name] = l
	}

	accepted, invalid := 0, 0
	for _, l := range gw.Spec.Listeners {
		res := listeners[l.Name]
		// Start from the listener's previous conditions so foreign ones survive and unchanged ones
		// keep their transition times. Listeners removed from spec are dropped.
		ls := gatewayv1.ListenerStatus{
			Name:           l.Name,
			SupportedKinds: res.supportedKinds,
			AttachedRoutes: res.attachedRoutes,
			Conditions:     previous[l.Name].Conditions,
		}
		if ls.Conditions == nil {
			ls.Conditions = []metav1.Condition{}
		}
		meta.SetStatusCondition(&ls.Conditions, res.accepted)
		meta.SetStatusCondition(&ls.Conditions, res.resolvedRefs)

		programmed := condition(string(gatewayv1.ListenerConditionProgrammed), string(gatewayv1.ListenerReasonProgrammed), metav1.ConditionTrue, gen, "listener programmed")
		if res.accepted.Status != metav1.ConditionTrue || res.resolvedRefs.Status != metav1.ConditionTrue {
			programmed = condition(string(gatewayv1.ListenerConditionProgrammed), string(gatewayv1.ListenerReasonInvalid), metav1.ConditionFalse, gen, "listener is not valid")
		}
		meta.SetStatusCondition(&ls.Conditions, programmed)
		status.Listeners = append(status.Listeners, ls)

		if res.accepted.Status == metav1.ConditionTrue {
			accepted++
		} else {
			invalid++
		}
	}

	// Accepted when at least one listener is; ListenersNotValid flags any listener that isn't, even
	// when the Gateway is accepted.
	gwAccepted := condition(string(gatewayv1.GatewayConditionAccepted), string(gatewayv1.GatewayReasonAccepted), metav1.ConditionTrue, gen, "gateway accepted by the ngrok operator")
	switch {
	case accepted == 0:
		gwAccepted = condition(string(gatewayv1.GatewayConditionAccepted), string(gatewayv1.GatewayReasonListenersNotValid), metav1.ConditionFalse, gen, "no listener is valid")
	case invalid > 0:
		gwAccepted = condition(string(gatewayv1.GatewayConditionAccepted), string(gatewayv1.GatewayReasonListenersNotValid), metav1.ConditionTrue, gen, "some listeners are not valid")
	}
	meta.SetStatusCondition(&status.Conditions, gwAccepted)

	for _, addr := range s.addresses(gw) {
		status.Addresses = append(status.Addresses, gatewayv1.GatewayStatusAddress{
			Type:  ptr.To(gatewayv1.HostnameAddressType),
			Value: addr,
		})
	}

	gwProgrammed := condition(string(gatewayv1.GatewayConditionProgrammed), string(gatewayv1.GatewayReasonProgrammed), metav1.ConditionTrue, gen, "gateway programmed")
	switch {
	case gwAccepted.Status != metav1.ConditionTrue:
		gwProgrammed = condition(string(gatewayv1.GatewayConditionProgrammed), string(gatewayv1.GatewayReasonInvalid), metav1.ConditionFalse, gen, "gateway is not accepted")
	case len(status.Addresses) == 0:
		gwProgrammed = condition(string(gatewayv1.GatewayConditionProgrammed), string(gatewayv1.GatewayReasonAddressNotAssigned), metav1.ConditionFalse, gen, "waiting for the Gateway's domains to be reserved")
	}
	meta.SetStatusCondition(&status.Conditions, gwProgrammed)

	return status
}

// routeParentStatuses merges our results for a route into its existing parent statuses: entries
// written by other controllers are kept as they are, and ours are replaced. Conditions of an entry we
// keep writing carry over, so their transition times only move when they change.
func (s *gatewayAPIState) routeParentStatuses(existing []gatewayv1.RouteParentStatus, results []routeParentResult, resolvedRefs metav1.Condition) []gatewayv1.RouteParentStatus {
	out := []gatewayv1.RouteParentStatus{}
	ours := map[string][]metav1.Condition{}
	for _, p := range existing {
		if p.ControllerName != s.controllerName {
			out = append(out, p)
			continue
		}
		ours[parentRefKey(p.ParentRef)] = p.Conditions
	}
	for _, r := range results {
		conditions := slices.Clone(ours[parentRefKey(r.parentRef)])
		if conditions == nil {
			conditions = []metav1.Condition{}
		}
		meta.SetStatusCondition(&conditions, r.accepted)
		meta.SetStatusCondition(&conditions, resolvedRefs)
		out = append(out, gatewayv1.RouteParentStatus{
			ParentRef:      r.parentRef,
			ControllerName: s.controllerName,
			Conditions:     conditions,
		})
	}
	return out
}

// normalizeParentRef fills in the defaults of a parentRef, so status names its Gateway in full.
func normalizeParentRef(ref gatewayv1.ParentReference, namespace string) gatewayv1.ParentReference {
	ref = *ref.DeepCopy()
	ref.Group = ptr.To(gatewayv1.Group(gatewayAPIGroup))
	ref.Kind = ptr.To(gatewayv1.Kind(kindGateway))
	ref.Namespace = new(gatewayv1.Namespace(namespace))
	return ref
}

func parentRefKey(ref gatewayv1.ParentReference) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s/%d",
		ptr.Deref(ref.Group, gatewayAPIGroup), ptr.Deref(ref.Kind, kindGateway),
		ptr.Deref(ref.Namespace, ""), ref.Name, ptr.Deref(ref.SectionName, ""), ptr.Deref(ref.Port, 0))
}

func parentRefIsGateway(ref gatewayv1.ParentReference) bool {
	return ptr.Deref(ref.Group, gatewayAPIGroup) == gatewayAPIGroup && ptr.Deref(ref.Kind, kindGateway) == kindGateway
}

func hostnameStrings(hostnames []gatewayv1.Hostname) []string {
	out := make([]string, 0, len(hostnames))
	for _, h := range hostnames {
		out = append(out, string(h))
	}
	return out
}
