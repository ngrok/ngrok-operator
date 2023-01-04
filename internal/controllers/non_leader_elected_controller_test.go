package controllers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestNonLeaderElectedController(t *testing.T) {
	testenv := &envtest.Environment{}
	cfg, err := testenv.Start()
	assert.NoError(t, err)

	rec := reconcile.Func(func(context.Context, reconcile.Request) (reconcile.Result, error) {
		return reconcile.Result{}, nil
	})

	m, err := manager.New(cfg, manager.Options{})
	assert.NoError(t, err)

	c, err := controller.NewUnmanaged("tunnel-controller", m, controller.Options{
		Reconciler: rec,
	})
	assert.NoError(t, err)

	wrappedController := NonLeaderElectedController{c}

	// Assert that the wrapped controller implements the LeaderElectionRunnable interface
	assert.Implements(t, (*manager.LeaderElectionRunnable)(nil), wrappedController)

	// Assert that the wrapped controller does not need to be leader elected
	assert.False(t, wrappedController.NeedLeaderElection())
}
