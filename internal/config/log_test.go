package config

import (
	"flag"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestApplyLogFlags(t *testing.T) {
	tests := []struct {
		name    string
		log     LogConfig
		args    []string
		want    map[string]string
		wantErr string
	}{
		{
			name: "empty settings keep the zap defaults",
			want: map[string]string{"zap-log-level": "", "zap-encoder": ""},
		},
		{
			name: "settings seed the zap flags",
			log:  LogConfig{Level: "debug", Format: "console", StacktraceLevel: "panic"},
			want: map[string]string{"zap-log-level": "debug", "zap-encoder": "console", "zap-stacktrace-level": "panic"},
		},
		{
			name: "numeric levels keep working",
			log:  LogConfig{Level: "8"},
			want: map[string]string{"zap-log-level": "8"},
		},
		{
			name: "a flag on the command line wins",
			log:  LogConfig{Level: "debug"},
			args: []string{"--zap-log-level=error"},
			want: map[string]string{"zap-log-level": "error"},
		},
		{
			name:    "an invalid value names the variable",
			log:     LogConfig{Level: "loud"},
			wantErr: "NGROK_OPERATOR_LOG_LEVEL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goFlags := flag.NewFlagSet("zap", flag.ContinueOnError)
			(&zap.Options{}).BindFlags(goFlags)
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			fs.AddGoFlagSet(goFlags)
			require.NoError(t, fs.Parse(tt.args))

			err := ApplyLogFlags(fs, tt.log)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			for name, want := range tt.want {
				assert.Equal(t, want, fs.Lookup(name).Value.String(), name)
			}
		})
	}
}
