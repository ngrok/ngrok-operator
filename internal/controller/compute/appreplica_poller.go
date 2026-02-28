package compute

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v7"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// LabelNgrokID is the label used to associate managed resources with vault objects.
	LabelNgrokID = "ngrok-id"
)

// vaultAppReplica is the parsed metadata shape we look for in vault objects.
type vaultAppReplica struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Image string `json:"image"`
	ID    string `json:"id"`
	Ports []int  `json:"ports"`
}

func deploymentName(id string) string {
	return "app-replica-" + id
}

func agentEndpointName(id string, port int) string {
	return fmt.Sprintf("app-replica-%s-%d", id, port)
}

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=agentendpoints,verbs=get;list;watch;create;update;delete

// AppReplicaPoller polls the ngrok Vaults API for app-replica objects
// and reconciles them as Deployments, Services, and AgentEndpoints in the cluster.
type AppReplicaPoller struct {
	client.Client
	Log logr.Logger

	// Namespace is where resources are managed.
	Namespace string

	// NgrokClientset is the ngrok API clientset.
	NgrokClientset ngrokapi.Clientset

	// PollingInterval is how often to poll the ngrok API.
	PollingInterval time.Duration

	stopCh chan struct{}
}

// Start implements manager.Runnable.
func (r *AppReplicaPoller) Start(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx).WithName("AppReplicaPoller")

	log.Info("Starting app-replica polling routine")
	r.stopCh = make(chan struct{})
	defer close(r.stopCh)

	go r.pollLoop(ctx, log)

	<-ctx.Done()
	log.Info("Stopping app-replica polling routine")
	return nil
}

func (r *AppReplicaPoller) pollLoop(ctx context.Context, log logr.Logger) {
	ticker := time.NewTicker(r.PollingInterval)
	defer ticker.Stop()

	// Reconcile immediately on startup.
	if err := r.reconcile(ctx, log); err != nil {
		log.Error(err, "reconcile failed")
	}

	for {
		select {
		case <-ticker.C:
			log.V(9).Info("Polling vaults for app-replica objects")
			if err := r.reconcile(ctx, log); err != nil {
				log.Error(err, "reconcile failed")
			}
		case <-r.stopCh:
			return
		}
	}
}

// reconcile fetches desired state from the Vaults API, fetches current
// managed resources, then creates or deletes to converge.
func (r *AppReplicaPoller) reconcile(ctx context.Context, log logr.Logger) error {
	desired, err := r.fetchDesiredReplicas(ctx, log)
	if err != nil {
		return fmt.Errorf("fetching vault app-replicas: %w", err)
	}

	desiredByID := make(map[string]vaultAppReplica, len(desired))
	for _, d := range desired {
		desiredByID[d.ID] = d
	}

	// List existing resources managed by us.
	var deployList appsv1.DeploymentList
	if err := r.List(ctx, &deployList,
		client.InNamespace(r.Namespace),
		client.HasLabels{LabelNgrokID},
	); err != nil {
		return fmt.Errorf("listing deployments: %w", err)
	}

	existingByID := make(map[string]struct{}, len(deployList.Items))
	for _, d := range deployList.Items {
		existingByID[d.Labels[LabelNgrokID]] = struct{}{}
	}

	// Create missing resources.
	for id, replica := range desiredByID {
		if _, exists := existingByID[id]; exists {
			continue
		}
		log.Info("Creating resources for app-replica", "id", id, "name", replica.Name, "image", replica.Image, "ports", replica.Ports)
		if err := r.createResources(ctx, log, replica); err != nil {
			log.Error(err, "failed to create resources", "id", id)
		}
	}

	// Delete orphaned resources.
	for _, deploy := range deployList.Items {
		id := deploy.Labels[LabelNgrokID]
		if _, exists := desiredByID[id]; exists {
			continue
		}
		log.Info("Deleting orphaned resources", "id", id)
		r.deleteResources(ctx, log, id)
	}

	return nil
}

