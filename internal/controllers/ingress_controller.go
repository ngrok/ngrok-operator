package controllers

import (
	"context"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// This implements the Reconciler for the controller-runtime
// https://pkg.go.dev/sigs.k8s.io/controller-runtime#section-readme
type IngressReconciler struct {
	client.Client
	Log       logr.Logger
	Scheme    *runtime.Scheme
	Recorder  record.EventRecorder
	Namespace string
}

// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses,verbs=get;list;watch;update
// +kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses/status,verbs=get;list;watch;update
// +kubebuilder:rbac:groups="networking.k8s.io",resources=ingressclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;delete

// This reconcile function is called by the controller-runtime manager.
// It is invoked whenever there is an event that occurs for a resource
// being watched (in our case, ingress objects). If you tail the controller
// logs and delete, update, edit ingress objects, you see the events come in.
func (ir *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	edgeName := getEdgeName(req.NamespacedName.String())
	log := ir.Log.WithValues("ingress", req.NamespacedName)
	ingress, err := getIngress(ctx, ir.Client, req.NamespacedName)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	err = setStatus(ctx, ir, ingress, edgeName)
	if err != nil {
		log.Error(err, "Failed to set status")
		return ctrl.Result{}, err

	}
	return ctrl.Result{}, nil
}

// Create a new controller using our reconciler and set it up with the manager
func (t *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&netv1.Ingress{}).
		Complete(t)
}

// LogicalEdgeNamespace returns a string that can be used to namespace api
// resources in the ngrok api. The namespace would be used to control load balancing
// between clusters. This function should be only called by the leader to avoid multiple
// controllers attempting a read/write operation on the same config map without a lock.
func (ir *IngressReconciler) LogicalEdgeNamespace(ctx context.Context) (string, error) {
	configMapName := "ngrok-ingress-controller-edge-namespace"
	configMapKey := "edge-namespace"
	// This should be configurable by the user eventually or random. For now, be consistent for testing
	newName := "devenv-users"
	config := &v1.ConfigMap{}
	// Try to find the existing config map
	err := ir.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: ir.Namespace}, config)
	if err == nil {
		if val, ok := config.Data[configMapKey]; ok {
			return val, nil
		} else {
			panic("Config map is missing the key " + configMapKey + " which shouldn't be possible")
		}
	}

	// Valid non "not found" error
	if client.IgnoreNotFound(err) != nil {
		return "", err
	}

	// If its not found, try to make it
	if err := ir.Create(ctx, &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: ir.Namespace,
		},
		Data: map[string]string{
			configMapKey: newName,
		},
	}); err != nil {
		return "", err
	}
	return newName, nil
}
