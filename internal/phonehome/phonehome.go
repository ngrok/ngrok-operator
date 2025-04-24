package phonehome

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"github.com/go-logr/logr"
	clientcommpb "github.com/ngrok/ngrok-operator/pkg/gen/clientcomm/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/proto"
)

const (
	// PhoneHomeInterval = 8 * time.Hour
	PhoneHomeInterval = 2 * time.Minute

	PhoneHomeEndpoint = "alice-clientcomm.ngrok.io:443" // TODO: Alice, add real endpoint after configuring it
)

// TODO: Alice
// - Use NGROK_API_KEY to request a certificate
// - Server validates the API key via ngrok API
// - Server issues a signed client certificate
// - Client uses the cert/key for mTLS to the main service
//
// Server-side TODO:
// - accepts CSRs
// - Authenticates the client using NGROK_API_KEY
// - sign the CSR with internal CA
// - return signed cert
// - alternatively, client can gen its own keypair and submit a CSR
//

func ConnectAndStream(logger logr.Logger) error {
	// TODO: Alice, mTLS later when we have the certs added
	// creds := credentials.NewTLS(&tls.Config{
	// 	Certificates:       []tls.Certificate{clientCert},
	// 	RootCAs:            caCertPool,
	// 	InsecureSkipVerify: false,
	// 	ClientAuth:         tls.RequireAndVerifyClientCert,
	// })

	conn, err := grpc.Dial(PhoneHomeEndpoint, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
		// Optional: verify server certificate
		MinVersion: tls.VersionTLS12,
		// NextProtos: []string{"h2"},
		// ServerName: "alice-clientcomm.ngrok.io", // must match the server cert
	})))
	if err != nil {
		return err
	}
	defer conn.Close()

	client := clientcommpb.NewClientCommServiceClient(conn)
	ctx := context.Background()
	stream, err := client.Stream(ctx)
	if err != nil {
		return err
	}

	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)

	// Start sender and receiver goroutines
	errCh := make(chan error, 2)
	go func() { errCh <- sendLoop(logger, stream, privateKey) }()
	go func() { errCh <- receiveLoop(logger, stream) }()
	return <-errCh // return on first error (will trigger retry)
}

func sendLoop(logger logr.Logger, stream clientcommpb.ClientCommService_StreamClient, privateKey *rsa.PrivateKey) error {
	ticker := time.NewTicker(8 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			payload := &clientcommpb.ClientMessage{
				Kind: &clientcommpb.ClientMessage_K8SOperatorTelemetry{
					K8SOperatorTelemetry: &clientcommpb.K8SOperatorTelemetry{
						Placeholder: "alice-foo",
					},
				},
			}

			// Deterministically serialize and sign
			// TODO: Alice, uncomment after keys are added
			// data, signature, err := signPayload(payload, privateKey)
			// if err != nil {
			// 	return err
			// }

			marshaler := proto.MarshalOptions{Deterministic: true}
			data, err := marshaler.Marshal(payload)
			if err != nil {
				logger.Error(err, "failed to marshal payload to send to server")
				return err
			}

			msg := &clientcommpb.SignedClientMessage{
				Payload:   data,
				Signature: []byte{},
			}

			if err := stream.Send(msg); err != nil {
				logger.Error(err, "failed to send message to server")
				return err
			}
		}
	}
}

func receiveLoop(logger logr.Logger, stream clientcommpb.ClientCommService_StreamClient) error {
	for {
		signedMsg, err := stream.Recv()
		if err != nil {
			logger.Error(err, "failed to receive message from server")
			return err
		}

		// Verify the signature (omitted here) TODO: Alice
		// Deserialize the payload
		var serverMsg clientcommpb.ServerMessage
		if err := proto.Unmarshal(signedMsg.GetPayload(), &serverMsg); err != nil {
			logger.Error(err, "failed to unmarshal server message")
			continue
		}

		switch cmd := serverMsg.Kind.(type) {
		case *clientcommpb.ServerMessage_SetReportingInterval:
			// TODO: Alice, implement
			newInterval := cmd.SetReportingInterval
			if newInterval == nil {
				logger.Error(fmt.Errorf("nil cmd.SetReportingInterval"), "invalid server message received")
				continue
			}
			logger.Info("Received SetReportingInterval message from server",
				"new interval", newInterval.NewInterval.AsDuration(),
			)
		default:
			logger.Error(fmt.Errorf("unknown or unsupported command: %T", cmd), "invalid server message received")
		}
	}
}

func signPayload(payload proto.Message, privKey *rsa.PrivateKey) (data []byte, signature []byte, err error) {
	marshaler := proto.MarshalOptions{Deterministic: true}
	data, err = marshaler.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}

	hash := sha256.Sum256(data)
	sig, err := rsa.SignPKCS1v15(rand.Reader, privKey, crypto.SHA256, hash[:])
	if err != nil {
		return nil, nil, err
	}

	return data, sig, nil
}

func newMTLSClient(certFile, keyFile, caFile string) (*grpc.ClientConn, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}

	caCertPool := x509.NewCertPool()
	caBytes, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	caCertPool.AppendCertsFromPEM(caBytes)

	creds := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert}, // Client cert
		RootCAs:      caCertPool,              // For server cert verification
	})

	return grpc.Dial("alice.todo:8443", grpc.WithTransportCredentials(creds))
}
