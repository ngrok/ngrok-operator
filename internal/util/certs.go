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
	// TODO: Make this configurable via helm and document it so users can
	// use it for things like proxies
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
		var err error
		// Load the system cert pool
		ngrokCertPool, err = x509.SystemCertPool()
		if err != nil {
			loadCertsOnceErr = err
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

		// if WalkDir or cert appending fails, clear the pool
		if loadCertsOnceErr != nil {
			ngrokCertPool = nil
		}
	})

	return ngrokCertPool, loadCertsOnceErr
}
