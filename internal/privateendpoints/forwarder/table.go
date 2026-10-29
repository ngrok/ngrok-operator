// Package forwarder forwards in-cluster connections to ngrok private
// endpoints over private dial.
package forwarder

import (
	"net"
	"slices"
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

// Replace swaps in a table built from entries and returns the ports claimed
// by more than one entry, sorted. Those resolve to nothing: an ambiguous port
// fails rather than guessing a target.
func (t *Table) Replace(entries []Entry) []int32 {
	claims := map[int32]int{}
	for _, e := range entries {
		claims[e.ForwarderPort]++
	}
	var dupes []int32
	for port, n := range claims {
		if n > 1 {
			dupes = append(dupes, port)
		}
	}
	slices.Sort(dupes)

	next := map[int32]string{}
	for _, e := range entries {
		if claims[e.ForwarderPort] == 1 {
			next[e.ForwarderPort] = net.JoinHostPort(pe.NormalizeHost(e.Hostname), strconv.Itoa(int(e.Port)))
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ports, t.synced = next, true
	return dupes
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
