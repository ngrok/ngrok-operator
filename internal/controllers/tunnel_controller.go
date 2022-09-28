package controllers

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-ingress-controller/pkg/agentapiclient"
	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	// TODO: We can technically figure this out by looking at things like our resolv.conf or we can just take this as a helm option
	clusterDomain = "svc.cluster.local"
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
func (trec *TunnelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := trec.Log.WithValues("ingress", req.NamespacedName)
	ctx = ctrl.LoggerInto(ctx, log)
	// TODO: This name is used as the name of the tunnel. Right now its just the ingress name + namespace. So if we need to create
	// multiple tunnels for 1 ingress object, we'll need to provide some namespacing here.
	tunnelName := strings.Replace(req.NamespacedName.String(), "/", "-", -1)
	ingress, err := getIngress(ctx, trec.Client, req.NamespacedName)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	// getIngress didn't return the object, so we can't do anything with it
	if ingress == nil {
		return ctrl.Result{}, nil
	}

	trec.Recorder.Event(ingress, "Normal", "Reconcile Called", fmt.Sprintf("tunnelName:%q with req: %q", tunnelName, req))

	// Check if the ingress object is being deleted
	if ingress.ObjectMeta.DeletionTimestamp != nil && !ingress.ObjectMeta.DeletionTimestamp.IsZero() {
		for _, tunnel := range IngressToTunnels(ingress) {
			trec.Recorder.Event(ingress, v1.EventTypeNormal, "TunnelDeleting", fmt.Sprintf("Tunnel %s deleting", tunnel.Name))
			err := agentapiclient.NewAgentApiClient().DeleteTunnel(ctx, tunnel.Name)
			if err != nil {
				trec.Recorder.Event(ingress, "Warning", "TunnelDeleteFailed", fmt.Sprintf("Tunnel %s delete failed", tunnel.Name))
				return ctrl.Result{}, err
			}
			trec.Recorder.Event(ingress, v1.EventTypeNormal, "TunnelDeleted", fmt.Sprintf("Tunnel %s deleted", tunnel.Name))
		}
		return ctrl.Result{}, nil
	}

	for _, tunnel := range IngressToTunnels(ingress) {
		trec.Recorder.Event(ingress, v1.EventTypeNormal, "TunnelCreating", fmt.Sprintf("Tunnel %s creating", tunnel.Name))
		err = agentapiclient.NewAgentApiClient().CreateTunnel(ctx, tunnel)
		if err != nil {
			trec.Recorder.Event(ingress, "Warning", "TunnelCreateFailed", fmt.Sprintf("Tunnel %s create failed", tunnelName))
			return ctrl.Result{}, err
		}
		trec.Recorder.Event(ingress, "Normal", "TunnelCreated", fmt.Sprintf("Tunnel %s created with labels %q", tunnelName, tunnel.Labels))
	}

	return ctrl.Result{}, nil
}

// Create a new Controller that watches Ingress objects.
// Add it to our manager.
func (trec *TunnelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	tCont, err := NewTunnelControllerNew("tunnel-controller", mgr, trec)
	if err != nil {
		return err
	}

	if err := tCont.Watch(&source.Kind{Type: &netv1.Ingress{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	mgr.Add(tCont)
	return nil
}

// Small wrapper struct of the core controller.Controller so we get most of its functionality
type TunnelController struct {
	controller.Controller
}

// Creates an un-managed controller that can be embeded in our controller struct so we can override functions.
func NewTunnelControllerNew(name string, mgr manager.Manager, trec *TunnelReconciler) (controller.Controller, error) {
	cont, err := controller.NewUnmanaged(name, mgr, controller.Options{
		Reconciler: trec,
	})
	if err != nil {
		return nil, err
	}
	return &TunnelController{
		Controller: cont,
	}, nil
}

// This controller should not use leader election. It should run on all controllers by default to control the agents on each.
func (trec *TunnelController) NeedLeaderElection() bool {
	return false
}

func (trec *TunnelController) Start(ctx context.Context) error {
	// TODO: Wait for k8s config map with controller namespaces to be ready
	return trec.Controller.Start(ctx)
}

// Converts a k8s Ingress Rule to and Ngrok Agent Tunnel configuration.
func tunnelsPlanner(rule netv1.IngressRuleValue, ingressName, namespace string) []agentapiclient.TunnelsApiBody {
	var agentTunnels []agentapiclient.TunnelsApiBody

	for _, httpIngressPath := range rule.HTTP.Paths {
		serviceName := httpIngressPath.Backend.Service.Name
		servicePort := int(httpIngressPath.Backend.Service.Port.Number)
		tunnelAddr := fmt.Sprintf("%s.%s.%s:%d", serviceName, namespace, clusterDomain, servicePort)

		var labels []string
		for key, value := range backendToLabelMap(httpIngressPath.Backend, ingressName, namespace) {
			labels = append(labels, fmt.Sprintf("%s=%s", key, value))
		}

		agentTunnels = append(agentTunnels, agentapiclient.TunnelsApiBody{
			Name:   fmt.Sprintf("%s-%s-%s-%d", ingressName, namespace, serviceName, servicePort),
			Addr:   tunnelAddr,
			Labels: labels,
		})
	}

	return agentTunnels
}

// Converts a k8s ingress object into a slice of Ngrok Agent Tunnels
// TODO: Support multiple Rules per Ingress
func IngressToTunnels(ingress *netv1.Ingress) []agentapiclient.TunnelsApiBody {
	ingressRule := ingress.Spec.Rules[0]

	tunnels := tunnelsPlanner(ingressRule.IngressRuleValue, ingress.Name, ingress.Namespace)
	return tunnels
}
