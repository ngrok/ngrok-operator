package agent

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEndpointForwarderMap(t *testing.T) {
	m := newEndpointForwarderMap()

	ep, ok := m.Get("test")
	assert.False(t, ok)
	assert.Nil(t, ep)

	m.Add("test", nil, endpointConfig{})
	ep, ok = m.Get("test")
	assert.True(t, ok)
	assert.Nil(t, ep)

	m.Delete("test")
	ep, ok = m.Get("test")
	assert.False(t, ok)
	assert.Nil(t, ep)
}

// selfSignedCert returns a throwaway certificate for building CA pools.
// x509.CertPool only takes parsed certificates, so these can't be fake bytes.
func selfSignedCert(t *testing.T, cn string) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: cn}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return cert
}

func TestAgentTLSEqual(t *testing.T) {
	serverA := tls.Certificate{Certificate: [][]byte{[]byte("server-a")}}
	serverB := tls.Certificate{Certificate: [][]byte{[]byte("server-b")}}
	caA, caB := selfSignedCert(t, "ca-a"), selfSignedCert(t, "ca-b")

	// Builds a new pool on every call, like the controller does each reconcile.
	build := func(server tls.Certificate, ca *x509.Certificate, auth tls.ClientAuthType) *AgentTLSTermination {
		cfg := &AgentTLSTermination{ServerCert: &server, ClientAuth: auth}
		if ca != nil {
			cfg.ClientCAs = x509.NewCertPool()
			cfg.ClientCAs.AddCert(ca)
		}
		return cfg
	}
	const mTLS = tls.RequireAndVerifyClientCert

	tests := []struct {
		name string
		a, b *AgentTLSTermination
		want bool
	}{
		{name: "both nil", want: true},
		{name: "enabled vs disabled", a: build(serverA, nil, tls.NoClientCert), want: false},
		{name: "equivalent configs built separately", a: build(serverA, caA, mTLS), b: build(serverA, caA, mTLS), want: true},
		{name: "server cert rotated", a: build(serverA, caA, mTLS), b: build(serverB, caA, mTLS), want: false},
		{name: "client CA changed", a: build(serverA, caA, mTLS), b: build(serverA, caB, mTLS), want: false},
		{name: "mTLS added", a: build(serverA, nil, tls.NoClientCert), b: build(serverA, caA, mTLS), want: false},
		{name: "client auth mode changed", a: build(serverA, caA, mTLS), b: build(serverA, caA, tls.VerifyClientCertIfGiven), want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, agentTLSEqual(tc.a, tc.b))
			assert.Equal(t, tc.want, agentTLSEqual(tc.b, tc.a), "symmetric")
		})
	}
}
