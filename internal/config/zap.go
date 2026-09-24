package config

import (
	"fmt"

	uberzap "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// ZapOptions builds controller-runtime zap options from the log config.
//
// This replaces zap.Options.BindFlags, whose --zap-* flag names did not match
// any config key. The operator's own --log-* flags are registered against the
// same config fields instead.
func ZapOptions(log LogConfig) (*zap.Options, error) {
	level, err := parseLevel(log.Level)
	if err != nil {
		return nil, fmt.Errorf("log.level: %w", err)
	}

	stacktrace, err := parseLevel(log.StacktraceLevel)
	if err != nil {
		return nil, fmt.Errorf("log.stacktraceLevel: %w", err)
	}

	opts := &zap.Options{
		Level:           level,
		StacktraceLevel: stacktrace,
	}

	switch log.Format {
	case "json":
		opts.Encoder = zapcore.NewJSONEncoder(uberzap.NewProductionEncoderConfig())
	case "console":
		opts.Encoder = zapcore.NewConsoleEncoder(uberzap.NewDevelopmentEncoderConfig())
	default:
		return nil, fmt.Errorf("log.format: unknown format %q, want json or console", log.Format)
	}

	return opts, nil
}

func parseLevel(level string) (zapcore.Level, error) {
	parsed, err := zapcore.ParseLevel(level)
	if err != nil {
		return 0, fmt.Errorf("unknown level %q: %w", level, err)
	}
	return parsed, nil
}
