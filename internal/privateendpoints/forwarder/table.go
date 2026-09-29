// Package forwarder forwards in-cluster connections to ngrok private
// endpoints over private dial.
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
	ForwarderPort int32
}

// Table maps forwarder ports to dial targets. Replace swaps it wholesale.
type Table struct {
	mu     sync.RWMutex
	synced bool
	ports  map[int32]string
}

func NewTable() *Table { return &Table{} }

func (t *Table) Replace(entries []Entry) {
	ports := map[int32]string{}
	for _, e := range entries {
		ports[e.ForwarderPort] = net.JoinHostPort(pe.NormalizeHost(e.Hostname), strconv.Itoa(int(e.Port)))
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ports, t.synced = ports, true
}

func (t *Table) Synced() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.synced
}

func (t *Table) PortTarget(port int32) (string, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	target, ok := t.ports[port]
	return target, ok
}
