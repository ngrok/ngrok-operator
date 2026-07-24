package compute

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"net/url"
	"time"

	"github.com/go-logr/logr"
	ngrok "github.com/ngrok/ngrok-api-go/v7"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	remoteEndpointName = "ngrok-compute-kubernetes-api"
	remoteTLSSecret    = "ngrok-compute-kubernetes-api-tls"
	remoteClientCA     = "ngrok-compute-client-ca"
)

type remoteAccessRequest struct {
	State       string `json:"state"`
	CSR         string `json:"csr,omitempty"`
	Endpoint    string `json:"endpoint,omitempty"`
	AssignedURL string `json:"assigned_url,omitempty"`
}

type remoteAccessResponse struct {
	Endpoint          string `json:"endpoint"`
	ServerCertificate string `json:"server_certificate"`
	ClientCA          string `json:"client_ca"`
}

// RemoteAccess provisions the mTLS AgentEndpoint and reports its lifecycle to Compute.
type RemoteAccess struct {
	client.Client
	Log             logr.Logger
	NgrokBaseClient *ngrok.BaseClient
	ComputeBaseURL  string
	Namespace       string
	K8sOpName       string
	GatewayService  string
	Interval        time.Duration

	lastReportedState string
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

	var endpoint ngrokv1alpha1.AgentEndpoint
	err := r.Get(ctx, client.ObjectKey{Namespace: r.Namespace, Name: remoteEndpointName}, &endpoint)
	if client.IgnoreNotFound(err) != nil {
		return err
	}
	if err == nil {
		ready := meta.IsStatusConditionTrue(endpoint.Status.Conditions, "Ready")
		state := "provisioning"
		if ready {
			state = "ready"
		}
		if err := r.register(ctx, ko.Status.ID, remoteAccessRequest{
			State: state, Endpoint: endpoint.Spec.URL, AssignedURL: endpoint.Status.AssignedURL,
		}, nil); err != nil {
			return err
		}
		if state != r.lastReportedState {
			r.Log.Info("reported remote Kubernetes API access state",
				"runner_id", ko.Status.ID,
				"state", state,
				"endpoint", endpoint.Spec.URL,
				"assigned_url", endpoint.Status.AssignedURL,
			)
			r.lastReportedState = state
		}
		return nil
	}

	keyPEM, csrPEM, err := newEndpointCSR(ko.Status.ID)
	if err != nil {
		return err
	}
	var registration remoteAccessResponse
	if err := r.register(ctx, ko.Status.ID, remoteAccessRequest{State: "provisioning", CSR: string(csrPEM)}, &registration); err != nil {
		return err
	}
	if registration.Endpoint == "" || registration.ServerCertificate == "" || registration.ClientCA == "" {
		return fmt.Errorf("compute remote-access registration returned incomplete credentials")
	}
	if err := r.Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: remoteTLSSecret, Namespace: r.Namespace},
		Type:       corev1.SecretTypeTLS,
		Data:       map[string][]byte{corev1.TLSPrivateKeyKey: keyPEM, corev1.TLSCertKey: []byte(registration.ServerCertificate)},
	}); client.IgnoreAlreadyExists(err) != nil {
		return err
	}
	if err := r.Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: remoteClientCA, Namespace: r.Namespace},
		Data:       map[string][]byte{"ca.crt": []byte(registration.ClientCA)},
	}); client.IgnoreAlreadyExists(err) != nil {
		return err
	}
	if err := r.Create(ctx, &ngrokv1alpha1.AgentEndpoint{
		ObjectMeta: metav1.ObjectMeta{Name: remoteEndpointName, Namespace: r.Namespace},
		Spec: ngrokv1alpha1.AgentEndpointSpec{
			URL:      registration.Endpoint,
			Upstream: ngrokv1alpha1.EndpointUpstream{URL: fmt.Sprintf("http://%s.%s.svc.cluster.local:8080", r.GatewayService, r.Namespace)},
			TLSTermination: &ngrokv1alpha1.EndpointTLSTermination{
				ServerCertificateRef: ngrokv1alpha1.K8sObjectRef{Name: remoteTLSSecret},
				MutualTLS: &ngrokv1alpha1.EndpointMutualTLS{
					ClientCAsRef: ngrokv1alpha1.K8sObjectRef{Name: remoteClientCA},
					Mode:         ngrokv1alpha1.EndpointMutualTLSModeRequire,
				},
			},
		},
	}); err != nil {
		return err
	}
	r.Log.Info("provisioned remote Kubernetes API access",
		"runner_id", ko.Status.ID,
		"endpoint", registration.Endpoint,
		"agent_endpoint", remoteEndpointName,
		"gateway_service", r.GatewayService,
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

func newEndpointCSR(runnerID string) (keyPEM, csrPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: runnerID},
	}, key)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}), nil
}
