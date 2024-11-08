package bindings

import (
	"fmt"
	"sync"

	"github.com/docker/docker/libnetwork/bitmap"
)

// portBitmap is a thin wrapper around bitmap.Bitmap to
// automatically offset by start and perform locking.
type portBitmap struct {
	start uint16
	mu    sync.Mutex
	ports *bitmap.Bitmap
}

func newPortBitmap(start uint16, end uint16) *portBitmap {
	size := uint64(end - start)

	return &portBitmap{
		start: start,
		ports: bitmap.New(size),
	}
}

// Set sets a port in the portmap. It must be between 'start' and 'start+size'.
// If it cannot be set, such as due to a conflict, an error will be returned.
func (pb *portBitmap) Set(port uint16) error {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	if port < pb.start {
		panic(fmt.Sprintf("portBitmap.Set called with port before start of port range; port=%v, start=%v", port, pb.start))
	}
	return pb.ports.Set(uint64(port - pb.start))
}

// SetAny sets the next port in the portmap.
func (pb *portBitmap) SetAny() (uint16, error) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	port, err := pb.ports.SetAny(true) // set serially in the range
	if err != nil {
		return 0, err
	}

	return uint16(port) + pb.start, nil
}

// Check checks if a port is set in the portmap. It must be between 'start' and
// 'start+size'.
func (pb *portBitmap) IsSet(port uint16) bool {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	if port < pb.start {
		panic(fmt.Sprintf("portBitmap.Set called with port before start of port range; port=%v, start=%v", port, pb.start))
	}
	return pb.ports.IsSet(uint64(port - pb.start))
}

// Unset clears a port in the portmap. It must be between 'start' and 'start+size'
func (pb *portBitmap) Unset(port uint16) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	if port < pb.start {
		panic(fmt.Sprintf("portBitmap.Set called with port before start of port range; port=%v, start=%v", port, pb.start))
	}
	if err := pb.ports.Unset(uint64(port - pb.start)); err != nil {
		panic(fmt.Sprintf("error unsetting port %d: %s", port, err))
	}
}

func (pb *portBitmap) NumFree() uint64 {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	return pb.ports.Unselected()
}
