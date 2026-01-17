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
package controller

import (
	"context"

	"github.com/ngrok/ngrok-operator/internal/annotations"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	FinalizerName     = "k8s.ngrok.com/finalizer"
	CleanupAnnotation = annotations.CleanupAnnotation
)

var (
	// HasCleanupAnnotation returns true if the object has the cleanup annotation set to "true".
	// It is a re-export of the function from the annotations package.
	HasCleanupAnnotation = annotations.HasCleanupAnnotation
)

// IsUpsert returns true if the object is being created or updated. That is, if the deletion timestamp is not set.
func IsUpsert(o client.Object) bool {
	return o.GetDeletionTimestamp().IsZero()
}

// IsDelete returns true if the object is being deleted. That is, if the deletion timestamp is set and non-zero.
func IsDelete(o client.Object) bool {
	return !o.GetDeletionTimestamp().IsZero()
}

// HasFinalizer returns true if the object has the ngrok finalizer present.
// It is our wrapper around controllerutil.ContainsFinalizer.
func HasFinalizer(o client.Object) bool {
	return controllerutil.ContainsFinalizer(o, FinalizerName)
}

// AddFinalizer accepts an Object and adds the ngrok finalizer if not already present.
// It returns an indication of whether it updated the object's list of finalizers.
// It is our wrapper around controllerutil.AddFinalizer.
func AddFinalizer(o client.Object) bool {
	return controllerutil.AddFinalizer(o, FinalizerName)
}

// RemoveFinalizer accepts an Object and removes the ngrok finalizer if present.
// It returns an indication of whether it updated the object's list of finalizers.
// It is our wrapper around controllerutil.RemoveFinalizer.
func RemoveFinalizer(o client.Object) bool {
	return controllerutil.RemoveFinalizer(o, FinalizerName)
}

// RegisterAndSyncFinalizer adds the ngrok finalizer to the object if not already present.
// If it adds the finalizer, it updates the object in the Kubernetes API.
func RegisterAndSyncFinalizer(ctx context.Context, c client.Writer, o client.Object) error {
	if AddFinalizer(o) {
		return c.Update(ctx, o)
	}
	return nil
}

// RemoveAndSyncFinalizer removes the ngrok finalizer from the object if present.
// If it removes the finalizer, it updates the object in the Kubernetes API.
func RemoveAndSyncFinalizer(ctx context.Context, c client.Writer, o client.Object) error {
	if RemoveFinalizer(o) {
		return c.Update(ctx, o)
	}
	return nil
}

// AddAnnotations adds the given annotations to the object.
func AddAnnotations(o client.Object, annotations map[string]string) {
	if o == nil || annotations == nil {
		return
	}

	existing := o.GetAnnotations()
	if existing == nil {
		existing = make(map[string]string)
	}

	for k, v := range annotations {
		existing[k] = v
	}

	o.SetAnnotations(existing)
}

// IsCleanedUp returns true if the object has the cleanup annotation and no finalizer
// indicating that cleanup has been completed.
func IsCleanedUp(o client.Object) bool {
	return HasCleanupAnnotation(o) && !HasFinalizer(o)
}
