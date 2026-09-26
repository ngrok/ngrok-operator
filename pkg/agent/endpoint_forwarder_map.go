package agent

import (
	"bytes"
	"crypto/tls"
	"reflect"
	"slices"
	"sync"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"golang.ngrok.com/ngrok/v2"
)

// endpointConfig is the desired state a forwarder was created from. It is
// kept so an unchanged reconcile can skip rebinding the tunnel.
type endpointConfig struct {
	spec          ngrokv1alpha1.AgentEndpointSpec
	trafficPolicy string
	clientCerts   []tls.Certificate
	agentTLS      *AgentTLSTermination
}

func (c endpointConfig) equal(o endpointConfig) bool {
	return reflect.DeepEqual(c.spec, o.spec) &&
		c.trafficPolicy == o.trafficPolicy &&
		slices.EqualFunc(c.clientCerts, o.clientCerts, certEqual) &&
		agentTLSEqual(c.agentTLS, o.agentTLS)
}

func certEqual(a, b tls.Certificate) bool {
	return slices.EqualFunc(a.Certificate, b.Certificate, bytes.Equal)
}

func agentTLSEqual(a, b *AgentTLSTermination) bool {
	if a == nil || b == nil {
		return a == b
	}
	if (a.ServerCert == nil) != (b.ServerCert == nil) {
		return false
	}
	if a.ServerCert != nil && !certEqual(*a.ServerCert, *b.ServerCert) {
		return false
	}
	if (a.ClientCAs == nil) != (b.ClientCAs == nil) {
		return false
	}
	if a.ClientCAs != nil && !a.ClientCAs.Equal(b.ClientCAs) {
		return false
	}
	return a.ClientAuth == b.ClientAuth
}

type endpointForwarderMap struct {
	m       map[string]ngrok.EndpointForwarder
	configs map[string]endpointConfig
	mu      sync.Mutex
}

func newEndpointForwarderMap() *endpointForwarderMap {
	return &endpointForwarderMap{
		m:       make(map[string]ngrok.EndpointForwarder),
		configs: make(map[string]endpointConfig),
	}
}

func (a *endpointForwarderMap) Add(name string, ep ngrok.EndpointForwarder, cfg endpointConfig) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.m[name] = ep
	a.configs[name] = cfg
}

func (a *endpointForwarderMap) Get(name string) (ngrok.EndpointForwarder, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	ep, ok := a.m[name]
	return ep, ok
}

// Config returns the desired state the named forwarder was created from.
func (a *endpointForwarderMap) Config(name string) (endpointConfig, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cfg, ok := a.configs[name]
	return cfg, ok
}

func (a *endpointForwarderMap) Delete(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.m, name)
	delete(a.configs, name)
}
