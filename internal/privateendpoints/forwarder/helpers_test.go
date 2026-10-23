package forwarder

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func listen(t *testing.T) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	return l
}

func readN(t *testing.T, c net.Conn, n int) string {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, n)
	_, err := io.ReadFull(c, buf)
	require.NoError(t, err)
	return string(buf)
}
