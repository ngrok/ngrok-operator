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

package cloudendpoint

import (
	"context"
	"testing"

	"github.com/ngrok/ngrok-api-go/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
)

func TestSyncer_Delete(t *testing.T) {
	for kind, r := range kinds(spec{url: "https://example.ngrok.app"}) {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			endpoints := nmockapi.NewEndpointsClient()
			s := &Syncer{Endpoints: endpoints}

			remote, err := endpoints.Create(ctx, &ngrok.EndpointCreate{Type: "cloud", URL: r.GetURL()})
			require.NoError(t, err)
			r.SetID(remote.ID)

			require.NoError(t, s.Delete(ctx, r))
			assert.Empty(t, r.GetID())
			_, err = endpoints.Get(ctx, remote.ID)
			assert.True(t, ngrok.IsNotFound(err))

			r.SetID(remote.ID)
			err = s.Delete(ctx, r)
			assert.True(t, ngrok.IsNotFound(err), "already-gone endpoint returns NotFound for the caller")
			assert.Empty(t, r.GetID())
		})
	}
}
