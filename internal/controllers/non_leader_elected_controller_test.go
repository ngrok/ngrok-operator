package controllers

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// dummyController is a controller that does nothing.
type dummyController struct{}

func (d dummyController) Reconcile(context.Context, reconcile.Request) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}
func (d dummyController) Watch(src source.Source, handler handler.EventHandler, predicates ...predicate.Predicate) error {
	return nil
}
func (d dummyController) Start(context.Context) error {
	return nil
}
func (d dummyController) GetLogger() logr.Logger {
	return logr.Discard()
}

func TestNonLeaderElectedController(t *testing.T) {
	ctrl := dummyController{}
	assert.Implements(t, (*controller.Controller)(nil), ctrl)
	wrappedController := NonLeaderElectedController{ctrl}
	assert.Implements(t, (*controller.Controller)(nil), wrappedController)

	// Assert that the wrapped controller implements the LeaderElectionRunnable interface
	assert.Implements(t, (*manager.LeaderElectionRunnable)(nil), wrappedController)

	// Assert that the wrapped controller does not need to be leader elected
	assert.False(t, wrappedController.NeedLeaderElection())
}
