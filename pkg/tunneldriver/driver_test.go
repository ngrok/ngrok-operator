package tunneldriver

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/ngrok/kubernetes-ingress-controller/internal/mocks"
)

func TestConnectionIsClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	mockTun := mocks.NewMockTunnel(ctrl)
	mockDialer := mocks.NewMockDialer(ctrl)
	mockNgrokConn := mocks.NewMockConn(ctrl)
	mockBackendConn := mocks.NewMockConn(ctrl)

	bothClosed := sync.WaitGroup{}
	bothClosed.Add(2)

	gomock.InOrder(
		mockTun.EXPECT().ID().Return("logging id"),
		// It should ask ngrok for a connection
		mockTun.EXPECT().Accept().Return(mockNgrokConn, nil),
		// dial the backend
		mockNgrokConn.EXPECT().RemoteAddr().Return(&net.TCPAddr{}),
		mockDialer.EXPECT().DialContext(gomock.Any(), "tcp", "target:port").Return(mockBackendConn, nil),
	)

	// both conns should receive a read, and if they EOF get closed.
	// This is not in order because it depends on goroutine scheduling which
	// happens first
	for _, c := range []*mocks.MockConn{mockNgrokConn, mockBackendConn} {
		c.EXPECT().Read(gomock.Any()).Return(0, io.EOF)
		c.EXPECT().Close().Do(func() {
			bothClosed.Done()
		}).Return(nil)
	}
	mockTun.EXPECT().Accept().Do(func() {
		select {}
	}).AnyTimes()

	go handleConnections(ctx, mockDialer, mockTun, "target:port", "")

	bothClosed.Wait()
	ctrl.Finish()
}
