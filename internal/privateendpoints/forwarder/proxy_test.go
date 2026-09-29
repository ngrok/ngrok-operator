package forwarder

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeUpstream records the target each dial asked for and echoes bytes back
// prefixed with that target, standing in for private dial.
type fakeUpstream struct {
	mu      sync.Mutex
	targets []string
}

func (f *fakeUpstream) dial(_ context.Context, target string) (net.Conn, error) {
	f.mu.Lock()
	f.targets = append(f.targets, target)
	f.mu.Unlock()
	a, b := net.Pipe()
	go func() {
		defer b.Close()
		fmt.Fprintf(b, "[%s]", target)
		_, _ = io.Copy(b, b)
	}()
	return a, nil
}

func listen(t *testing.T) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	return l
}

func newProxy(tbl *Table, up *fakeUpstream) *Proxy {
	return &Proxy{Table: tbl, Dial: up.dial, Log: logr.Discard(), DrainTimeout: time.Second}
}

func readPrefix(t *testing.T, c net.Conn, n int) string {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, n)
	_, err := io.ReadFull(c, buf)
	require.NoError(t, err)
	return string(buf)
}

// A client that never closes must not pin the upstream stream forever once
// the upstream has finished sending.
func TestPipeDrainsAfterUpstreamEOF(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	upstreamL := listen(t)
	defer upstreamL.Close()
	upstreamSawClose := make(chan error, 1)
	go func() {
		c, err := upstreamL.Accept()
		if err != nil {
			upstreamSawClose <- err
			return
		}
		defer c.Close()
		_, _ = io.WriteString(c, "bye")
		_ = c.(*net.TCPConn).CloseWrite()
		_, err = io.Copy(io.Discard, c) // returns once the forwarder closes its side
		upstreamSawClose <- err
	}()

	tbl := NewTable()
	port := freePort(t)
	tbl.Replace([]Entry{{Hostname: "done.internal", Port: 80, ForwarderPort: port}})
	p := &Proxy{
		Table:        tbl,
		Log:          logr.Discard(),
		DrainTimeout: 200 * time.Millisecond,
		Dial: func(context.Context, string) (net.Conn, error) {
			return net.Dial("tcp", upstreamL.Addr().String())
		},
	}
	pl := &PortListeners{Proxy: p, Log: logr.Discard(), BindHost: "127.0.0.1"}
	defer pl.Close()
	require.NoError(t, pl.Sync(ctx, []int32{port}))

	c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	require.NoError(t, err)
	defer c.Close() // the client stays open and silent for the whole test
	assert.Equal(t, "bye", readPrefix(t, c, 3))

	select {
	case <-upstreamSawClose:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream connection still open long after DrainTimeout")
	}
}

func TestPortListenersFollowSync(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tbl := NewTable()
	up := &fakeUpstream{}
	pl := &PortListeners{Proxy: newProxy(tbl, up), Log: logr.Discard(), BindHost: "127.0.0.1"}
	defer pl.Close()

	port := freePort(t)
	tbl.Replace([]Entry{{Hostname: "old.internal", Port: 6379, ForwarderPort: port}})
	require.NoError(t, pl.Sync(ctx, []int32{port}))
	assert.Equal(t, "[old.internal:6379]", dialAndRead(t, port, len("[old.internal:6379]")))

	require.NoError(t, pl.Sync(ctx, nil))
	assert.Empty(t, pl.Ports())
	_, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	assert.Error(t, err, "listener should be closed")

	tbl.Replace([]Entry{{Hostname: "new.internal", Port: 5432, ForwarderPort: port}})
	require.NoError(t, pl.Sync(ctx, []int32{port}))
	assert.Equal(t, "[new.internal:5432]", dialAndRead(t, port, len("[new.internal:5432]")))
}

func freePort(t *testing.T) int32 {
	t.Helper()
	l := listen(t)
	defer l.Close()
	return int32(l.Addr().(*net.TCPAddr).Port)
}

func dialAndRead(t *testing.T, port int32, n int) string {
	t.Helper()
	c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	require.NoError(t, err)
	defer c.Close()
	return readPrefix(t, c, n)
}
