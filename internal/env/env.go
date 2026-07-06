/*
MIT License

Copyright (c) 2026 ngrok, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

// Package env provides helpers for reading NGROK_OPERATOR_* environment
// variables as CLI flag defaults, following the same model as Argo CD: flag
// definitions use these helpers for their default values, so the precedence
// observed by users is:
//
//	CLI flag > environment variable > built-in default
//
// The Helm chart renders app config into per-component ConfigMaps and injects
// the values into the pods as these environment variables.
//
// Empty environment variables are treated as unset. Values that fail to parse
// log a warning to stderr and fall back to the built-in default.
package env

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
)

// warned tracks environment variables that already produced a parse warning.
// All subcommand constructors run at program start regardless of which
// subcommand executes, so without deduplication a malformed value would be
// reported once per command that reads it.
var warned sync.Map

func warnOnce(key, format string, args ...any) {
	if _, loaded := warned.LoadOrStore(key, true); !loaded {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}

// StringFromEnv returns the value of the environment variable if it is set
// and non-empty, otherwise defaultValue.
func StringFromEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// BoolFromEnv parses the environment variable as a boolean if it is set and
// non-empty, otherwise returns defaultValue.
func BoolFromEnv(key string, defaultValue bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		warnOnce(key, "WARNING: invalid boolean %q for %s, using default %t\n", v, key, defaultValue)
		return defaultValue
	}
	return b
}

// StringSliceFromEnv splits the environment variable on commas if it is set
// and non-empty, otherwise returns defaultValue.
func StringSliceFromEnv(key string, defaultValue []string) []string {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	parts := strings.Split(v, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// zapFlagEnvVars maps the controller-runtime zap flag names to their
// environment variables. These flags are defined by zap.Options.BindFlags
// rather than by this operator, so their defaults can't be set inline the way
// the operator's own flags are.
var zapFlagEnvVars = map[string]string{
	"zap-log-level":        "NGROK_OPERATOR_LOG_LEVEL",
	"zap-encoder":          "NGROK_OPERATOR_LOG_FORMAT",
	"zap-stacktrace-level": "NGROK_OPERATOR_LOG_STACKTRACE_LEVEL",
}

// BindZapFlagDefaults applies the NGROK_OPERATOR_LOG_* environment variables
// as defaults for the controller-runtime zap flags. Call it after
// zap.Options.BindFlags and before the flag set is parsed; flags passed on
// the command line still win.
func BindZapFlagDefaults(fs *flag.FlagSet) {
	for flagName, envVar := range zapFlagEnvVars {
		v := os.Getenv(envVar)
		if v == "" || fs.Lookup(flagName) == nil {
			continue
		}
		if err := fs.Set(flagName, v); err != nil {
			warnOnce(envVar, "WARNING: invalid value %q for %s, using default: %v\n", v, envVar, err)
		}
	}
}
