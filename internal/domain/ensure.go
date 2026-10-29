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

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
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
// The returned bool reports whether this call created the object.
func EnsureV1(ctx context.Context, c client.Client, key client.ObjectKey, defaults func(*ngrokv1.Domain)) (*ngrokv1.Domain, bool, error) {
	existing := &ngrokv1.Domain{}
	err := c.Get(ctx, key, existing)
	if err == nil {
		return existing, false, nil
	}
	if client.IgnoreNotFound(err) != nil {
		return nil, false, err
	}

	desired := &ngrokv1.Domain{}
	desired.Name = key.Name
	desired.Namespace = key.Namespace

	// LEGACY-domain-kind: BEGIN
	legacy := &ingressv1alpha1.Domain{}
	err = c.Get(ctx, key, legacy)
	switch {
	case err == nil:
		if desired, err = V1FromLegacy(legacy); err != nil {
			return nil, false, err
		}
	case client.IgnoreNotFound(err) != nil:
		return nil, false, err
	default:
		defaults(desired)
	}
	// LEGACY-domain-kind: END

	if err := c.Create(ctx, desired); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return nil, false, err
		}
		// Created concurrently, e.g. by the driver and an endpoint reconcile.
		if err := c.Get(ctx, key, existing); err != nil {
			return nil, false, err
		}
		return existing, false, nil
	}
	return desired, true, nil
}
