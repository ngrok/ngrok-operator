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

package domain

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
)

// EnsureV1 returns the v1 Domain at key, creating it if it doesn't exist.
//
// When a legacy Domain exists at key, the new v1 Domain is a copy of its spec
// as-is, never defaults: the two share one reservation, and v1 becomes the
// object that pushes its spec to it, so defaults would silently drop a
// resolvesTo, reclaim policy or description someone set on the legacy object.
// Otherwise defaults fills in the new object (name and namespace are already
// set).
//
// live, if non-nil, reads straight from the API server. It is used for the
// two reads a stale cache would get wrong: whether a legacy Domain exists (a
// miss would create the v1 object from defaults instead of a copy), and the
// re-read after a concurrent create. Pass the manager's API reader; nil falls
// back to c.
//
// The returned bool reports whether this call created the object.
func EnsureV1(ctx context.Context, c client.Client, live client.Reader, key client.ObjectKey, defaults func(*ngrokv1.Domain)) (*ngrokv1.Domain, bool, error) {
	if live == nil {
		live = c
	}

	// The cached read is fine here: if the cache missed a v1 Domain that
	// exists, the Create below fails with AlreadyExists and we re-read it.
	existing := &ngrokv1.Domain{}
	err := c.Get(ctx, key, existing)
	if err == nil {
		return existing, false, nil
	}
	if client.IgnoreNotFound(err) != nil {
		return nil, false, err
	}

	// Build the object to create: a copy of the legacy Domain at key if there
	// is one, otherwise a fresh object filled in by defaults.
	desired, err := copyOfLegacy(ctx, live, key) // LEGACY-domain-kind: at cleanup replace with `var desired *ngrokv1.Domain`.
	if err != nil {
		return nil, false, err
	}
	if desired == nil {
		desired = &ngrokv1.Domain{}
		desired.Name = key.Name
		desired.Namespace = key.Namespace
		defaults(desired)
	}

	if err := c.Create(ctx, desired); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return nil, false, err
		}
		// Created concurrently, e.g. by the driver and an endpoint reconcile.
		// The cache may not have it yet.
		if err := live.Get(ctx, key, existing); err != nil {
			return nil, false, err
		}
		return existing, false, nil
	}
	return desired, true, nil
}
