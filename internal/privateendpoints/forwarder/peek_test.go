package forwarder

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"io"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPeekHTTPHost(t *testing.T) {
	tests := []struct {
		name    string
		req     string
		want    string
		wantErr bool
	}{
		{"simple", "GET / HTTP/1.1\r\nHost: foo.internal\r\n\r\n", "foo.internal", false},
		{"with port and case", "GET / HTTP/1.1\r\nHost: FOO.internal:80\r\nUser-Agent: x\r\n\r\nbody", "foo.internal", false},
		{"no host header", "GET / HTTP/1.0\r\n\r\n", "", true},
		{"not http", "\x16\x03\x01garbage\r\n\r\n", "", true},
		{"truncated", "GET / HTTP/1.1\r\nHost: foo", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			br := bufio.NewReaderSize(strings.NewReader(tt.req), PeekBufferSize)
			got, err := PeekHTTPHost(br)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			replay, _ := io.ReadAll(br)
			assert.Equal(t, tt.req, string(replay), "peek must not consume bytes")
		})
	}
}

// clientHello captures the bytes a real TLS client sends for serverName.
func clientHello(t *testing.T, serverName string) []byte {
	t.Helper()
	c, s := net.Pipe()
	go func() {
		_ = tls.Client(c, &tls.Config{ServerName: serverName, InsecureSkipVerify: true}).Handshake()
	}()
	buf := make([]byte, PeekBufferSize)
	n, err := io.ReadAtLeast(s, buf, 5)
	require.NoError(t, err)
	recLen := 5 + (int(buf[3])<<8 | int(buf[4]))
	for n < recLen {
		m, err := s.Read(buf[n:])
		require.NoError(t, err)
		n += m
	}
	_ = s.Close()
	_ = c.Close()
	return buf[:n]
}

func TestPeekSNI(t *testing.T) {
	hello := clientHello(t, "Foo.ngrok.direct")
	tests := []struct {
		name    string
		data    []byte
		want    string
		wantErr bool
	}{
		{"client hello", hello, "foo.ngrok.direct", false},
		{"not tls", []byte("GET / HTTP/1.1\r\n\r\n"), "", true},
		{"no sni", clientHello(t, ""), "", true},
		{"truncated", hello[:20], "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			br := bufio.NewReaderSize(bytes.NewReader(tt.data), PeekBufferSize)
			got, err := PeekSNI(br)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			replay, _ := io.ReadAll(br)
			assert.Equal(t, tt.data, replay, "peek must not consume bytes")
		})
	}
}
