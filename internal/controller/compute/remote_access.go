package compute

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-logr/logr"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	remoteEndpointKey  = "endpoint"
	remoteTokenHashKey = "access-key-sha256"

	// accessRevisionAnnotation fingerprints the endpoint and key verifier the
	// gateway is meant to serve. The gateway reads both files once at startup,
	// so republishing them means nothing until its pods restart; stamping the
	// fingerprint on the pod template is what restarts them.
	accessRevisionAnnotation = "ngrok.k8s.ngrok.com/compute-access-revision"
)

// RemoteAccess provisions an ephemeral bearer key and the internal endpoint
// configuration consumed by the Compute Kubernetes API gateway. Access is
// published against the ship runner identity, so the operator registers as a
// runner (idempotent on the operator ID) before its first publication.
type RemoteAccess struct {
	client.Client
	Log              logr.Logger
	RunnerAPI        *RunnerClient
	Namespace        string
	K8sOpName        string
	GatewayName      string
	ReplicaNamespace string
	Interval         time.Duration

	lastReportedReady *bool
	registered        bool
	runnerID          string
}

// NeedLeaderElection ensures only one api-manager instance provisions and
// reports the runner endpoint.
func (*RemoteAccess) NeedLeaderElection() bool { return true }

func (r *RemoteAccess) Start(ctx context.Context) error {
	if r.Interval == 0 {
		r.Interval = 10 * time.Second
	}
	ticker := time.NewTicker(r.Interval)
	defer ticker.Stop()
	for {
		if err := r.reconcile(ctx); err != nil {
			r.Log.Error(err, "remote Kubernetes API access reconcile failed")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (r *RemoteAccess) reconcile(ctx context.Context) error {
	var ko ngrokv1alpha1.KubernetesOperator
	if err := r.Get(ctx, client.ObjectKey{Namespace: r.Namespace, Name: r.K8sOpName}, &ko); err != nil {
		return client.IgnoreNotFound(err)
	}
	if ko.Status.ID == "" {
		return nil
	}

	if r.runnerID == "" {
		runnerID, err := r.RunnerAPI.Register(ctx, ko.Status.ID)
		if err != nil {
			return fmt.Errorf("register compute runner: %w", err)
		}
		r.runnerID = runnerID
	}

	// The gateway Deployment is resolved first: it owns the endpoint ConfigMap
	// and carries the annotation that rolls its pods onto a new one.
	gateway, err := r.gatewayDeployment(ctx)
	if err != nil {
		return err
	}

	if !r.registered {
		if err := r.provision(ctx, r.runnerID, gateway); err != nil {
			return err
		}
	}

	var endpointConfig corev1.ConfigMap
	endpointConfigKey := client.ObjectKey{Namespace: r.Namespace, Name: r.GatewayName}
	if err := r.Get(ctx, endpointConfigKey, &endpointConfig); apierrors.IsNotFound(err) {
		r.registered = false
		if err := r.provision(ctx, r.runnerID, gateway); err != nil {
			return err
		}
		if err := r.Get(ctx, endpointConfigKey, &endpointConfig); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	endpoint := endpointConfig.Data[remoteEndpointKey]
	if endpoint == "" {
		return fmt.Errorf("compute remote-access ConfigMap %q is missing %q", r.GatewayName, remoteEndpointKey)
	}

	// Readiness is a claim about the pods Ship will actually reach, not about
	// the Deployment existing. Reporting ready while the gateway still serves
	// a previous endpoint or key sends Ship into server-side reconcile against
	// something unreachable, where the only symptom is replicas that never
	// start.
	ready := false
	if gateway != nil {
		revision := accessRevision(endpointConfig.Data)
		if gateway.Spec.Template.Annotations[accessRevisionAnnotation] == revision {
			ready = gatewayRolloutComplete(gateway)
		} else if err := r.stampGateway(ctx, gateway, revision); err != nil {
			return err
		}
	}
	// Access rides the status exchange, so restating readiness is also this
	// runner's heartbeat: ship learns we are alive from the same call.
	// Description and version are restated rather than left to registration:
	// registration is idempotent, so a runner that first joined without them
	// would otherwise never acquire them.
	if _, err := r.RunnerAPI.Status(ctx, r.runnerID, RunnerStatusRequest{
		Description:               r.RunnerAPI.Description,
		Version:                   r.RunnerAPI.Version,
		KubernetesAccessNamespace: r.ReplicaNamespace,
		KubernetesAccessReady:     ready,
	}); err != nil {
		return err
	}
	if r.lastReportedReady == nil || *r.lastReportedReady != ready {
		r.Log.Info("reported remote Kubernetes API access state",
			"runner_id", r.runnerID,
			"ready", ready,
			"endpoint", endpoint,
		)
		r.lastReportedReady = &ready
	}
	return nil
}

// gatewayDeployment loads the gateway, treating absence as "not deployed yet"
// rather than an error: remote access is published from the api-manager, which
// can be running before the gateway lands.
func (r *RemoteAccess) gatewayDeployment(ctx context.Context) (*appsv1.Deployment, error) {
	var gateway appsv1.Deployment
	err := r.Get(ctx, client.ObjectKey{Namespace: r.Namespace, Name: r.GatewayName}, &gateway)
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &gateway, nil
}

// accessRevision fingerprints the published access configuration.
//
// Derived from the ConfigMap rather than held in memory, so a manager that
// restarts mid-rollout computes the same value the gateway is converging on
// instead of forcing another roll. Both inputs are already public — the
// endpoint hostname and a SHA-256 verifier — so the fingerprint carries no
// secret.
func accessRevision(data map[string]string) string {
	sum := sha256.Sum256([]byte(data[remoteEndpointKey] + "\n" + data[remoteTokenHashKey]))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// gatewayRolloutComplete reports whether every live gateway pod is running the
// current pod template and at least one of them is serving.
//
// Status is only comparable to spec once the Deployment controller has observed
// the current generation. After that, a finished rollout means no pod predates
// the template, so an available replica is a replica serving the current files.
func gatewayRolloutComplete(gateway *appsv1.Deployment) bool {
	return gateway.Status.ObservedGeneration >= gateway.Generation &&
		gateway.Status.UpdatedReplicas == gateway.Status.Replicas &&
		gateway.Status.AvailableReplicas > 0
}

// stampGateway records the revision on the gateway's pod template, which rolls
// its pods onto the newly published endpoint and key.
//
// Also the repair path: a stamp lost to a competing write is reapplied on the
// next tick instead of leaving the gateway pinned to files nobody honours.
func (r *RemoteAccess) stampGateway(ctx context.Context, gateway *appsv1.Deployment, revision string) error {
	if gateway.Spec.Template.Annotations == nil {
		gateway.Spec.Template.Annotations = map[string]string{}
	}
	gateway.Spec.Template.Annotations[accessRevisionAnnotation] = revision
	if err := r.Update(ctx, gateway); err != nil {
		return fmt.Errorf("roll compute API gateway onto published access: %w", err)
	}
	r.Log.Info("rolling compute API gateway onto published access",
		"gateway", r.GatewayName,
		"revision", revision,
	)
	return nil
}

// ownConfigMap ties the endpoint ConfigMap to the gateway Deployment that
// consumes it.
//
// Nothing else reads it, and unowned it outlives `helm uninstall` — Helm only
// deletes what it templated. A leftover is worse than a missing one: a missing
// ConfigMap blocks the next install's gateway until the manager writes a fresh
// one, while a stale one lets it boot and serve a decommissioned runner's
// endpoint.
func (r *RemoteAccess) ownConfigMap(endpointConfig *corev1.ConfigMap, gateway *appsv1.Deployment) error {
	if gateway == nil {
		return nil
	}
	if err := controllerutil.SetOwnerReference(gateway, endpointConfig, r.Scheme()); err != nil {
		return fmt.Errorf("own compute remote-access ConfigMap: %w", err)
	}
	return nil
}

func (r *RemoteAccess) provision(ctx context.Context, runnerID string, gateway *appsv1.Deployment) error {
	accessKey, accessKeyHash, err := newAccessKey()
	if err != nil {
		return err
	}
	// Minting withholds readiness: the gateway cannot be serving a key that was
	// generated a line ago, and claiming otherwise is what sends ship
	// reconciling against an endpoint nobody answers.
	registration, err := r.RunnerAPI.Status(ctx, runnerID, RunnerStatusRequest{
		KubernetesAccessKey:       accessKey,
		KubernetesAccessNamespace: r.ReplicaNamespace,
	})
	if err != nil {
		return err
	}
	if err := validateEndpointURL(registration.KubernetesAccessEndpoint); err != nil {
		return fmt.Errorf("compute remote-access registration returned invalid endpoint: %w", err)
	}

	var endpointConfig corev1.ConfigMap
	key := client.ObjectKey{Namespace: r.Namespace, Name: r.GatewayName}
	err = r.Get(ctx, key, &endpointConfig)
	switch {
	case err == nil:
		endpointConfig.Data = map[string]string{
			remoteEndpointKey:  registration.KubernetesAccessEndpoint,
			remoteTokenHashKey: accessKeyHash,
		}
		if err := r.ownConfigMap(&endpointConfig, gateway); err != nil {
			return err
		}
		if err := r.Update(ctx, &endpointConfig); err != nil {
			return err
		}
	case !apierrors.IsNotFound(err):
		return err
	default:
		endpointConfig = corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: r.GatewayName, Namespace: r.Namespace},
			Data: map[string]string{
				remoteEndpointKey:  registration.KubernetesAccessEndpoint,
				remoteTokenHashKey: accessKeyHash,
			},
		}
		if err := r.ownConfigMap(&endpointConfig, gateway); err != nil {
			return err
		}
		if err := r.Create(ctx, &endpointConfig); err != nil {
			return err
		}
	}

	r.registered = true
	r.Log.Info("provisioned remote Kubernetes API access",
		"runner_id", runnerID,
		"endpoint", registration.KubernetesAccessEndpoint,
		"gateway", r.GatewayName,
	)
	return nil
}

func newAccessKey() (accessKey, accessKeyHash string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("generate compute remote-access key: %w", err)
	}
	accessKey = base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(accessKey))
	return accessKey, base64.RawURLEncoding.EncodeToString(hash[:]), nil
}

func validateEndpointURL(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	if u.Scheme != "https" ||
		u.Hostname() == "" ||
		!strings.HasSuffix(u.Hostname(), ".internal") ||
		(u.Path != "" && u.Path != "/") ||
		u.RawQuery != "" ||
		u.Fragment != "" ||
		u.User != nil {
		return fmt.Errorf("endpoint must be an https:// URL with a .internal hostname")
	}
	return nil
}
