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
package reaper

import (
	"context"
	"time"

	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/util"
	"golang.org/x/sync/errgroup"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type reaper struct {
	client              client.Client
	apiGroupConcurrency *int
}

type ReaperOpt func(*reaper)

// WithAPIGroupConcurrency sets the number of concurrent API group deletions.
func WithAPIGroupConcurrency(c int) ReaperOpt {
	return func(r *reaper) {
		r.apiGroupConcurrency = &c
	}
}

// New creates a new Reaper instance that cleans up all resources managed by the operator.
// It is normally invoked during shutdown, when the deployment is terminating, to ensure all resources are cleaned up.
func New(client client.Client, opts ...ReaperOpt) *reaper {
	r := &reaper{
		client: client,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *reaper) Cleanup(ctx context.Context, typeToCleanup ...any) error {

	g, gctx := errgroup.WithContext(ctx)
	defer gctx.Done()
	if r.apiGroupConcurrency != nil {
		g.SetLimit(*r.apiGroupConcurrency)
	}

	for _, t := range typeToCleanup {
		g.Go(func() error {
			return r.continuallyDeleteResourcesWithFinalizer(gctx, t)
		})
	}
	return g.Wait()
}

// continuallyDeleteResourcesWithFinalizer continually deletes resources with our finalizer set.
// This is useful for resources that may be re-created during shutdown.
func (r *reaper) continuallyDeleteResourcesWithFinalizer(ctx context.Context, v any) error {
	for {
		err := r.deleteResourcesWithFinalizer(ctx, v)
		if err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

// deleteResourcesWithFinalizer deletes all resources of the given type that have our finalizer set and
// are not already being deleted.
func (r *reaper) deleteResourcesWithFinalizer(ctx context.Context, v any) error {
	objs, err := util.ListObjectsForType(ctx, r.client, v)
	if err != nil {
		return err
	}

	g, gctx := errgroup.WithContext(ctx)
	defer gctx.Done()

	for _, obj := range objs {
		o := obj
		if !controller.HasFinalizer(o) {
			continue
		}

		if controller.IsDelete(o) {
			continue
		}

		g.Go(func() error {
			return client.IgnoreNotFound(r.client.Delete(gctx, o))
		})
	}

	return g.Wait()
}
