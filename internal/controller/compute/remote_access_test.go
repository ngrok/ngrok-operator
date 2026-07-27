package compute

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	ngrok "github.com/ngrok/ngrok-api-go/v7"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRemoteAccessRegistersBearerKeyAndStoresOnlyVerifier(t *testing.T) {
	ctx := context.Background()
	var requests []remoteAccessRequest
	httpClient := &http.Client{Transport: computeRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPut, r.Method)
		var request remoteAccessRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		requests = append(requests, request)
		if request.AccessKey != "" {
			return jsonResponse(t, remoteAccessResponse{
				Endpoint: "https://ko-abc123.k8s.compute.internal",
			}), nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})}

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&ngrokv1alpha1.KubernetesOperator{
			ObjectMeta: metav1.ObjectMeta{Name: "operator", Namespace: "ngrok"},
			Status:     ngrokv1alpha1.KubernetesOperatorStatus{ID: "k8sop_ABC123"},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "operator-compute-api", Namespace: "ngrok"},
			Status:     appsv1.DeploymentStatus{AvailableReplicas: 1},
		},
	).Build()
	const computeBaseURL = "https://compute.example"
	apiClient := ngrok.NewBaseClient(ngrok.NewClientConfig(
		"api-key", ngrok.WithBaseURL(computeBaseURL), ngrok.WithHTTPClient(httpClient),
	))
	remote := &RemoteAccess{
		Client: k8sClient, Log: logr.Discard(), NgrokBaseClient: apiClient,
		ComputeBaseURL: computeBaseURL, Namespace: "ngrok", K8sOpName: "operator",
		GatewayName: "operator-compute-api",
	}

	require.NoError(t, remote.reconcile(ctx))
	require.Len(t, requests, 2)
	require.Equal(t, "provisioning", requests[0].State)
	require.Len(t, requests[0].AccessKey, 43)
	require.Equal(t, "ready", requests[1].State)
	require.Empty(t, requests[1].AccessKey)
	require.Equal(t, "https://ko-abc123.k8s.compute.internal", requests[1].AssignedURL)

	var config corev1.ConfigMap
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{
		Name: "operator-compute-api", Namespace: "ngrok",
	}, &config))
	require.Equal(t, "https://ko-abc123.k8s.compute.internal", config.Data[remoteEndpointKey])
	hash := sha256.Sum256([]byte(requests[0].AccessKey))
	require.Equal(t, base64.RawURLEncoding.EncodeToString(hash[:]), config.Data[remoteTokenHashKey])

	var secrets corev1.SecretList
	require.NoError(t, k8sClient.List(ctx, &secrets, client.InNamespace("ngrok")))
	require.Empty(t, secrets.Items)

	restarted := &RemoteAccess{
		Client: k8sClient, Log: logr.Discard(), NgrokBaseClient: apiClient,
		ComputeBaseURL: computeBaseURL, Namespace: "ngrok", K8sOpName: "operator",
		GatewayName: "operator-compute-api",
	}
	require.NoError(t, restarted.reconcile(ctx))
	require.Len(t, requests, 4)
	require.NotEqual(t, requests[0].AccessKey, requests[2].AccessKey)
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{
		Name: "operator-compute-api", Namespace: "ngrok",
	}, &config))
	restartedHash := sha256.Sum256([]byte(requests[2].AccessKey))
	require.Equal(t, base64.RawURLEncoding.EncodeToString(restartedHash[:]), config.Data[remoteTokenHashKey])
}

func TestRemoteAccessReprovisionsDeletedConfigMap(t *testing.T) {
	ctx := context.Background()
	var requests []remoteAccessRequest
	httpClient := &http.Client{Transport: computeRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		var request remoteAccessRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		requests = append(requests, request)
		if request.AccessKey != "" {
			return jsonResponse(t, remoteAccessResponse{
				Endpoint: "https://ko-abc123.k8s.compute.internal",
			}), nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})}

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&ngrokv1alpha1.KubernetesOperator{
			ObjectMeta: metav1.ObjectMeta{Name: "operator", Namespace: "ngrok"},
			Status:     ngrokv1alpha1.KubernetesOperatorStatus{ID: "k8sop_ABC123"},
		},
	).Build()
	const computeBaseURL = "https://compute.example"
	remote := &RemoteAccess{
		Client: k8sClient, Log: logr.Discard(),
		NgrokBaseClient: ngrok.NewBaseClient(ngrok.NewClientConfig(
			"api-key", ngrok.WithBaseURL(computeBaseURL), ngrok.WithHTTPClient(httpClient),
		)),
		ComputeBaseURL: computeBaseURL, Namespace: "ngrok", K8sOpName: "operator",
		GatewayName: "operator-compute-api",
	}

	require.NoError(t, remote.reconcile(ctx))
	var config corev1.ConfigMap
	key := client.ObjectKey{Name: "operator-compute-api", Namespace: "ngrok"}
	require.NoError(t, k8sClient.Get(ctx, key, &config))
	firstHash := config.Data[remoteTokenHashKey]
	require.NoError(t, k8sClient.Delete(ctx, &config))

	require.NoError(t, remote.reconcile(ctx))
	require.NoError(t, k8sClient.Get(ctx, key, &config))
	require.NotEqual(t, firstHash, config.Data[remoteTokenHashKey])
	require.Len(t, requests, 4)
	require.NotEmpty(t, requests[2].AccessKey)
}

func TestNewAccessKey(t *testing.T) {
	key, encodedHash, err := newAccessKey()
	require.NoError(t, err)
	require.Len(t, key, 43)
	hash, err := base64.RawURLEncoding.DecodeString(encodedHash)
	require.NoError(t, err)
	require.Equal(t, sha256.Size, len(hash))
	expected := sha256.Sum256([]byte(key))
	require.Equal(t, expected[:], hash)
}

func TestValidateEndpointURL(t *testing.T) {
	require.NoError(t, validateEndpointURL("https://runner.k8s.compute.internal"))
	require.Error(t, validateEndpointURL("tls://runner.k8s.compute.internal"))
	require.Error(t, validateEndpointURL("https://example.ngrok.app"))
}

type computeRoundTripFunc func(*http.Request) (*http.Response, error)

func (f computeRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(t *testing.T, value any) *http.Response {
	t.Helper()
	body, err := json.Marshal(value)
	require.NoError(t, err)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(body))),
	}
}
