package managerdriver

import (
	"errors"
	"testing"

	"github.com/ngrok/ngrok-operator/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

const testController = gatewayv1.GatewayController("ngrok.com/gateway-controller")

var errNotFound = errors.New("not found")

// testState is a gatewayAPIState over in-memory objects.
type testState struct {
	namespaces map[string]*corev1.Namespace
	services   map[types.NamespacedName]*corev1.Service
	secrets    map[types.NamespacedName]*corev1.Secret
	gateways   map[types.NamespacedName]*gatewayv1.Gateway
	grants     []*gatewayv1beta1.ReferenceGrant
	addresses  []string
	hostnames  GatewayHostnames
}

func (ts testState) state() *gatewayAPIState {
	return &gatewayAPIState{
		controllerName:  testController,
		hostnames:       ts.hostnames,
		referenceGrants: ts.grants,
		getNamespace: func(name string) (*corev1.Namespace, error) {
			if ns, ok := ts.namespaces[name]; ok {
				return ns, nil
			}
			return nil, errNotFound
		},
		getService: func(name, namespace string) (*corev1.Service, error) {
			if svc, ok := ts.services[types.NamespacedName{Name: name, Namespace: namespace}]; ok {
				return svc, nil
			}
			return nil, errNotFound
		},
		getSecret: func(name, namespace string) (*corev1.Secret, error) {
			if s, ok := ts.secrets[types.NamespacedName{Name: name, Namespace: namespace}]; ok {
				return s, nil
			}
			return nil, errNotFound
		},
		getGateway: func(key types.NamespacedName) (*gatewayv1.Gateway, bool) {
			gw, ok := ts.gateways[key]
			return gw, ok
		},
		addresses: func(*gatewayv1.Gateway) []string { return ts.addresses },
	}
}

func httpListener(name, hostname string) gatewayv1.Listener {
	l := gatewayv1.Listener{Name: gatewayv1.SectionName(name), Port: 80, Protocol: gatewayv1.HTTPProtocolType}
	if hostname != "" {
		l.Hostname = new(gatewayv1.Hostname(hostname))
	}
	return l
}

func httpsListener(name string, refs ...gatewayv1.SecretObjectReference) gatewayv1.Listener {
	return gatewayv1.Listener{
		Name: gatewayv1.SectionName(name), Port: 443, Protocol: gatewayv1.HTTPSProtocolType,
		Hostname: ptr.To(gatewayv1.Hostname("secure.example.com")),
		TLS:      &gatewayv1.ListenerTLSConfig{CertificateRefs: refs},
	}
}

func testGateway(listeners ...gatewayv1.Listener) *gatewayv1.Gateway {
	return &gatewayv1.Gateway{
		Name: "gw", Namespace: "infra", Generation: 3, UID: "0b2f6c1e-1111",
		Spec: gatewayv1.GatewaySpec{GatewayClassName: "ngrok", Listeners: listeners},
	}
}

func refGrant(namespace, fromKind, fromNamespace, toKind string, toName string) *gatewayv1beta1.ReferenceGrant {
	g := &gatewayv1beta1.ReferenceGrant{
		Name: "grant", Namespace: namespace,
		Spec: gatewayv1beta1.ReferenceGrantSpec{
			From: []gatewayv1beta1.ReferenceGrantFrom{{Group: gatewayv1.GroupName, Kind: gatewayv1.Kind(fromKind), Namespace: gatewayv1.Namespace(fromNamespace)}},
			To:   []gatewayv1beta1.ReferenceGrantTo{{Group: "", Kind: gatewayv1.Kind(toKind)}},
		},
	}
	if toName != "" {
		g.Spec.To[0].Name = new(gatewayv1.ObjectName(toName))
	}
	return g
}

func TestListenerResult(t *testing.T) {
	secretRef := func(name, namespace string) gatewayv1.SecretObjectReference {
		r := gatewayv1.SecretObjectReference{Name: gatewayv1.ObjectName(name)}
		if namespace != "" {
			r.Namespace = new(gatewayv1.Namespace(namespace))
		}
		return r
	}
	malformed := &corev1.Secret{
		Name: "malformed", Namespace: "infra",
		Data: map[string][]byte{corev1.TLSCertKey: []byte("Hello world"), corev1.TLSPrivateKeyKey: []byte("Hello world")},
	}

	tests := []struct {
		name            string
		ts              testState
		listener        gatewayv1.Listener
		wantAccepted    string
		wantResolved    string
		wantKinds       []string
		wantKindsIsZero bool
	}{
		{
			name:         "valid HTTP listener",
			listener:     httpListener("http", "a.example.com"),
			wantAccepted: "Accepted", wantResolved: "ResolvedRefs", wantKinds: []string{"HTTPRoute"},
		},
		{
			name:         "unsupported protocol has no supported kinds",
			listener:     gatewayv1.Listener{Name: "invalid", Port: 1111, Protocol: "INVALID"},
			wantAccepted: "UnsupportedProtocol", wantResolved: "ResolvedRefs", wantKindsIsZero: true,
		},
		{
			name:         "HTTP must be on port 80",
			listener:     gatewayv1.Listener{Name: "http", Port: 8080, Protocol: gatewayv1.HTTPProtocolType, Hostname: ptr.To(gatewayv1.Hostname("a.com"))},
			wantAccepted: "PortUnavailable", wantResolved: "ResolvedRefs", wantKinds: []string{"HTTPRoute"},
		},
		{
			name:         "no hostname and no generated suffix",
			listener:     httpListener("http", ""),
			wantAccepted: "HostnameRequired", wantResolved: "ResolvedRefs", wantKinds: []string{"HTTPRoute"},
		},
		{
			name:         "no hostname with a generated suffix",
			ts:           testState{hostnames: GatewayHostnames{GeneratedSuffix: "ngrok.app"}},
			listener:     httpListener("http", ""),
			wantAccepted: "Accepted", wantResolved: "ResolvedRefs", wantKinds: []string{"HTTPRoute"},
		},
		{
			name: "only invalid route kinds",
			listener: func() gatewayv1.Listener {
				l := httpListener("http", "a.com")
				l.AllowedRoutes = &gatewayv1.AllowedRoutes{Kinds: []gatewayv1.RouteGroupKind{{Kind: "InvalidRoute"}}}
				return l
			}(),
			wantAccepted: "Accepted", wantResolved: "InvalidRouteKinds", wantKindsIsZero: true,
		},
		{
			name: "valid and invalid route kinds keeps the valid one",
			listener: func() gatewayv1.Listener {
				l := httpListener("http", "a.com")
				l.AllowedRoutes = &gatewayv1.AllowedRoutes{Kinds: []gatewayv1.RouteGroupKind{{Kind: "InvalidRoute"}, {Kind: "HTTPRoute"}}}
				return l
			}(),
			wantAccepted: "Accepted", wantResolved: "InvalidRouteKinds", wantKinds: []string{"HTTPRoute"},
		},
		{
			name:         "certificate in a missing Secret",
			listener:     httpsListener("https", secretRef("missing", "")),
			wantAccepted: "Accepted", wantResolved: "InvalidCertificateRef", wantKinds: []string{"HTTPRoute"},
		},
		{
			name:         "certificate ref of the wrong kind",
			listener:     httpsListener("https", gatewayv1.SecretObjectReference{Name: "x", Kind: ptr.To(gatewayv1.Kind("WrongKind"))}),
			wantAccepted: "Accepted", wantResolved: "InvalidCertificateRef", wantKinds: []string{"HTTPRoute"},
		},
		{
			name:         "certificate ref of the wrong group",
			listener:     httpsListener("https", gatewayv1.SecretObjectReference{Name: "x", Group: ptr.To(gatewayv1.Group("wrong.group.io"))}),
			wantAccepted: "Accepted", wantResolved: "InvalidCertificateRef", wantKinds: []string{"HTTPRoute"},
		},
		{
			name:         "malformed certificate",
			ts:           testState{secrets: map[types.NamespacedName]*corev1.Secret{{Name: "malformed", Namespace: "infra"}: malformed}},
			listener:     httpsListener("https", secretRef("malformed", "")),
			wantAccepted: "Accepted", wantResolved: "InvalidCertificateRef", wantKinds: []string{"HTTPRoute"},
		},
		{
			name:         "certificate in another namespace without a grant, even when missing",
			listener:     httpsListener("https", secretRef("missing", "other")),
			wantAccepted: "Accepted", wantResolved: "RefNotPermitted", wantKinds: []string{"HTTPRoute"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := tt.ts.state().listenerResult(testGateway(tt.listener), tt.listener)
			assert.Equal(t, tt.wantAccepted, res.accepted.Reason)
			assert.Equal(t, tt.wantAccepted == "Accepted", res.accepted.Status == metav1.ConditionTrue)
			assert.Equal(t, tt.wantResolved, res.resolvedRefs.Reason)
			assert.Equal(t, int64(3), res.accepted.ObservedGeneration)
			if tt.wantKindsIsZero {
				assert.NotNil(t, res.supportedKinds)
				assert.Empty(t, res.supportedKinds)
			} else {
				kinds := []string{}
				for _, k := range res.supportedKinds {
					kinds = append(kinds, string(k.Kind))
				}
				assert.Equal(t, tt.wantKinds, kinds)
			}
		})
	}

	t.Run("valid certificates, local and granted", func(t *testing.T) {
		ts := testState{
			secrets: map[types.NamespacedName]*corev1.Secret{
				{Name: "cert", Namespace: "infra"}: testutils.NewTLSSecret("cert", "infra"),
				{Name: "cert", Namespace: "other"}: testutils.NewTLSSecret("cert", "other"),
			},
			grants: []*gatewayv1beta1.ReferenceGrant{refGrant("other", "Gateway", "infra", "Secret", "cert")},
		}
		for _, l := range []gatewayv1.Listener{httpsListener("local", secretRef("cert", "")), httpsListener("granted", secretRef("cert", "other"))} {
			res := ts.state().listenerResult(testGateway(l), l)
			assert.Equal(t, "ResolvedRefs", res.resolvedRefs.Reason, l.Name)
		}
	})
}

func TestReferenceGrantAllows(t *testing.T) {
	right := refGrant("backend", "HTTPRoute", "infra", "Service", "")
	tests := []struct {
		name  string
		grant *gatewayv1beta1.ReferenceGrant
		want  bool
	}{
		{"matching grant for any name", right, true},
		{"matching grant for the name", refGrant("backend", "HTTPRoute", "infra", "Service", "web"), true},
		{"grant for another name", refGrant("backend", "HTTPRoute", "infra", "Service", "other"), false},
		{"grant in the wrong namespace", refGrant("infra", "HTTPRoute", "infra", "Service", ""), false},
		{"grant from another kind", refGrant("backend", "Gateway", "infra", "Service", ""), false},
		{"grant from another namespace", refGrant("backend", "HTTPRoute", "elsewhere", "Service", ""), false},
		{"grant to another kind", refGrant("backend", "HTTPRoute", "infra", "Secret", ""), false},
		{
			"grant from another group",
			func() *gatewayv1beta1.ReferenceGrant {
				g := right.DeepCopy()
				g.Spec.From[0].Group = "wrong.group.io"
				return g
			}(),
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := referenceGrantAllows([]*gatewayv1beta1.ReferenceGrant{tt.grant}, "infra", gatewayv1.GroupName, "HTTPRoute", "web", "backend", "", "Service")
			assert.Equal(t, tt.want, got)
		})
	}
	assert.True(t, referenceGrantAllows(nil, "infra", gatewayv1.GroupName, "HTTPRoute", "web", "infra", "", "Service"), "same namespace needs no grant")
}

