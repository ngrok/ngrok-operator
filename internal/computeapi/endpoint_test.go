package computeapi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadEndpointURL(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{name: "internal HTTPS endpoint", value: "https://runner.k8s.compute.internal\n", want: "https://runner.k8s.compute.internal"},
		{name: "public endpoint", value: "https://example.ngrok.app", wantErr: true},
		{name: "raw TLS endpoint", value: "tls://runner.k8s.compute.internal", wantErr: true},
		{name: "missing hostname", value: "https://", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "endpoint")
			require.NoError(t, os.WriteFile(path, []byte(tc.value), 0o600))
			got, err := loadEndpointURL(path)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
