package phonehome

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"log"
	"os"
	"time"

	clientcommpb "github.com/ngrok/ngrok-operator/pkg/gen/clientcomm/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/proto"
)

const (
	PhoneHomeInterval = 8 * time.Hour

	PhoneHomeEndpoint = "https://alice-todo.ngrok.io" // TODO: Alice, add real endpoint after configuring it
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

func ConnectAndStream() error {
	// TODO: Alice, mTLS later when we have the certs added
	// creds := credentials.NewTLS(&tls.Config{
	// 	Certificates:       []tls.Certificate{clientCert},
	// 	RootCAs:            caCertPool,
	// 	InsecureSkipVerify: false,
	// 	ClientAuth:         tls.RequireAndVerifyClientCert,
	// })

	conn, err := grpc.Dial(PhoneHomeEndpoint, grpc.WithInsecure())
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
	go func() { errCh <- sendLoop(stream, privateKey) }()
	go func() { errCh <- receiveLoop(stream) }()
	return <-errCh // return on first error (will trigger retry)
}

func sendLoop(stream clientcommpb.ClientCommService_StreamClient, key *rsa.PrivateKey) error {
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
				return err
			}

			msg := &clientcommpb.SignedClientMessage{
				Payload:   data,
				Signature: []byte{},
			}

			if err := stream.Send(msg); err != nil {
				return err
			}
		}
	}
}

func receiveLoop(stream clientcommpb.ClientCommService_StreamClient) error {
	for {
		signedMsg, err := stream.Recv()
		if err != nil {
			return err
		}

		// Verify the signature (omitted here) TODO: Alice
		// Deserialize the payload
		var serverMsg clientcommpb.ServerMessage
		if err := proto.Unmarshal(signedMsg.GetPayload(), &serverMsg); err != nil {
			log.Printf("Failed to unmarshal server message: %v", err)
			continue
		}

		switch cmd := serverMsg.Kind.(type) {
		case *clientcommpb.ServerMessage_SetReportingInterval:
			// TODO: Alice, implement
			log.Printf("Received SetReportingInterval message from server")
		default:
			log.Printf("Received unknown or unsupported command: %T", cmd)
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
