package tunneldriver

import (
	"context"
	"io"
	"net"
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
	_, backendUs := net.Pipe()

	closed := make(chan struct{})

	gomock.InOrder(
		mockTun.EXPECT().ID().Return("logging id"),
		// It should ask ngrok for a connection
		mockTun.EXPECT().Accept().Return(mockNgrokConn, nil),
		// dial the backend
		mockNgrokConn.EXPECT().RemoteAddr().Return(&net.TCPAddr{}),
		mockDialer.EXPECT().DialContext(gomock.Any(), "tcp", "target:port").Return(backendUs, nil),
		// try to read data to copy to the backend
		mockNgrokConn.EXPECT().Read(gomock.Any()).Return(0, io.EOF),
		// and then, when it gets EOF, close the backend connection
		mockNgrokConn.EXPECT().Close().Do(func() {
			close(closed)
		}).Return(nil),
	)
	mockTun.EXPECT().Accept().Do(func() {
		select {}
	}).AnyTimes()

	go handleConnections(ctx, mockDialer, mockTun, "target:port")

	<-closed
	ctrl.Finish()
}