func TestRouteParents(t *testing.T) {
	fromAll := func(l gatewayv1.Listener) gatewayv1.Listener {
		l.AllowedRoutes = &gatewayv1.AllowedRoutes{Namespaces: &gatewayv1.RouteNamespaces{From: ptr.To(gatewayv1.NamespacesFromAll)}}
		return l
	}
	fromSelector := func(l gatewayv1.Listener) gatewayv1.Listener {
		l.AllowedRoutes = &gatewayv1.AllowedRoutes{Namespaces: &gatewayv1.RouteNamespaces{
			From:     ptr.To(gatewayv1.NamespacesFromSelector),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "web"}},
		}}
		return l
	}
	ref := func(sectionName string) gatewayv1.ParentReference {
		r := gatewayv1.ParentReference{Name: "gw", Namespace: ptr.To(gatewayv1.Namespace("infra"))}
		if sectionName != "" {
			r.SectionName = new(gatewayv1.SectionName(sectionName))
		}
		return r
	}

	tests := []struct {
		name           string
		gw             *gatewayv1.Gateway
		routeNamespace string
		parentRef      gatewayv1.ParentReference
		hostnames      []gatewayv1.Hostname
		wantReason     string
		wantAttached   map[gatewayv1.SectionName]int32
	}{
		{
			name:           "accepted on every listener it matches",
			gw:             testGateway(httpListener("a", "a.example.com"), httpListener("b", "*.example.com"), httpListener("c", "other.com")),
			routeNamespace: "infra", parentRef: ref(""),
			hostnames:    []gatewayv1.Hostname{"a.example.com"},
			wantReason:   "Accepted",
			wantAttached: map[gatewayv1.SectionName]int32{"a": 1, "b": 1, "c": 0},
		},
		{
			name:           "sectionName matching no listener",
			gw:             testGateway(httpListener("http", "a.com")),
			routeNamespace: "infra", parentRef: ref("http1"),
			wantReason:   "NoMatchingParent",
			wantAttached: map[gatewayv1.SectionName]int32{"http": 0},
		},
		{
			name:           "route in another namespace on a Same listener",
			gw:             testGateway(httpListener("http", "a.com")),
			routeNamespace: "web", parentRef: ref(""),
			wantReason:   "NotAllowedByListeners",
			wantAttached: map[gatewayv1.SectionName]int32{"http": 0},
		},
		{
			name:           "route in another namespace on an All listener",
			gw:             testGateway(fromAll(httpListener("http", "a.com"))),
			routeNamespace: "web", parentRef: ref(""),
			wantReason:   "Accepted",
			wantAttached: map[gatewayv1.SectionName]int32{"http": 1},
		},
		{
			name:           "namespace selector matches the route's namespace labels",
			gw:             testGateway(fromSelector(httpListener("http", "a.com"))),
			routeNamespace: "web", parentRef: ref(""),
			wantReason:   "Accepted",
			wantAttached: map[gatewayv1.SectionName]int32{"http": 1},
		},
		{
			name:           "namespace selector not matching",
			gw:             testGateway(fromSelector(httpListener("http", "a.com"))),
			routeNamespace: "infra", parentRef: ref(""),
			wantReason:   "NotAllowedByListeners",
			wantAttached: map[gatewayv1.SectionName]int32{"http": 0},
		},
		{
			name:           "no shared hostname",
			gw:             testGateway(httpListener("http", "very.specific.com")),
			routeNamespace: "infra", parentRef: ref(""),
			hostnames:    []gatewayv1.Hostname{"specific.but.wrong.com", "wildcard.io"},
			wantReason:   "NoMatchingListenerHostname",
			wantAttached: map[gatewayv1.SectionName]int32{"http": 0},
		},
		{
			name:           "listener with an invalid certificate still attaches routes",
			gw:             testGateway(httpsListener("tls", gatewayv1.SecretObjectReference{Name: "does-not-exist"})),
			routeNamespace: "infra", parentRef: ref("tls"),
			wantReason:   "Accepted",
			wantAttached: map[gatewayv1.SectionName]int32{"tls": 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := types.NamespacedName{Name: "gw", Namespace: "infra"}
			ts := testState{
				gateways: map[types.NamespacedName]*gatewayv1.Gateway{key: tt.gw},
				namespaces: map[string]*corev1.Namespace{
					"web":   {ObjectMeta: metav1.ObjectMeta{Name: "web", Labels: map[string]string{"team": "web"}}},
					"infra": {ObjectMeta: metav1.ObjectMeta{Name: "infra"}},
				},
			}
			s := ts.state()
			listeners := map[types.NamespacedName]map[gatewayv1.SectionName]*listenerResult{key: s.listenerResults(tt.gw)}

			results := s.routeParents(tt.routeNamespace, kindHTTPRoute, 7, []gatewayv1.ParentReference{tt.parentRef}, tt.hostnames, listeners)
			require.Len(t, results, 1)
			assert.Equal(t, tt.wantReason, results[0].accepted.Reason)
			assert.Equal(t, int64(7), results[0].accepted.ObservedGeneration)
			assert.Equal(t, "infra", string(*results[0].parentRef.Namespace))
			assert.Equal(t, gatewayv1.GroupName, string(*results[0].parentRef.Group))
			assert.Equal(t, "Gateway", string(*results[0].parentRef.Kind))
			for name, want := range tt.wantAttached {
				assert.Equal(t, want, listeners[key][name].attachedRoutes, "attachedRoutes on %s", name)
			}
		})
	}

	t.Run("parentRefs to Gateways that aren't ours get no status", func(t *testing.T) {
		s := testState{}.state()
		results := s.routeParents("infra", kindHTTPRoute, 1, []gatewayv1.ParentReference{{Name: "unmanaged-gateway"}}, nil, nil)
		assert.Empty(t, results)
	})
}

