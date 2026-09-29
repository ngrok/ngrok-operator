package forwarder

import (
	"bufio"
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
	return &Proxy{Table: tbl, Dial: up.dial, Log: logr.Discard(), PeekTimeout: 200 * time.Millisecond}
}

func readPrefix(t *testing.T, c net.Conn, n int) string {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, n)
	_, err := io.ReadFull(c, buf)
	require.NoError(t, err)
	return string(buf)
}

func TestServeSharedHTTP(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tbl := NewTable()
	tbl.Replace([]Entry{{Hostname: "foo.internal", Port: 80, ClusterIP: "10.0.0.1"}})
	up := &fakeUpstream{}
	l := listen(t)
	go func() { _ = newProxy(tbl, up).ServeShared(ctx, l, 80, PeekHTTPHost) }()

	t.Run("known host is dialed and bytes replayed", func(t *testing.T) {
		c, err := net.Dial("tcp", l.Addr().String())
		require.NoError(t, err)
		defer c.Close()
		req := "GET / HTTP/1.1\r\nHost: foo.internal\r\n\r\n"
		_, err = io.WriteString(c, req)
		require.NoError(t, err)
		want := "[foo.internal:80]" + req
		assert.Equal(t, want, readPrefix(t, c, len(want)))
	})

	t.Run("unknown host is closed", func(t *testing.T) {
		c, err := net.Dial("tcp", l.Addr().String())
		require.NoError(t, err)
		defer c.Close()
		_, _ = io.WriteString(c, "GET / HTTP/1.1\r\nHost: nope.internal\r\n\r\n")
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, err = bufio.NewReader(c).ReadByte()
		assert.ErrorIs(t, err, io.EOF)
	})

	t.Run("idle client is closed", func(t *testing.T) {
		c, err := net.Dial("tcp", l.Addr().String())
		require.NoError(t, err)
		defer c.Close()
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, err = bufio.NewReader(c).ReadByte()
		assert.ErrorIs(t, err, io.EOF, "server should hang up after PeekTimeout")
	})
}

func TestServeSharedTLS(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tbl := NewTable()
	tbl.Replace([]Entry{{Hostname: "foo.ngrok.direct", Port: 443, ClusterIP: "10.0.0.1"}})
	up := &fakeUpstream{}
	l := listen(t)
	go func() { _ = newProxy(tbl, up).ServeShared(ctx, l, 443, PeekSNI) }()

	hello := clientHello(t, "foo.ngrok.direct")
	c, err := net.Dial("tcp", l.Addr().String())
	require.NoError(t, err)
	defer c.Close()
	_, err = c.Write(hello)
	require.NoError(t, err)
	want := "[foo.ngrok.direct:443]" + string(hello)
	assert.Equal(t, want, readPrefix(t, c, len(want)))
}

func TestPortListenersFollowSync(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tbl := NewTable()
	up := &fakeUpstream{}
	pl := &PortListeners{Proxy: newProxy(tbl, up), Log: logr.Discard(), BindHost: "127.0.0.1"}
	defer pl.Close()

	port := freePort(t)
	tbl.Replace([]Entry{{Hostname: "old.internal", Port: 6379, ClusterIP: "10.0.0.2", ForwarderPort: port}})
	require.NoError(t, pl.Sync(ctx, []int32{port}))
	assert.Equal(t, "[old.internal:6379]", dialAndRead(t, port, len("[old.internal:6379]")))

	require.NoError(t, pl.Sync(ctx, nil))
	assert.Empty(t, pl.Ports())
	_, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	assert.Error(t, err, "listener should be closed")

	tbl.Replace([]Entry{{Hostname: "new.internal", Port: 5432, ClusterIP: "10.0.0.3", ForwarderPort: port}})
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
