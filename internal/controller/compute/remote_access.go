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
	ngrok "github.com/ngrok/ngrok-api-go/v7"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	remoteEndpointKey  = "endpoint"
	remoteTokenHashKey = "access-key-sha256"
)

type remoteAccessRequest struct {
	State       string `json:"state"`
	AccessKey   string `json:"access_key,omitempty"`
	Endpoint    string `json:"endpoint,omitempty"`
	AssignedURL string `json:"assigned_url,omitempty"`
}

type remoteAccessResponse struct {
	Endpoint string `json:"endpoint"`
}

// RemoteAccess provisions an ephemeral bearer key and the internal endpoint
// configuration consumed by the Compute Kubernetes API gateway.
type RemoteAccess struct {
	client.Client
	Log             logr.Logger
	NgrokBaseClient *ngrok.BaseClient
	ComputeBaseURL  string
	Namespace       string
	K8sOpName       string
	GatewayName     string
	Interval        time.Duration

	lastReportedState string
	registered        bool
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

	if !r.registered {
		if err := r.provision(ctx, ko.Status.ID); err != nil {
			return err
		}
	}

	var endpointConfig corev1.ConfigMap
	endpointConfigKey := client.ObjectKey{Namespace: r.Namespace, Name: r.GatewayName}
	if err := r.Get(ctx, endpointConfigKey, &endpointConfig); apierrors.IsNotFound(err) {
		r.registered = false
		if err := r.provision(ctx, ko.Status.ID); err != nil {
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

	var deployment appsv1.Deployment
	err := r.Get(ctx, client.ObjectKey{Namespace: r.Namespace, Name: r.GatewayName}, &deployment)
	if client.IgnoreNotFound(err) != nil {
		return err
	}
	ready := err == nil && deployment.Status.AvailableReplicas > 0
	state := "provisioning"
	assignedURL := ""
	if ready {
		state = "ready"
		assignedURL = endpoint
	}
	if err := r.register(ctx, ko.Status.ID, remoteAccessRequest{
		State: state, Endpoint: endpoint, AssignedURL: assignedURL,
	}, nil); err != nil {
		return err
	}
	if state != r.lastReportedState {
		r.Log.Info("reported remote Kubernetes API access state",
			"runner_id", ko.Status.ID,
			"state", state,
			"endpoint", endpoint,
			"assigned_url", assignedURL,
		)
		r.lastReportedState = state
	}
	return nil
}

func (r *RemoteAccess) provision(ctx context.Context, runnerID string) error {
	accessKey, accessKeyHash, err := newAccessKey()
	if err != nil {
		return err
	}
	var registration remoteAccessResponse
	if err := r.register(ctx, runnerID, remoteAccessRequest{
		State: "provisioning", AccessKey: accessKey,
	}, &registration); err != nil {
		return err
	}
	if err := validateEndpointURL(registration.Endpoint); err != nil {
		return fmt.Errorf("compute remote-access registration returned invalid endpoint: %w", err)
	}

	var endpointConfig corev1.ConfigMap
	key := client.ObjectKey{Namespace: r.Namespace, Name: r.GatewayName}
	err = r.Get(ctx, key, &endpointConfig)
	switch {
	case err == nil:
		endpointConfig.Data = map[string]string{
			remoteEndpointKey:  registration.Endpoint,
			remoteTokenHashKey: accessKeyHash,
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
				remoteEndpointKey:  registration.Endpoint,
				remoteTokenHashKey: accessKeyHash,
			},
		}
		if err := r.Create(ctx, &endpointConfig); err != nil {
			return err
		}
	}

	r.registered = true
	r.Log.Info("provisioned remote Kubernetes API access",
		"runner_id", runnerID,
		"endpoint", registration.Endpoint,
		"gateway", r.GatewayName,
	)
	return nil
}

func (r *RemoteAccess) register(ctx context.Context, runnerID string, request remoteAccessRequest, response *remoteAccessResponse) error {
	u, err := url.Parse(fmt.Sprintf("%s/v1/runners/%s/kubernetes-access", r.ComputeBaseURL, url.PathEscape(runnerID)))
	if err != nil {
		return err
	}
	// Passing a typed nil pointer as interface{} produces a non-nil interface.
	// Pass a literal nil so ngrok-api-go does not try to decode an intentionally
	// empty lifecycle response.
	if response == nil {
		return r.NgrokBaseClient.Do(ctx, "PUT", u, request, nil)
	}
	return r.NgrokBaseClient.Do(ctx, "PUT", u, request, response)
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