// fetchDesiredReplicas lists all vaults and parses their metadata for app-replica entries.
func (r *AppReplicaPoller) fetchDesiredReplicas(ctx context.Context, log logr.Logger) ([]vaultAppReplica, error) {
	var replicas []vaultAppReplica

	iter := r.NgrokClientset.Vaults().List(&ngrok.Paging{})
	for iter.Next(ctx) {
		vault := iter.Item()
		if vault == nil || vault.Metadata == "" {
			continue
		}

		var parsed vaultAppReplica
		if err := json.Unmarshal([]byte(vault.Metadata), &parsed); err != nil {
			log.V(5).Info("Skipping vault with non-JSON metadata", "vault_id", vault.ID)
			continue
		}

		if parsed.Type != "app-replica" {
			continue
		}
		if parsed.Image == "" || parsed.ID == "" || parsed.Name == "" || len(parsed.Ports) == 0 {
			log.V(3).Info("Skipping app-replica vault with missing required fields", "vault_id", vault.ID)
			continue
		}

		replicas = append(replicas, parsed)
	}

	if err := iter.Err(); err != nil {
		return nil, err
	}

	return replicas, nil
}

func (r *AppReplicaPoller) createResources(ctx context.Context, log logr.Logger, replica vaultAppReplica) error {
	name := deploymentName(replica.ID)
	labels := map[string]string{LabelNgrokID: replica.ID}

	// Build container ports and service ports from the replica's port list.
	var containerPorts []corev1.ContainerPort
	var servicePorts []corev1.ServicePort
	for _, port := range replica.Ports {
		containerPorts = append(containerPorts, corev1.ContainerPort{
			ContainerPort: int32(port),
			Protocol:      corev1.ProtocolTCP,
		})
		servicePorts = append(servicePorts, corev1.ServicePort{
			Name:       fmt.Sprintf("port-%d", port),
			Port:       int32(port),
			TargetPort: intstr.FromInt32(int32(port)),
			Protocol:   corev1.ProtocolTCP,
		})
	}

	// Deployment
	replicas := int32(1)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: replica.Image,
							Ports: containerPorts,
						},
					},
				},
			},
		},
	}
	if err := r.Create(ctx, deploy); err != nil {
		return fmt.Errorf("creating deployment: %w", err)
	}

	// Service
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports:    servicePorts,
		},
	}
	if err := r.Create(ctx, svc); err != nil {
		return fmt.Errorf("creating service: %w", err)
	}

	// AgentEndpoint per port
	for _, port := range replica.Ports {
		aepName := agentEndpointName(replica.ID, port)
		aep := &ngrokv1alpha1.AgentEndpoint{
			ObjectMeta: metav1.ObjectMeta{
				Name:      aepName,
				Namespace: r.Namespace,
				Labels:    labels,
			},
			Spec: ngrokv1alpha1.AgentEndpointSpec{
				URL: fmt.Sprintf("https://%s.internal:%d", replica.Name, port),
				Upstream: ngrokv1alpha1.EndpointUpstream{
					URL: fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", name, r.Namespace, port),
				},
			},
		}
		if err := r.Create(ctx, aep); err != nil {
			return fmt.Errorf("creating agent endpoint for port %d: %w", port, err)
		}
	}

	log.Info("Created deployment, service, and agent endpoints", "id", replica.ID, "name", name, "ports", replica.Ports)
	return nil
}

func (r *AppReplicaPoller) deleteResources(ctx context.Context, log logr.Logger, id string) {
	ns := r.Namespace

	// Delete all AgentEndpoints with this ngrok-id label.
	var aepList ngrokv1alpha1.AgentEndpointList
	if err := r.List(ctx, &aepList,
		client.InNamespace(ns),
		client.MatchingLabels{LabelNgrokID: id},
	); err != nil {
		log.Error(err, "failed to list agent endpoints for deletion", "id", id)
	} else {
		for i := range aepList.Items {
			if err := r.Delete(ctx, &aepList.Items[i]); client.IgnoreNotFound(err) != nil {
				log.Error(err, "failed to delete agent endpoint", "id", id, "name", aepList.Items[i].Name)
			}
		}
	}

	// Delete Service
	name := deploymentName(id)
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
	if err := r.Delete(ctx, svc); client.IgnoreNotFound(err) != nil {
		log.Error(err, "failed to delete service", "id", id)
	}

	// Delete Deployment
	deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
	if err := r.Delete(ctx, deploy); client.IgnoreNotFound(err) != nil {
		log.Error(err, "failed to delete deployment", "id", id)
	}
}
