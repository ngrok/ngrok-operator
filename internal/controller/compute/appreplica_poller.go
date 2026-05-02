package compute

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-logr/logr"
	ngrok "github.com/ngrok/ngrok-api-go/v7"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// LabelNgrokID is the label used to associate managed resources with compute replica objects.
	LabelNgrokID = "ngrok-id"
)

// replicaEndpoint represents a single endpoint with its URL and optional traffic policy.
type replicaEndpoint struct {
	URL           string `json:"url"`
	TrafficPolicy string `json:"traffic_policy"`
}

// computeReplica represents a replica object from the ngrok Compute Replicas API.
type computeReplica struct {
	ID              string            `json:"id"`
	CreatedAt       string            `json:"created_at"`
	AppID           string            `json:"app_id"`
	EnvironmentID   string            `json:"environment_id"`
	DeploymentID    string            `json:"deployment_id"`
	RunnerID        string            `json:"runner_id"`
	State           string            `json:"state"`
	ContainerImage  string            `json:"container_image"`
	Endpoints       []replicaEndpoint `json:"endpoints"`
	EnvironmentVars map[string]string `json:"environment_vars"`
	URI             string            `json:"uri"`
}

// computeReplicaList represents a paginated list response from the Compute Replicas API.
type computeReplicaList struct {
	ComputeReplicas []computeReplica `json:"compute_replicas"`
	URI             string           `json:"uri"`
	NextPageURI     *string          `json:"next_page_uri"`
}

// urlPort extracts the port from a URL string. If no port is specified, returns 443.
func urlPort(rawURL string) (int, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0, err
	}
	if p := u.Port(); p != "" {
		var port int
		_, err := fmt.Sscanf(p, "%d", &port)
		return port, err
	}
	return 443, nil
}

func k8sName(id string) string {
	return strings.ReplaceAll(id, "_", "-")
}

func deploymentName(id string) string {
	return "app-replica-" + k8sName(id)
}

func agentEndpointName(id string, port int) string {
	return fmt.Sprintf("app-replica-%s-%d", k8sName(id), port)
}

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=agentendpoints,verbs=get;list;watch;create;update;delete

// AppReplicaPoller polls the ngrok Compute Replicas API for app-replica objects
// and reconciles them as Deployments, Services, and AgentEndpoints in the cluster.
type AppReplicaPoller struct {
	client.Client
	Log logr.Logger

	// Namespace is where resources are managed.
	Namespace string

	// K8sOpName is the name of the KubernetesOperator CR to look up for the runner ID.
	K8sOpName string

	// K8sOpNamespace is the namespace of the KubernetesOperator CR.
	K8sOpNamespace string

	// NgrokBaseClient is the ngrok API base client.
	NgrokBaseClient *ngrok.BaseClient

	// PollingInterval is how often to poll the ngrok API.
	PollingInterval time.Duration

	runnerID string
	stopCh   chan struct{}
}

// Start implements manager.Runnable.
func (r *AppReplicaPoller) Start(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx).WithName("AppReplicaPoller")

	r.runnerID = r.waitForRunnerID(ctx, log)
	if r.runnerID == "" {
		log.Info("Context canceled before runner ID was available, not starting")
		return nil
	}

	log.Info("Starting app-replica polling routine", "runner_id", r.runnerID)
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
			log.V(9).Info("Polling compute replicas API")
			if err := r.reconcile(ctx, log); err != nil {
				log.Error(err, "reconcile failed")
			}
		case <-r.stopCh:
			return
		}
	}
}

// reconcile fetches desired state from the Replicas API, fetches current
// managed resources, then creates or deletes to converge.
func (r *AppReplicaPoller) reconcile(ctx context.Context, log logr.Logger) error {
	desired, err := r.fetchDesiredReplicas(ctx, log)
	if err != nil {
		return fmt.Errorf("fetching compute replicas: %w", err)
	}

	desiredByID := make(map[string]computeReplica, len(desired))
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
		log.Info("Creating resources for app-replica", "id", id, "image", replica.ContainerImage)
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

// waitForRunnerID polls the KubernetesOperator CR until it has a registered ID to use as the runner ID.
func (r *AppReplicaPoller) waitForRunnerID(ctx context.Context, log logr.Logger) string {
	log.Info("Waiting for KubernetesOperator to be registered")

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ticker.Stop()
			ticker.Reset(30 * time.Second)

			var ko ngrokv1alpha1.KubernetesOperator
			if err := r.Get(ctx, client.ObjectKey{Name: r.K8sOpName, Namespace: r.K8sOpNamespace}, &ko); err != nil {
				log.Error(err, "Failed to get KubernetesOperator", "name", r.K8sOpName)
				continue
			}

			if ko.Status.ID == "" {
				log.V(1).Info("KubernetesOperator not yet registered, waiting...")
				continue
			}

			log.Info("KubernetesOperator registered, using as runner ID", "id", ko.Status.ID)
			return ko.Status.ID
		case <-ctx.Done():
			return ""
		}
	}
}

