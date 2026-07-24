package compute

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/go-logr/logr"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAppReplicaPollerCreateResourcesWithPullSecret(t *testing.T) {
	ctx := context.Background()
	poller := newAppReplicaPollerTestHarness(t)

	replica := computeReplica{
		ID:              "dplyrep_ABC",
		ContainerImage:  "ghcr.io/acme/app:latest",
		EnvironmentName: "Prod_App",
		ReplicaIndex:    0,
		Endpoints: []replicaEndpoint{{
			Name: "web",
			URL:  "https://example.ngrok.app:8443",
		}},
		RegistryPullCredential: &registryPullCredential{
			RegistryID: "creg_ABC",
			Server:     "ghcr.io",
			Username:   "x-access-token",
			Password:   "pat-123",
		},
	}

	require.NoError(t, poller.createResources(ctx, logr.Discard(), replica))

	var secret corev1.Secret
	require.NoError(t, poller.Get(ctx, client.ObjectKey{Namespace: "default", Name: "ngrok-container-registry-creg-abc"}, &secret))
	require.Equal(t, corev1.SecretTypeDockerConfigJson, secret.Type)

	var dockerConfig struct {
		Auths map[string]struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Auth     string `json:"auth"`
		} `json:"auths"`
	}
	require.NoError(t, json.Unmarshal(secret.Data[corev1.DockerConfigJsonKey], &dockerConfig))
	ghcr := dockerConfig.Auths["ghcr.io"]
	require.Equal(t, "x-access-token", ghcr.Username)
	require.Equal(t, "pat-123", ghcr.Password)
	require.Equal(t, base64.StdEncoding.EncodeToString([]byte("x-access-token:pat-123")), ghcr.Auth)

	var deploy appsv1.Deployment
	require.NoError(t, poller.Get(ctx, client.ObjectKey{Namespace: "default", Name: "prod-app-0-dplyrep-abc"}, &deploy))
	require.Equal(t, []corev1.LocalObjectReference{{Name: "ngrok-container-registry-creg-abc"}}, deploy.Spec.Template.Spec.ImagePullSecrets)
	require.Equal(t, "ghcr.io/acme/app:latest", deploy.Spec.Template.Spec.Containers[0].Image)
}

func TestAppReplicaPollerCreateResourcesWithoutPullSecret(t *testing.T) {
	ctx := context.Background()
	poller := newAppReplicaPollerTestHarness(t)

	replica := computeReplica{
		ID:              "dplyrep_PUBLIC",
		ContainerImage:  "docker.io/library/nginx:latest",
		EnvironmentName: "Public",
		ReplicaIndex:    1,
		Endpoints: []replicaEndpoint{{
			Name: "web",
			URL:  "https://example.ngrok.app:8080",
		}},
	}

	require.NoError(t, poller.createResources(ctx, logr.Discard(), replica))

	var deploy appsv1.Deployment
	require.NoError(t, poller.Get(ctx, client.ObjectKey{Namespace: "default", Name: "public-1-dplyrep-public"}, &deploy))
	require.Empty(t, deploy.Spec.Template.Spec.ImagePullSecrets)

	var secrets corev1.SecretList
	require.NoError(t, poller.List(ctx, &secrets, client.InNamespace("default")))
	require.Empty(t, secrets.Items)
}

func TestAppReplicaPollerCreateResourcesWithGPUAndLimits(t *testing.T) {
	ctx := context.Background()
	poller := newAppReplicaPollerTestHarness(t)

	replica := computeReplica{
		ID:              "dplyrep_GPU",
		ContainerImage:  "ghcr.io/acme/model:latest",
		EnvironmentName: "Train",
		ReplicaIndex:    0,
		Endpoints: []replicaEndpoint{{
			Name: "web",
			URL:  "https://example.ngrok.app:8080",
		}},
		CPUMillicores: 500,
		MemoryMiB:     2048,
		GPUs:          1,
	}

	require.NoError(t, poller.createResources(ctx, logr.Discard(), replica))

	var deploy appsv1.Deployment
	require.NoError(t, poller.Get(ctx, client.ObjectKey{Namespace: "default", Name: "train-0-dplyrep-gpu"}, &deploy))

	res := deploy.Spec.Template.Spec.Containers[0].Resources
	const mib = int64(1024 * 1024)
	require.Equal(t, "500m", res.Requests.Cpu().String())
	require.Equal(t, 2048*mib, res.Requests.Memory().Value())
	require.Equal(t, 2048*mib, res.Limits.Memory().Value())
	gpuLimit := res.Limits[nvidiaGPUResource]
	require.Equal(t, int64(1), gpuLimit.Value())
	// CPU has no limit (compressible).
	_, hasCPULimit := res.Limits[corev1.ResourceCPU]
	require.False(t, hasCPULimit)

	require.Equal(t, []corev1.Toleration{{
		Key:      "nvidia.com/gpu",
		Operator: corev1.TolerationOpExists,
		Effect:   corev1.TaintEffectNoSchedule,
	}}, deploy.Spec.Template.Spec.Tolerations)
}

func TestAppReplicaPollerCreateResourcesUnsetOmitsRequirements(t *testing.T) {
	ctx := context.Background()
	poller := newAppReplicaPollerTestHarness(t)

	replica := computeReplica{
		ID:              "dplyrep_NORES",
		ContainerImage:  "docker.io/library/nginx:latest",
		EnvironmentName: "Public",
		ReplicaIndex:    0,
		Endpoints: []replicaEndpoint{{
			Name: "web",
			URL:  "https://example.ngrok.app:8080",
		}},
	}

	require.NoError(t, poller.createResources(ctx, logr.Discard(), replica))

	var deploy appsv1.Deployment
	require.NoError(t, poller.Get(ctx, client.ObjectKey{Namespace: "default", Name: "public-0-dplyrep-nores"}, &deploy))
	require.Empty(t, deploy.Spec.Template.Spec.Containers[0].Resources.Requests)
	require.Empty(t, deploy.Spec.Template.Spec.Containers[0].Resources.Limits)
	require.Empty(t, deploy.Spec.Template.Spec.Tolerations)
}

func newAppReplicaPollerTestHarness(t *testing.T) *AppReplicaPoller {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))

	return &AppReplicaPoller{
		Client:    fake.NewClientBuilder().WithScheme(scheme).Build(),
		Namespace: "default",
	}
}
