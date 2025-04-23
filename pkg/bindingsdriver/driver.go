package bindingsdriver

import (
	"fmt"
	"net"
	"sync"

	"github.com/go-logr/logr"
)

type BindingsDriver struct {
	listenerMap   map[int32]*bindingsListener
	listenerMapMu sync.Mutex
}

func New() *BindingsDriver {
	return &BindingsDriver{
		listenerMap:   make(map[int32]*bindingsListener),
		listenerMapMu: sync.Mutex{},
	}
}

func (b *BindingsDriver) Listen(port int32, cnxnHandler ConnectionHandler) error {
	b.listenerMapMu.Lock()
	defer b.listenerMapMu.Unlock()

	if _, ok := b.listenerMap[port]; ok {
		return nil // already listening
	}

	bl, err := newBindingsListener(
		fmt.Sprintf("0.0.0.0:%d", port),
		cnxnHandler,
	)
	if err != nil {
		return err
	}

	b.listenerMap[port] = bl
	return nil
}

func (b *BindingsDriver) Close(port int32) {
	b.listenerMapMu.Lock()
	bl, ok := b.listenerMap[port]
	if !ok {
		// not listening
		b.listenerMapMu.Unlock()
		return
	}

	delete(b.listenerMap, port)
	b.listenerMapMu.Unlock()

	bl.Stop()
}

type ConnectionHandler func(net.Conn) error

type bindingsListener struct {
	listener    net.Listener
	cnxnHandler ConnectionHandler
	log         logr.Logger

	stopOnce sync.Once
	stop     chan struct{}
}

func newBindingsListener(address string, cnxnHandler ConnectionHandler) (*bindingsListener, error) {
	l, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	bl := &bindingsListener{
		listener:    l,
		cnxnHandler: cnxnHandler,
		stop:        make(chan struct{}),
	}

	go bl.run()

	return bl, nil
}

// Stop stops the listener. It is safe to call stop multiple times.
func (b *bindingsListener) Stop() {
	b.stopOnce.Do(func() {
		if err := b.listener.Close(); err != nil {
			b.log.Error(err, "encountered error while closing bindings listener")
		}
		b.stop <- struct{}{}
	})
}

func (b *bindingsListener) run() {
	for {
		select {
		case <-b.stop:
			return
		default:
		}

		conn, err := b.listener.Accept()
		if err != nil {
			b.log.Error(err, "failed to accept connection")
			continue
		}

		// handle connection
		go b.cnxnHandler(conn) // nolint:errcheck
	}
}