// fetchDesiredReplicas lists running compute replicas assigned to this runner via the Compute Replicas API.
func (r *AppReplicaPoller) fetchDesiredReplicas(ctx context.Context, log logr.Logger) ([]computeReplica, error) {
	var replicas []computeReplica

	filter := fmt.Sprintf(`obj.runner_id == "%s" && obj.state == "running"`, r.runnerID)
	nextPage := &url.URL{
		Path:     "/compute/replicas",
		RawQuery: url.Values{"filter": {filter}}.Encode(),
	}

	for nextPage != nil {
		var resp computeReplicaList
		if err := r.NgrokBaseClient.Do(ctx, "GET", nextPage, nil, &resp); err != nil {
			return nil, fmt.Errorf("listing compute replicas: %w", err)
		}

		for _, replica := range resp.ComputeReplicas {
			if replica.ContainerImage == "" || replica.ID == "" || len(replica.Endpoints) == 0 {
				log.V(3).Info("Skipping replica with missing required fields", "replica_id", replica.ID)
				continue
			}
			replica.ID = strings.ToLower(replica.ID)
			replicas = append(replicas, replica)
		}

		if resp.NextPageURI != nil {
			nextPage, _ = url.Parse(*resp.NextPageURI)
		} else {
			nextPage = nil
		}
	}

	return replicas, nil
}

func (r *AppReplicaPoller) createResources(ctx context.Context, log logr.Logger, replica computeReplica) error {
	name := deploymentName(replica.ID)
	labels := map[string]string{LabelNgrokID: replica.ID}

	// Derive ports from endpoint URLs.
	type endpointWithPort struct {
		endpoint replicaEndpoint
		port     int
	}
	var epPorts []endpointWithPort
	for _, ep := range replica.Endpoints {
		port, err := urlPort(ep.URL)
		if err != nil {
			return fmt.Errorf("parsing URL %q: %w", ep.URL, err)
		}
		epPorts = append(epPorts, endpointWithPort{endpoint: ep, port: port})
	}

	// Build container ports and service ports.
	var containerPorts []corev1.ContainerPort
	var servicePorts []corev1.ServicePort
	seenPorts := make(map[int]bool)
	for _, up := range epPorts {
		if seenPorts[up.port] {
			continue
		}
		seenPorts[up.port] = true
		containerPorts = append(containerPorts, corev1.ContainerPort{
			ContainerPort: int32(up.port),
			Protocol:      corev1.ProtocolTCP,
		})
		servicePorts = append(servicePorts, corev1.ServicePort{
			Name:       fmt.Sprintf("port-%d", up.port),
			Port:       int32(up.port),
			TargetPort: intstr.FromInt32(int32(up.port)),
			Protocol:   corev1.ProtocolTCP,
		})
	}

	// Build environment variables for the container.
	var envVars []corev1.EnvVar
	for k, v := range replica.EnvironmentVars {
		envVars = append(envVars, corev1.EnvVar{Name: k, Value: v})
	}

	// Deployment
	replicaCount := int32(1)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicaCount,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: replica.ContainerImage,
							Ports: containerPorts,
							Env:   envVars,
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

	// AgentEndpoint per endpoint
	for _, up := range epPorts {
		aepName := agentEndpointName(replica.ID, up.port)
		aep := &ngrokv1alpha1.AgentEndpoint{
			ObjectMeta: metav1.ObjectMeta{
				Name:      aepName,
				Namespace: r.Namespace,
				Labels:    labels,
			},
			Spec: ngrokv1alpha1.AgentEndpointSpec{
				URL: up.endpoint.URL,
				Upstream: ngrokv1alpha1.EndpointUpstream{
					URL: fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", name, r.Namespace, up.port),
				},
			},
		}
		if up.endpoint.TrafficPolicy != "" {
			aep.Spec.TrafficPolicy = &ngrokv1alpha1.TrafficPolicyCfg{
				Inline: json.RawMessage(up.endpoint.TrafficPolicy),
			}
		}
		if err := r.Create(ctx, aep); err != nil {
			return fmt.Errorf("creating agent endpoint for URL %s: %w", up.endpoint.URL, err)
		}
	}

	log.Info("Created deployment, service, and agent endpoints", "id", replica.ID, "name", name)
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
