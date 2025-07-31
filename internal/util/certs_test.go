package util

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func resetCertsState() {
	ngrokCertPool = nil
	certsMu = sync.RWMutex{
	}
	}

func TestLoadCerts_ValidPEM(t *testing.T) {
	resetCertsState()
	tmpDir := t.TempDir()
	customCertsPath = tmpDir

	err := os.WriteFile(filepath.Join(tmpDir, "valid.pem"), []byte(validPEM), 0644)
	require.NoError(t, err)

	reloadCerts()

	pool, err := LoadCerts()
	require.NoError(t, err)
	require.NotNil(t, pool)
}

func TestLoadCerts_InvalidPEM(t *testing.T) {
	resetCertsState()
	tmpDir := t.TempDir()
	customCertsPath = tmpDir

	err := os.WriteFile(filepath.Join(tmpDir, "invalid.pem"), []byte("not a cert"), 0644)
	require.NoError(t, err)

	reloadCerts()
	pool, err := LoadCerts()

	require.Error(t, err, "expected error, got nil")
	t.Logf("err: %v", err)

	require.Nil(t, pool, "expected nil pool when certs path is missing")
	require.Error(t, err)

}

func TestLoadCerts_MissingPath(t *testing.T) {
	resetCertsState()
	tmpDir := filepath.Join(t.TempDir(), "nonexistent")
	customCertsPath = tmpDir

	pool, err := LoadCerts()

	require.Error(t, err, "expected error, got nil")
	t.Logf("err: %v", err)

	// pool will be nil because cert loading failed completely
	require.Nil(t, pool, "expected nil pool when certs path is missing")
}

func TestCerts_AutoReloadOnChange(t *testing.T) {
	resetCertsState()
	tmpDir := t.TempDir()
	customCertsPath = tmpDir

	// write initial valid cert
	certPath := filepath.Join(tmpDir, "valid.pem")
	err := os.WriteFile(certPath, []byte(validPEM), 0644)
	require.NoError(t, err)

	reloadCerts()
	pool1, err := LoadCerts()
	require.NoError(t, err)
	require.NotNil(t, pool1)

	// overwrite with invalid cert to simulate cert rotation
	err = os.WriteFile(certPath, []byte("not a cert"), 0644)
	require.NoError(t, err)

	// simulate watcher event by calling reloadCerts again
	reloadCerts()
	pool2, err := LoadCerts()
	require.Error(t, err)
	require.Nil(t, pool2)
}

const validPEM = `-----BEGIN CERTIFICATE-----
MIIDwjCCAqqgAwIBAgIUZqF2AkB17pISojTndgc2U5BDt7wwDQYJKoZIhvcNAQEL
BQAwbzEQMA4GA1UEAwwHUm9vdCBDQTENMAsGA1UECwwEcHJvZDESMBAGA1UECgwJ
bmdyb2sgSW5jMRYwFAYDVQQHDA1TYW4gRnJhbmNpc2NvMRMwEQYDVQQIDApDYWxp
Zm9ybmlhMQswCQYDVQQGEwJVUzAeFw0yMjA4MzExNDU5NDhaFw0zMjA4MjgxNDU5
NDhaMF8xCzAJBgNVBAYTAlVTMRMwEQYDVQQIDApDYWxpZm9ybmlhMRIwEAYDVQQK
DAluZ3JvayBJbmMxDTALBgNVBAsMBHByb2QxGDAWBgNVBAMMD0ludGVybWVkaWF0
ZSBDQTCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAK+t8q9Ost9BxCWX
fyGG0mVQOpIiyrzzZWyqT6CZpMY2fpOadLuZeBP7ti2Iw4FgCpfLntL0RldvMMNY
4qq61dVrCwhL/v2ldsaHUdzjtFj1i+ZNGUtV4E9korHxm2YdsD91w6WIjF/J0lvo
X2koLwFlGc/CkhT8z2VWebY8a6mYNyz5S7yPTQh2/mQ14lx/QPJgZSFEE/EEkMDC
bs4BoMuqKMhCpqEP8m4+CxPQ5/V6POSqUIxT4A7eWWj2MRpnmirmVbXOc24Aznqk
bdQUP4qagiR/i7qPsRx+f4mFfDninPsXp/djjByo0xzdh+i1HFyOR/7nyNDKlJ+e
rymRgnUCAwEAAaNmMGQwHQYDVR0OBBYEFJ47nRzHaOT+vY44N3TCMYtGlBjIMB8G
A1UdIwQYMBaAFNxeUxPIM8G7cX0DhFc81pLD4W+HMBIGA1UdEwEB/wQIMAYBAf8C
AQAwDgYDVR0PAQH/BAQDAgGGMA0GCSqGSIb3DQEBCwUAA4IBAQBRmnMoOtQbYL7P
Co1B5Chslb86HP2WI1jGRXhbfwAF2ySDFnX2ZbRPVtoQ+IuqXWxyXAeicYjXR6kz
xX8hLWfD14kWUIz6ZgT3uZrDSIzmQ+tz8ztbT6mTI1ECWdjLV/i58f6vKzgLD8Vp
3VdVns8NA9ee6a65QNjZEnwBVeccysoWkOwM/KzuazhSGcGu44y/S4ny9pAg7Pol
2kV4NicDKD6tSAdXmPmjFalYUfnMmyhurZIPrS2dgYgpOrGVMwronTOZ3BUf4DL4
zkkmcLXss1KztQnLd23nuNiIscwMcGM58a3O5zUp7aorfrm7cdRgkFmcYVNO/6uG
Q5iJ+Ppk
-----END CERTIFICATE-----`
