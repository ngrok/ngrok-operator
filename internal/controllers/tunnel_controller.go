package controllers

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"ngrok.io/ngrok-ingress-controller/pkg/agentapiclient"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// This implements the Reconciler for the controller-runtime
// https://pkg.go.dev/sigs.k8s.io/controller-runtime#section-readme
type TunnelReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses,verbs=get;list;watch;update
// +kubebuilder:rbac:groups="networking.k8s.io",resources=ingressclasses,verbs=get;list;watch

// This reconcile function is called by the controller-runtime manager.
// It is invoked whenever there is an event that occurs for a resource
// being watched (in our case, ingress objects). If you tail the controller
// logs and delete, update, edit ingress objects, you see the events come in.
func (t *TunnelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := t.Log.WithValues("ingress", req.NamespacedName)
	edgeName := strings.Replace(req.NamespacedName.String(), "/", "-", -1)
	ingress, err := getIngress(ctx, t.Client, req.NamespacedName)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info(fmt.Sprintf("We did find the ingress %+v \n", ingress))
	// TODO: For now this assumes 1 rule and 1 path. Expand on this and loop through them
	backendService := ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service
	log.Info(fmt.Sprintf("TODO: Create the api resources needed for this %s", edgeName))
	if err := agentapiclient.NewAgentApiClient().CreateTunnel(ctx, agentapiclient.TunnelsApiBody{
		Name:  edgeName,
		Proto: "http",
		// TODO: This will need to handle cross namespace connections
		Addr: fmt.Sprintf("%s:%d", backendService.Name, backendService.Port.Number),
		// Labels: []string{"ngrok.io/ingress-name=" + ingress.Name, "ngrok.io/service-name=" + backendService.Name},
	},
	); err != nil {
		log.Error(err, "Failed to create tunnel")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// Create a new Controller that watches Ingress objects.
// Add it to our manager.
func (t *TunnelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	c, err := NewTunnelControllerNew("tunnel-controller", mgr, t)
	if err != nil {
		return err
	}

	if err := c.Watch(&source.Kind{Type: &netv1.Ingress{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	mgr.Add(c)
	return nil
}

// Small wrapper struct of the core controller.Controller so we get most of its functionality
type TunnelController struct {
	controller.Controller
}

// Creates an un-managed controller that can be embeded in our controller struct so we can override functions.
func NewTunnelControllerNew(name string, mgr manager.Manager, tr *TunnelReconciler) (controller.Controller, error) {
	c, err := controller.NewUnmanaged(name, mgr, controller.Options{
		Reconciler: tr,
	})
	if err != nil {
		return nil, err
	}
	return &TunnelController{
		Controller: c,
	}, nil
}

// This controller should not use leader election. It should run on all controllers by default to control the agents on each.
func (t *TunnelController) NeedLeaderElection() bool {
	return false
}
