package util

import (
	"crypto/x509"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
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
	// Load all certificates from the well known ngrok certs directory,
	// combine them with the default certs, and save the cert pool once.
	// If we've already done this, just return the cert pool.
	ctrl.Log.Info("Loading custom certs", "path", customCertsPath)
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

		// if WalkDir or cert appending fails, clear the pool
		if loadCertsOnceErr != nil {
			ngrokCertPool = nil
		}
	})

	return ngrokCertPool, loadCertsOnceErr
}

func watchCertsDirWithHandler(path string, onChange func()) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	if err := watcher.Add(path); err != nil {
		return err
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
				onChange()
				return nil
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			return err
		}
	}
}

// watchCertsDir watches for changes in the certs directory and exits on any update
func watchCertsDir() {
	_ = watchCertsDirWithHandler(customCertsPath, func() {
		if shouldExitOnCertChange() {
			ctrl.Log.Info("Detected changes in custom certs directory, reloading certs")
			os.Exit(0)
		} else {
			ctrl.Log.Info("Detected changes in custom certs directory, but NGROK_OPERATOR_RESTART_ON_CERT_CHANGE is not set to true, so not restarting")
		}
	})
}

// shouldExitOnCertChange checks if the environment variable indicates we should exit on cert changes
func shouldExitOnCertChange() bool {
	envVar := os.Getenv("NGROK_OPERATOR_RESTART_ON_CERT_CHANGE")
	if envVar == "" {
		return false
	}

	shouldExit, err := strconv.ParseBool(envVar)
	if err != nil {
		ctrl.Log.Info("Invalid boolean value for NGROK_OPERATOR_RESTART_ON_CERT_CHANGE, defaulting to false", "value", envVar)
		return false
	}

	return shouldExit
}
