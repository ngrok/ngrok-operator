package util

import (
	"crypto/x509"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

const (
	customCertsPath = "/etc/ssl/certs/ngrok/"
)

var (
	ngrokCertPool    *x509.CertPool
	loadCertsOnce    sync.Once
	loadCertsOnceErr error
)

func LoadCerts() (*x509.CertPool, error) {
	// Load all certificates from the well known ngrok certs directory,
	// combine them with the default certs, and save the cert pool once.
	// If we've already done this, just return the cert pool.
	loadCertsOnce.Do(func() {
		// Load the system cert pool
		ngrokCertPool, loadCertsOnceErr = x509.SystemCertPool()
		if loadCertsOnceErr != nil {
			return
		}

		// Now, walk the ngrok certs dir and add all the certs to the cert pool
		loadCertsOnceErr = filepath.WalkDir(customCertsPath, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			// Skip directories
			if d.IsDir() {
				return nil
			}

			// Open the file
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			if ok := ngrokCertPool.AppendCertsFromPEM(content); !ok {
				return fmt.Errorf("failed to append certs from %s", path)
			}

			return nil
		})
	})

	return ngrokCertPool, loadCertsOnceErr
}
