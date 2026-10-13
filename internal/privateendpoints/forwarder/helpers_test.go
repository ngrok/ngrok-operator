package forwarder

import (
	"context"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// echoDial stands in for private dial: the "endpoint" writes back the target
// it was dialed with, then echoes whatever it receives.
func echoDial(_ context.Context, target string) (net.Conn, error) {
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

func freePort(t *testing.T) int32 {
	t.Helper()
	l := listen(t)
	defer l.Close()
	return int32(l.Addr().(*net.TCPAddr).Port)
}

func readN(t *testing.T, c net.Conn, n int) string {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, n)
	_, err := io.ReadFull(c, buf)
	require.NoError(t, err)
	return string(buf)
}
