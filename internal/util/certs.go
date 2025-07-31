package util

import (
	"crypto/x509"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
)

var (
	// TODO: Make this configurable via helm and document it so users can
	// use it for things like proxies
	customCertsPath  = "/etc/ssl/certs/ngrok/"
	ngrokCertPool    *x509.CertPool
	// Mutex to protect concurrent access to the cert pool
	certsMu          sync.RWMutex
)

// init starts the certs directory watcher and loads certs at startup
func init() {
	go watchCertsDir() 
	reloadCerts()      
}

// reloadCerts loads all certs from customCertsPath into ngrokCertPool
// Called at startup and whenever a file change is detected
func reloadCerts() {
	certsMu.Lock()
	defer certsMu.Unlock()

	pool, err := x509.SystemCertPool()
	if err != nil {
		ngrokCertPool = nil
		return
	}

	err = filepath.WalkDir(customCertsPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		// Skip directories, including symlinks to directories
		info, statErr := os.Stat(path)
		if statErr != nil {
			return statErr
		}
		if info.IsDir() {
			return nil
		}

		// Read and append cert
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("error reading %s: %w", path, readErr)
		}
		if ok := pool.AppendCertsFromPEM(content); !ok {
			return fmt.Errorf("failed to append certs from %s", path)
		}
		return nil
	})

	if err != nil {
		ngrokCertPool = nil
	} else {
		ngrokCertPool = pool
	}
}


// watchCertsDir watches the certs directory for changes and reloads certs automatically
func watchCertsDir() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer watcher.Close()

	// Add the certs directory to the watcher
	err = watcher.Add(customCertsPath)
	if err != nil {
		return
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			// Reload certs if any file is written, created, removed, or renamed
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
				reloadCerts()
			}
		case err, ok := <-watcher.Errors:
			if !ok || err != nil {
				return
			}
		}
	}
}

// LoadCerts returns the current cert pool, or an error if not loaded
func LoadCerts() (*x509.CertPool, error) {
	certsMu.RLock()
	defer certsMu.RUnlock()
	if ngrokCertPool == nil {
		return nil, errors.New("cert pool not loaded")
	}
	return ngrokCertPool, nil
}
