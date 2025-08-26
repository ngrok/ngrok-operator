package config

import (
	"fmt"
	"os"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// GetZapOptions returns zap.Options configured from the config
func (c *OperatorConfig) GetZapOptions() *zap.Options {
	opts := &zap.Options{}

	// Set log level
	switch strings.ToLower(c.Log.Level) {
	case "debug":
		opts.Development = true
	case "info":
		opts.Development = false
	case "error":
		opts.Development = false
	}

	// For now, let the default zap configuration handle encoding
	// This keeps compatibility with existing flag-based setup
	return opts
}

// GetNamespace returns the current pod namespace from environment
func GetNamespace() (string, error) {
	namespace, ok := os.LookupEnv("POD_NAMESPACE")
	if !ok {
		return "", fmt.Errorf("POD_NAMESPACE environment variable should be set, but was not")
	}
	return namespace, nil
}

// ParseMetadata parses comma-separated key=value pairs into a map
func ParseMetadata(metadata string) map[string]string {
	result := make(map[string]string)
	if metadata == "" {
		return result
	}

	pairs := strings.Split(metadata, ",")
	for _, pair := range pairs {
		if kv := strings.SplitN(strings.TrimSpace(pair), "=", 2); len(kv) == 2 {
			result[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return result
}

// ParseEndpointSelectors parses comma-separated endpoint selectors into a slice
func ParseEndpointSelectors(selectors string) []string {
	if selectors == "" {
		return []string{"true"}
	}

	result := make([]string, 0)
	for _, selector := range strings.Split(selectors, ",") {
		if trimmed := strings.TrimSpace(selector); trimmed != "" {
			result = append(result, trimmed)
		}
	}

	if len(result) == 0 {
		return []string{"true"}
	}

	return result
}
