// Package flagstest helps tests that read settings from the environment.
package flagstest

import (
	"os"
	"strings"
	"testing"
)

// ClearEnv blanks every NGROK_OPERATOR__* variable for the rest of the test, so
// a test sees only the variables it sets itself. A blank variable counts as
// unset.
func ClearEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); strings.HasPrefix(name, "NGROK_OPERATOR__") {
			t.Setenv(name, "")
		}
	}
}
