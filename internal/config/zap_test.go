package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

func TestZapOptions(t *testing.T) {
	for _, tc := range []struct {
		name      string
		log       LogConfig
		wantErr   string
		wantLevel zapcore.Level
		wantTrace zapcore.Level
	}{
		{
			name:      "defaults",
			log:       LogConfig{Level: "info", Format: "json", StacktraceLevel: "error"},
			wantLevel: zapcore.InfoLevel,
			wantTrace: zapcore.ErrorLevel,
		},
		{
			name:      "debug console",
			log:       LogConfig{Level: "debug", Format: "console", StacktraceLevel: "info"},
			wantLevel: zapcore.DebugLevel,
			wantTrace: zapcore.InfoLevel,
		},
		{
			name:    "bad level",
			log:     LogConfig{Level: "chatty", Format: "json", StacktraceLevel: "error"},
			wantErr: "chatty",
		},
		{
			name:    "bad format",
			log:     LogConfig{Level: "info", Format: "yaml", StacktraceLevel: "error"},
			wantErr: "yaml",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts, err := ZapOptions(tc.log)

			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantLevel, opts.Level.(zapcore.Level))
			assert.Equal(t, tc.wantTrace, opts.StacktraceLevel.(zapcore.Level))
		})
	}
}