func TestHTTPRouteResolvedRefs(t *testing.T) {
	svc := &corev1.Service{Name: "web", Namespace: "backend"}
	local := &corev1.Service{Name: "web", Namespace: "infra"}
	route := func(refs ...gatewayv1.HTTPBackendRef) *gatewayv1.HTTPRoute {
		return &gatewayv1.HTTPRoute{
			Name: "r", Namespace: "infra", Generation: 2,
			Spec: gatewayv1.HTTPRouteSpec{Rules: []gatewayv1.HTTPRouteRule{{BackendRefs: refs}}},
		}
	}
	backend := func(name, namespace string) gatewayv1.HTTPBackendRef {
		b := gatewayv1.HTTPBackendRef{Name: gatewayv1.ObjectName(name), Port: ptr.To(gatewayv1.PortNumber(8080))}
		if namespace != "" {
			b.Namespace = new(gatewayv1.Namespace(namespace))
		}
		return b
	}
	unknownKind := backend("x", "")
	unknownKind.Group = ptr.To(gatewayv1.Group("unknownkind.example.com"))
	unknownKind.Kind = ptr.To(gatewayv1.Kind("NonExistent"))

	ts := testState{services: map[types.NamespacedName]*corev1.Service{
		{Name: "web", Namespace: "backend"}: svc,
		{Name: "web", Namespace: "infra"}:   local,
	}}
	tests := []struct {
		name   string
		grants []*gatewayv1beta1.ReferenceGrant
		route  *gatewayv1.HTTPRoute
		want   string
	}{
		{"local Service", nil, route(backend("web", "")), "ResolvedRefs"},
		{"no backendRefs", nil, route(), "ResolvedRefs"},
		{"missing Service", nil, route(backend("nonexistent", "")), "BackendNotFound"},
		{"not a Service", nil, route(unknownKind), "InvalidKind"},
		{"other namespace without a grant", nil, route(backend("web", "backend")), "RefNotPermitted"},
		{"other namespace with a grant", []*gatewayv1beta1.ReferenceGrant{refGrant("backend", "HTTPRoute", "infra", "Service", "web")}, route(backend("web", "backend")), "ResolvedRefs"},
		{"one bad backend among good ones", nil, route(backend("web", ""), backend("nonexistent", "")), "BackendNotFound"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts.grants = tt.grants
			c := ts.state().httpRouteResolvedRefs(tt.route)
			assert.Equal(t, tt.want, c.Reason)
			assert.Equal(t, tt.want == "ResolvedRefs", c.Status == metav1.ConditionTrue)
			assert.Equal(t, int64(2), c.ObservedGeneration)
		})
	}
}

