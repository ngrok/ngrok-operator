/*
MIT License

Copyright (c) 2025 ngrok, Inc.

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
package cmd

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadNgrokClientset(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		expectErr bool
	}{
		{
			name:   "access token can list endpoints",
			status: http.StatusOK,
			body:   `{"endpoints": [], "uri": "", "next_page_uri": null}`,
		},
		{
			name:      "access token rejected",
			status:    http.StatusForbidden,
			body:      `{"error_code": "ERR_NGROK_203", "status_code": 403, "msg": "forbidden"}`,
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotAuth string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/endpoints", r.URL.Path)
				gotAuth = r.Header.Get("Authorization")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			t.Setenv("NGROK_ACCESS_TOKEN", "test-token")

			_, err := loadNgrokClientset(t.Context(), apiManagerOpts{apiURL: srv.URL})

			assert.Equal(t, "Bearer test-token", gotAuth)
			if tc.expectErr {
				require.ErrorContains(t, err, "Unable to verify access token")
				return
			}
			require.NoError(t, err)
		})
	}
}
