package forwarder

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForwardConnJoinsBothWays(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	go func() { _ = ForwardConn(context.Background(), echoDial, server, "foo.internal:80", time.Second) }()

	assert.Equal(t, "[foo.internal:80]", readN(t, client, len("[foo.internal:80]")))
	_, err := io.WriteString(client, "ping")
	require.NoError(t, err)
	assert.Equal(t, "ping", readN(t, client, 4))
}

func TestForwardConnDialFailureClosesClient(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	failDial := func(context.Context, string) (net.Conn, error) { return nil, errors.New("endpoint offline") }

	err := ForwardConn(context.Background(), failDial, server, "gone.internal:80", time.Second)
	require.ErrorContains(t, err, "endpoint offline")
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	_, err = client.Read(make([]byte, 1))
	assert.ErrorIs(t, err, io.EOF)
}

// A client that never closes must not pin the private dial stream forever
// once the endpoint has finished sending.
func TestForwardConnDrainsAfterUpstreamEOF(t *testing.T) {
	upstreamL := listen(t)
	defer upstreamL.Close()
	upstreamSawClose := make(chan struct{})
	go func() {
		c, err := upstreamL.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_, _ = io.WriteString(c, "bye")
		_ = c.(*net.TCPConn).CloseWrite()
		_, _ = io.Copy(io.Discard, c) // returns once the forwarder closes its side
		close(upstreamSawClose)
	}()
	dial := func(context.Context, string) (net.Conn, error) { return net.Dial("tcp", upstreamL.Addr().String()) }

	clientL := listen(t)
	defer clientL.Close()
	go func() {
		server, err := clientL.Accept()
		if err == nil {
			_ = ForwardConn(context.Background(), dial, server, "done.internal:80", 200*time.Millisecond)
		}
	}()
	client, err := net.Dial("tcp", clientL.Addr().String())
	require.NoError(t, err)
	defer client.Close() // stays open and silent for the whole test
	assert.Equal(t, "bye", readN(t, client, 3))

	select {
	case <-upstreamSawClose:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream connection still open long after the drain timeout")
	}
}
