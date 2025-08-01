package util

import (
	"crypto/x509"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	// TODO: Make this configurable via helm and document it so users can
	// use it for things like proxies
	customCertsPath  = "/etc/ssl/certs/ngrok/"
	ngrokCertPool    *x509.CertPool
	loadCertsOnce    sync.Once
	loadCertsOnceErr error
)

func init() {
	go watchCertsDir()
}

// LoadCerts loads all certs from customCertsPath once, merges them with system certs
func LoadCerts() (*x509.CertPool, error) {
	// create a logger 
	ctrl.Log.Info("Loading custom certs", "path", customCertsPath)
	loadCertsOnce.Do(func() {
		var err error
		ngrokCertPool, err = x509.SystemCertPool()
		if err != nil {
			loadCertsOnceErr = err
			return
		}

		loadCertsOnceErr = filepath.WalkDir(customCertsPath, func(path string, d fs.DirEntry, err error) error {
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

			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if ok := ngrokCertPool.AppendCertsFromPEM(content); !ok {
				return fmt.Errorf("failed to append certs from %s", path)
			}
			return nil
		})

		if loadCertsOnceErr != nil {
			ngrokCertPool = nil
		}
	})

	return ngrokCertPool, loadCertsOnceErr
}

// watchCertsDir watches for changes in the certs directory and exits on any update
func watchCertsDir() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer watcher.Close()

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
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
				ctrl.Log.Info("Detected change in custom certs directory, restarting pod")
				os.Exit(0)
			}
		case err, ok := <-watcher.Errors:
			if !ok || err != nil {
				return
			}
		}
	}
}
