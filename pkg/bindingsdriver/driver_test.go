package bindingsdriver

import (
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func loopbackAddr(port int32) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}

func randomPort() int32 {
	p := 30000 + rand.IntN(1000)
	return int32(p)
}

func testConnectionHandler(conn net.Conn) error {
	defer conn.Close()
	_, err := conn.Write([]byte("hello world"))
	return err
}

func TestBindingsListener(t *testing.T) {
	port := randomPort()
	bl, err := newBindingsListener(
		loopbackAddr(port),
		testConnectionHandler,
	)
	assert.NoError(t, err)
	assert.NotNil(t, bl)

	// test that we can connect to the listener
	conn, err := net.DialTimeout("tcp", loopbackAddr(port), 10*time.Millisecond)
	assert.NoError(t, err)

	out, err := io.ReadAll(conn)
	assert.NoError(t, err)

	assert.Equal(t, "hello world", string(out))

	assert.NotPanics(t, func() { bl.Stop() })

	// test that we can't connect to the listener after it's stopped
	conn, err = net.DialTimeout("tcp", loopbackAddr(port), 10*time.Millisecond)
	assert.Error(t, err)
	assert.Nil(t, conn)

	// test that we can stop the listener multiple times
	assert.NotPanics(t, func() { bl.Stop() })
}

func TestBindingsDriver(t *testing.T) {
	b := New()
	assert.NotNil(t, b)

	port := randomPort()
	err := b.Listen(port, testConnectionHandler)
	assert.NoError(t, err)

	// test that we can connect to the listener
	conn, err := net.Dial("tcp", loopbackAddr(port))
	assert.NoError(t, err)

	out, err := io.ReadAll(conn)
	assert.NoError(t, err)

	assert.Equal(t, "hello world", string(out))

	// test that trying to start a listener on the same port doesn't cause an error
	// and that only one listener exists
	assert.NoError(t, b.Listen(port, testConnectionHandler))
	assert.Len(t, b.listenerMap, 1)

	assert.NotPanics(t, func() { b.Close(port) })
	assert.NotPanics(t, func() { b.Close(port) })
}
