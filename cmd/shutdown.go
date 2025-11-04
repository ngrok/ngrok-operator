package cmd

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/signals"
)

var (
	terminating, terminator = controller.NewChannelTerminationPair()
)

// WatchDeploymentTermination sets up a watcher on the specified Deployment to
// notify when it is terminating (deletion timestamp is set). It sends a signal
// on the provided channel when termination is detected.
func WatchDeploymentTermination(mgr manager.Manager, namespace, name string, terminating chan<- bool) error {
	ctx := context.Background()

	// Get the shared informer for Deployments from controller-runtime’s cache.
	inf, err := mgr.GetCache().GetInformer(ctx, &appsv1.Deployment{})
	if err != nil {
		return fmt.Errorf("failed to get informer for Deployment: %w", err)
	}

	_, err = inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(oldObj, newObj any) {
			oldDep := oldObj.(*appsv1.Deployment)
			newDep := newObj.(*appsv1.Deployment)

			// Only the target Deployment
			if newDep.Namespace != namespace || newDep.Name != name {
				return
			}

			oldDT := oldDep.GetDeletionTimestamp()
			newDT := newDep.GetDeletionTimestamp()
			if (oldDT == nil || oldDT.IsZero()) && newDT != nil && !newDT.IsZero() {
				// Deletion just started
				shutdownLog.Info("Deployment is terminating", "namespace", namespace, "name", name)
				terminating <- true
				close(terminating)
			}
		},
	})

	return err
}

// OnDeploymentTerminating sets up a watcher for deployment termination and executes
// the provided function f when termination is detected during shutdown.
// It returns a context and cancel function for the manager to use.
func OnDeploymentTerminating(ctx context.Context, mgr manager.Manager, f func()) (context.Context, context.CancelFunc) {
	if !InKubernetes() {
		setupLog.Info("not running in Kubernetes, skipping deployment termination watcher")
		return ctx, func() {}
	}

	namespace := GetCurrentNamespace()
	deploymentName := GetDeploymentName()
	if deploymentName == "" {
		setupLog.Info("skipping deployment termination watcher: DEPLOYMENT_NAME environment variable was not set")
		return ctx, func() {}
	}

	if namespace == "" {
		setupLog.Info(fmt.Sprintf("skipping deployment termination watcher: unable to read namespace from file %s", serviceAccountNamespacePath))
		return ctx, func() {}
	}

	setupLog.Info("setting up deployment termination watcher", "namespace", namespace, "deployment", deploymentName)
	deploymentTerminating := make(chan bool)
	if err := WatchDeploymentTermination(mgr, namespace, deploymentName, deploymentTerminating); err != nil {
		setupLog.Error(err, "failed to set up deployment termination watcher: proceeding without it")
		return ctx, func() {}
	}

	// Perform pre-shutdown cleanup when we receive a shutdown signal. If the deployment
	// is terminating, we want to clean up resources so that finalizers can be processed
	// before the pod is killed. Once the pod is killed, the finalizers cannot be processed
	// because the operator is no longer running.
	mgrCtx, mgrCancel := context.WithCancel(ctx)
	signals.OnShutdown(func() {
		shutdownLog.Info("shutdown signal received, waiting to see if deployment is terminating")
		// ensure we cancel the manager context on shutdown
		defer mgrCancel()

		select {
		case <-time.After(10 * time.Second):
			shutdownLog.Info("timeout waiting for deployment termination, proceeding with shutdown")
			return
		case <-deploymentTerminating:
			shutdownLog.Info("deployment is terminating, proceeding with shutdown")
			f()
		}
	})

	return mgrCtx, mgrCancel
}
