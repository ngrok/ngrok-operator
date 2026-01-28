package mux

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-operator/internal/pb_agent"
	"google.golang.org/protobuf/proto"
)

// ReadProxyMessage reads a protobuf message from a connection
func ReadProxyMessage(conn net.Conn, msg proto.Message) error {
	// read header length
	var hdrLength uint16
	err := binary.Read(conn, binary.LittleEndian, &hdrLength)
	if err != nil {
		// Note: If you are seeing this error it likely means the TLS handshake from client -> mux did not work
		// This can happen if:
		// * the cert is bad or empty
		// * the cert was signed by a different mux than the one you are connecting to (i.e. the ingress-binding-endpoint is mismatched to the tls.crt)
		return fmt.Errorf("failed to read header length: %w", err)
	}

	// read header bytes
	buf := make([]byte, hdrLength)
	_, err = io.ReadFull(conn, buf)
	if err != nil {
		return fmt.Errorf("failed to read header: %w", err)
	}

	// unmarshal header
	err = proto.Unmarshal(buf, msg)
	if err != nil {
		return fmt.Errorf("failed to unmarshal header: %w", err)
	}

	return nil
}

// WriteProxyMessage writes a protobuf message to a connection
func WriteProxyMessage(conn net.Conn, msg proto.Message) error {
	buf, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	hdrlen := uint16(len(buf))
	if err = binary.Write(conn, binary.LittleEndian, hdrlen); err != nil {
		return fmt.Errorf("failed to write header length: %w", err)
	}

	_, err = conn.Write(buf)
	return err
}

type BindingUpgradeFailure struct {
	resp *pb_agent.ConnResponse
}

func (e *BindingUpgradeFailure) Error() string {
	return fmt.Sprintf("binding upgrade failure: [%s]: %s", e.resp.ErrorCode, e.resp.ErrorMessage)
}

// UpgradeToBindingConnection upgrades a connection to a binding connection by exchanging header information
// with the server. It may return a BindingUpgradeFailure error if the server can't upgrade the connection. The
// underlying connection may also return an error if the connection is closed or otherwise fails.
func UpgradeToBindingConnection(log logr.Logger, conn net.Conn, host string, port int, podIdentity *pb_agent.PodIdentity) (resp *pb_agent.ConnResponse, err error) {
	resp = new(pb_agent.ConnResponse)

	// Exchange the header information
	err = WriteProxyMessage(conn, &pb_agent.ConnRequest{Host: host, Port: int64(port), PodIdentity: podIdentity})
	if err != nil {
		log.Error(err, "failed to write proxy message")
		return
	}

	err = ReadProxyMessage(conn, resp)
	if err != nil {
		log.Error(err, "failed to read proxy message")
	}

	if resp.ErrorCode != "" || resp.ErrorMessage != "" {
		err = &BindingUpgradeFailure{resp}
	}
	return
}