func TestGatewayStatus(t *testing.T) {
	foreign := metav1.Condition{Type: "example.com/Foreign", Status: metav1.ConditionTrue, Reason: "Seeded", ObservedGeneration: 1, LastTransitionTime: metav1.Unix(100, 0)}
	invalid := gatewayv1.Listener{Name: "invalid", Port: 1111, Protocol: "INVALID"}

	tests := []struct {
		name           string
		listeners      []gatewayv1.Listener
		addresses      []string
		wantAccepted   metav1.ConditionStatus
		wantReason     string
		wantProgrammed metav1.ConditionStatus
	}{
		{"all listeners valid", []gatewayv1.Listener{httpListener("http", "a.com")}, []string{"a.com"}, metav1.ConditionTrue, "Accepted", metav1.ConditionTrue},
		{"some listeners invalid", []gatewayv1.Listener{httpListener("http", "a.com"), invalid}, []string{"a.com"}, metav1.ConditionTrue, "ListenersNotValid", metav1.ConditionTrue},
		{"no listener valid", []gatewayv1.Listener{invalid}, nil, metav1.ConditionFalse, "ListenersNotValid", metav1.ConditionFalse},
		{"no address yet", []gatewayv1.Listener{httpListener("http", "a.com")}, nil, metav1.ConditionTrue, "Accepted", metav1.ConditionFalse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := testGateway(tt.listeners...)
			gw.Status.Conditions = []metav1.Condition{foreign}
			gw.Status.Listeners = []gatewayv1.ListenerStatus{
				{Name: "http", Conditions: []metav1.Condition{foreign}},
				{Name: "removed", Conditions: []metav1.Condition{foreign}},
			}
			s := testState{addresses: tt.addresses}.state()
			listeners := s.listenerResults(gw)
			status := s.gatewayStatus(gw, listeners)

			accepted := meta.FindStatusCondition(status.Conditions, "Accepted")
			require.NotNil(t, accepted)
			assert.Equal(t, tt.wantAccepted, accepted.Status)
			assert.Equal(t, tt.wantReason, accepted.Reason)
			programmed := meta.FindStatusCondition(status.Conditions, "Programmed")
			require.NotNil(t, programmed)
			assert.Equal(t, tt.wantProgrammed, programmed.Status)
			for _, c := range status.Conditions {
				if c.Type != foreign.Type {
					assert.Equal(t, int64(3), c.ObservedGeneration, c.Type)
				}
			}

			assert.Contains(t, status.Conditions, foreign, "foreign Gateway condition kept as is")
			assert.Len(t, status.Listeners, len(tt.listeners), "removed listeners are dropped")
			for _, l := range status.Listeners {
				if l.Name == "http" {
					assert.Contains(t, l.Conditions, foreign, "foreign listener condition kept as is")
				}
				for _, typ := range []string{"Accepted", "ResolvedRefs", "Programmed"} {
					assert.NotNil(t, meta.FindStatusCondition(l.Conditions, typ), "%s on listener %s", typ, l.Name)
				}
			}
			for _, a := range status.Addresses {
				assert.Equal(t, gatewayv1.HostnameAddressType, *a.Type)
			}
		})
	}
}

