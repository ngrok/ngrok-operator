package version

import (
	"fmt"
	"runtime"
)

var (
	// version of the ngrok-operator.
	// Injected at build time via LDFlags.
	version = "0.0.0"

	// gitCommit is the git sha1
	// Injected at build time via LDFlags.
	gitCommit = ""
)

// BuildInfo describes the compile time information.
type BuildInfo struct {
	// Version is the current semver.
	Version string `json:"version,omitempty"`
	// GitCommit is the git sha1.
	GitCommit string `json:"git_commit,omitempty"`
	// GoVersion is the version of the Go compiler used.
	GoVersion string `json:"go_version,omitempty"`
}

// GetVersion returns the semver string of the version
func GetVersion() string {
	return version
}

// GetUserAgent returns a user agent to use
func GetUserAgent() string {
	return fmt.Sprintf("ngrok-operator/%s", GetVersion())
}

// Get returns build info
func Get() BuildInfo {
	return BuildInfo{
		Version:   GetVersion(),
		GitCommit: gitCommit,
		GoVersion: runtime.Version(),
	}
}
