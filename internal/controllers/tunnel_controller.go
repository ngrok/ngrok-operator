package controllers

import (
	"context"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	k8sngrokcomv1 "github.com/ngrok/ngrok-ingress-controller/pkg/api/v1"
)

const (
	// TODO: We can technically figure this out by looking at things like our resolv.conf or we can just take this as a helm option
	clusterDomain = "svc.cluster.local"
)

// TunnelReconciler reconciles a Tunnel object
type TunnelReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Log      logr.Logger
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=k8s.ngrok.com,resources=tunnels,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.ngrok.com,resources=tunnels/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.ngrok.com,resources=tunnels/finalizers,verbs=update

func (trec *TunnelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := trec.Log.WithValues("tunnel", req.NamespacedName)
	ctx = ctrl.LoggerInto(ctx, log)
	ingress, err := getIngress(ctx, trec.Client, req.NamespacedName)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	// getIngress didn't return the object, so we can't do anything with it
	if ingress == nil {
		return ctrl.Result{}, nil
	}
	if err := validateIngress(ctx, ingress); err != nil {
		trec.Recorder.Event(ingress, v1.EventTypeWarning, "Invalid ingress, discarding the event.", err.Error())
		return ctrl.Result{}, nil
	}

	/*
		// Check if the ingress object is being deleted
		if ingress.ObjectMeta.DeletionTimestamp != nil && !ingress.ObjectMeta.DeletionTimestamp.IsZero() {
			for _, tunnel := range ingressToTunnels(ingress) {
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

		for _, tunnel := range ingressToTunnels(ingress) {
			trec.Recorder.Event(ingress, v1.EventTypeNormal, "TunnelCreating", fmt.Sprintf("Tunnel %s creating", tunnel.Name))
			err = agentapiclient.NewAgentApiClient().CreateTunnel(ctx, tunnel)
			if err != nil {
				trec.Recorder.Event(ingress, "Warning", "TunnelCreateFailed", fmt.Sprintf("Tunnel %s create failed", tunnel.Name))
				return ctrl.Result{}, err
			}
			trec.Recorder.Event(ingress, "Normal", "TunnelCreated", fmt.Sprintf("Tunnel %s created with labels %q", tunnel.Name, tunnel.Labels))
		}
	*/

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TunnelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8sngrokcomv1.Tunnel{}).
		Complete(r)
}