func TestRouteParentStatuses(t *testing.T) {
	s := testState{}.state()
	parentRef := normalizeParentRef(gatewayv1.ParentReference{Name: "gw"}, "infra")
	transition := metav1.Unix(100, 0)
	foreignEntry := gatewayv1.RouteParentStatus{
		ParentRef:      parentRef,
		ControllerName: "example.com/other-controller",
		Conditions:     []metav1.Condition{{Type: "Accepted", Status: metav1.ConditionTrue, Reason: "Accepted", LastTransitionTime: transition}},
	}
	staleOurs := gatewayv1.RouteParentStatus{
		ParentRef:      parentRef,
		ControllerName: testController,
		Conditions:     []metav1.Condition{{Type: "Accepted", Status: metav1.ConditionTrue, Reason: "Accepted", ObservedGeneration: 1, LastTransitionTime: transition}},
	}
	goneOurs := gatewayv1.RouteParentStatus{
		ParentRef:      normalizeParentRef(gatewayv1.ParentReference{Name: "removed-from-spec"}, "infra"),
		ControllerName: testController,
	}

	accepted := condition("Accepted", "Accepted", metav1.ConditionTrue, 2, "")
	resolved := condition("ResolvedRefs", "ResolvedRefs", metav1.ConditionTrue, 2, "")
	parents := s.routeParentStatuses([]gatewayv1.RouteParentStatus{foreignEntry, staleOurs, goneOurs}, []routeParentResult{{parentRef: parentRef, accepted: accepted}}, resolved)

	require.Len(t, parents, 2)
	assert.Equal(t, foreignEntry, parents[0], "another controller's entry for the same parentRef is kept as is")
	ours := parents[1]
	assert.Equal(t, testController, ours.ControllerName)
	got := meta.FindStatusCondition(ours.Conditions, "Accepted")
	require.NotNil(t, got)
	assert.Equal(t, int64(2), got.ObservedGeneration)
	assert.Equal(t, transition, got.LastTransitionTime, "an unchanged condition keeps its transition time")
	assert.NotNil(t, meta.FindStatusCondition(ours.Conditions, "ResolvedRefs"))
	assert.Equal(t, int64(1), staleOurs.Conditions[0].ObservedGeneration, "the existing status is not modified in place")
}
