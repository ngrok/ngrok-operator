package compute

import (
	"context"
	"encoding/base64"
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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
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
	Name          string `json:"name"`
	URL           string `json:"url"`
	TrafficPolicy string `json:"traffic_policy"`
}

type registryPullCredential struct {
	RegistryID string `json:"registry_id"`
	Server     string `json:"server"`
	Username   string `json:"username"`
	Password   string `json:"password"`
}

// computeReplica represents a replica from the runner replicas endpoint.
type computeReplica struct {
	ID                     string                  `json:"id"`
	ContainerImage         string                  `json:"container_image"`
	EnvironmentName        string                  `json:"environment_name"`
	ReplicaIndex           int32                   `json:"replica_index"`
	Endpoints              []replicaEndpoint       `json:"endpoints"`
	EnvironmentVars        map[string]string       `json:"environment_vars"`
	RegistryPullCredential *registryPullCredential `json:"registry_pull_credential"`
	// Per-replica resource requirements; zero means no request/limit.
	CPUMillicores int32 `json:"cpu_millicores"`
	MemoryMiB     int32 `json:"memory_mib"`
	GPUs          int32 `json:"gpus"`
}

// computeReplicaList represents the response from the runner replicas endpoint.
type computeReplicaList struct {
	Replicas []computeReplica `json:"replicas"`
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

func k8sName(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, "_", "-"))
}

func deploymentName(envName string, idx int32, id string) string {
	return fmt.Sprintf("%s-%d-%s", k8sName(envName), idx, k8sName(id))
}

func agentEndpointName(envName string, idx int32, epName, id string) string {
	return fmt.Sprintf("%s-%d-%s-%s", k8sName(envName), idx, k8sName(epName), k8sName(id))
}

func pullSecretName(registryID string) string {
	return "ngrok-container-registry-" + k8sName(registryID)
}

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;create;update
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

	// ComputeBaseURL is the base URL of the compute service.
	ComputeBaseURL string

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
	desired, err := r.fetchDesiredReplicas(ctx)
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
			if err := r.ensureReplicaPullSecret(ctx, replica); err != nil {
				log.Error(err, "failed to ensure app-replica pull secret", "id", id)
			}
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
		r.deleteResources(ctx, log, id, deploy.Name)
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

// fetchDesiredReplicas calls the compute service's runner replicas endpoint.
func (r *AppReplicaPoller) fetchDesiredReplicas(ctx context.Context) ([]computeReplica, error) {
	reqURL, err := url.Parse(fmt.Sprintf("%s/v1/runner-replicas?runner_id=%s", r.ComputeBaseURL, url.QueryEscape(r.runnerID)))
	if err != nil {
		return nil, fmt.Errorf("parsing compute URL: %w", err)
	}

	var resp computeReplicaList
	if err := r.NgrokBaseClient.Do(ctx, "GET", reqURL, nil, &resp); err != nil {
		return nil, fmt.Errorf("listing runner replicas: %w", err)
	}

	for i := range resp.Replicas {
		resp.Replicas[i].ID = strings.ToLower(resp.Replicas[i].ID)
	}

	return resp.Replicas, nil
}

// nvidiaGPUResource is the extended resource the NVIDIA device plugin
// advertises for schedulable GPUs.
const nvidiaGPUResource corev1.ResourceName = "nvidia.com/gpu"

// replicaResources maps a replica's requested resources onto Kubernetes
// requests/limits. CPU is request-only (compressible, so a limit would only
// cause throttling); memory sets request==limit for predictable OOM behavior;
// GPUs are a limit on the nvidia.com/gpu extended resource (Kubernetes sets the
// matching request itself). Zero values are omitted so unset means no
// request/limit.
func replicaResources(replica computeReplica) corev1.ResourceRequirements {
	requests := corev1.ResourceList{}
	limits := corev1.ResourceList{}
	if replica.CPUMillicores > 0 {
		requests[corev1.ResourceCPU] = *resource.NewMilliQuantity(int64(replica.CPUMillicores), resource.DecimalSI)
	}
	if replica.MemoryMiB > 0 {
		mem := *resource.NewQuantity(int64(replica.MemoryMiB)*1024*1024, resource.BinarySI)
		requests[corev1.ResourceMemory] = mem
		limits[corev1.ResourceMemory] = mem
	}
	if replica.GPUs > 0 {
		limits[nvidiaGPUResource] = *resource.NewQuantity(int64(replica.GPUs), resource.DecimalSI)
	}

	var res corev1.ResourceRequirements
	if len(requests) > 0 {
		res.Requests = requests
	}
	if len(limits) > 0 {
		res.Limits = limits
	}
	return res
}

// replicaTolerations returns the tolerations a replica's pod needs. GPU
// replicas tolerate the standard NVIDIA taint, since managed Kubernetes GPU
// node pools are commonly tainted to keep non-GPU workloads off them. On
// clusters without the taint (e.g. single-node k3s) the toleration is a no-op.
func replicaTolerations(replica computeReplica) []corev1.Toleration {
	if replica.GPUs <= 0 {
		return nil
	}
	return []corev1.Toleration{{
		Key:      string(nvidiaGPUResource),
		Operator: corev1.TolerationOpExists,
		Effect:   corev1.TaintEffectNoSchedule,
	}}
}

func (r *AppReplicaPoller) createResources(ctx context.Context, log logr.Logger, replica computeReplica) error {
	name := deploymentName(replica.EnvironmentName, replica.ReplicaIndex, replica.ID)
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

	if err := r.ensureReplicaPullSecret(ctx, replica); err != nil {
		return fmt.Errorf("ensuring pull secret: %w", err)
	}
	var imagePullSecrets []corev1.LocalObjectReference
	if replica.RegistryPullCredential != nil {
		imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReference{
			Name: pullSecretName(replica.RegistryPullCredential.RegistryID),
		})
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
					ImagePullSecrets: imagePullSecrets,
					Tolerations:      replicaTolerations(replica),
					Containers: []corev1.Container{
						{
							Name:      "app",
							Image:     replica.ContainerImage,
							Ports:     containerPorts,
							Env:       envVars,
							Resources: replicaResources(replica),
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
		aepName := agentEndpointName(replica.EnvironmentName, replica.ReplicaIndex, up.endpoint.Name, replica.ID)
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

func (r *AppReplicaPoller) ensureReplicaPullSecret(ctx context.Context, replica computeReplica) error {
	if replica.RegistryPullCredential == nil {
		return nil
	}
	return r.ensurePullSecret(ctx, pullSecretFor(r.Namespace, replica.RegistryPullCredential))
}

func pullSecretFor(namespace string, cred *registryPullCredential) *corev1.Secret {
	config := map[string]any{
		"auths": map[string]any{
			cred.Server: map[string]string{
				"username": cred.Username,
				"password": cred.Password,
				"auth":     base64.StdEncoding.EncodeToString([]byte(cred.Username + ":" + cred.Password)),
			},
		},
	}
	data, _ := json.Marshal(config)
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pullSecretName(cred.RegistryID),
			Namespace: namespace,
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: data,
		},
	}
}

func (r *AppReplicaPoller) ensurePullSecret(ctx context.Context, desired *corev1.Secret) error {
	var current corev1.Secret
	if err := r.Get(ctx, client.ObjectKeyFromObject(desired), &current); err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, desired)
		}
		return err
	}
	current.Type = desired.Type
	current.Data = desired.Data
	return r.Update(ctx, &current)
}

func (r *AppReplicaPoller) deleteResources(ctx context.Context, log logr.Logger, id, name string) {
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
