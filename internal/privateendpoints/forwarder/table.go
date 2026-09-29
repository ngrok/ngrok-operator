// Package forwarder serves in-cluster DNS and connection forwarding for
// ngrok private endpoints.
package forwarder

import (
	"net"
	"strconv"
	"sync"

	pe "github.com/ngrok/ngrok-operator/internal/privateendpoints"
)

// Entry is the forwarder's view of one Ready PrivateEndpoint.
type Entry struct {
	Hostname      string
	Port          int32
	ClusterIP     string
	ForwarderPort int32 // 0 when served by the shared listener
}

type hostPort struct {
	host string
	port int32
}

// Table maps names and ports to dial targets. Replace swaps it wholesale.
type Table struct {
	mu     sync.RWMutex
	synced bool
	ips    map[string]string
	shared map[hostPort]string
	ports  map[int32]string
}

func NewTable() *Table { return &Table{} }

func (t *Table) Replace(entries []Entry) {
	ips := map[string]string{}
	shared := map[hostPort]string{}
	ports := map[int32]string{}
	for _, e := range entries {
		if e.ClusterIP == "" {
			continue
		}
		host := pe.NormalizeHost(e.Hostname)
		target := net.JoinHostPort(host, strconv.Itoa(int(e.Port)))
		if _, ok := ips[host]; !ok {
			ips[host] = e.ClusterIP
		}
		if e.ForwarderPort != 0 {
			ports[e.ForwarderPort] = target
		} else {
			shared[hostPort{host, e.Port}] = target
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ips, t.shared, t.ports, t.synced = ips, shared, ports, true
}

func (t *Table) Synced() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.synced
}

func (t *Table) IP(name string) (string, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	ip, ok := t.ips[pe.NormalizeHost(name)]
	return ip, ok
}

func (t *Table) SharedTarget(host string, port int32) (string, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	target, ok := t.shared[hostPort{pe.NormalizeHost(host), port}]
	return target, ok
}

func (t *Table) PortTarget(port int32) (string, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	target, ok := t.ports[port]
	return target, ok
}
