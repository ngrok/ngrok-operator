package cmd

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/signals"
)

var (
	terminating, terminator = controller.NewChannelTerminationPair()
)

// WatchDeploymentTermination sets up a namespace-scoped watcher on the specified
// Deployment to notify when it is terminating (deletion timestamp is set).
// It sends a signal on the provided channel when termination is detected.
func WatchDeploymentTermination(mgr manager.Manager, namespace, name string, terminating chan<- bool) error {
	// Create a Kubernetes clientset for direct API access
	clientset, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	}

	// Create a ListWatch for the specific deployment in the namespace
	listWatch := cache.NewFilteredListWatchFromClient(
		clientset.AppsV1().RESTClient(),
		"deployments",
		namespace,
		func(options *metav1.ListOptions) {
			options.FieldSelector = fields.OneTermEqualSelector("metadata.name", name).String()
		},
	)

	informer := cache.NewSharedInformer(
		listWatch,
		&appsv1.Deployment{},
		0, // no resync
	)

	_, err = informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(oldObj, newObj any) {
			oldDep := oldObj.(*appsv1.Deployment)
			newDep := newObj.(*appsv1.Deployment)

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
	if err != nil {
		return fmt.Errorf("failed to add event handler: %w", err)
	}

	// Start the informer
	go informer.Run(make(chan struct{}))

	return nil
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
