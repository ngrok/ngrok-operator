package agent

import (
	"sync"

	"golang.ngrok.com/ngrok/v2"
)

type endpointForwarderMap struct {
	m  map[string]ngrok.EndpointForwarder
	mu sync.Mutex
}

func newEndpointForwarderMap() *endpointForwarderMap {
	return &endpointForwarderMap{
		m: make(map[string]ngrok.EndpointForwarder),
	}
}

func (a *endpointForwarderMap) Add(name string, ep ngrok.EndpointForwarder) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.m[name] = ep
}

func (a *endpointForwarderMap) Get(name string) (ngrok.EndpointForwarder, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	ep, ok := a.m[name]
	return ep, ok
}

func (a *endpointForwarderMap) Delete(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.m, name)
}
