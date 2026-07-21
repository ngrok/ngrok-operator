package bindings

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	crand "crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"

	"github.com/go-logr/logr"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"time"

	utilrand "k8s.io/apimachinery/pkg/util/rand"
)

func TestGetIngressEndpointWithFallback(t *testing.T) {
	cases := []struct {
		input            string
		expectedEndpoint string
		shouldErr        bool
	}{
		{
			"",
			"",
			true,
		},
		{
			"foo.example.com",
			"foo.example.com:443",
			false,
		},
		{
			"foo.example.com:443",
			"foo.example.com:443",
			false,
		},
		{
			"foo.example.com:443:1234",
			"",
			true,
		},
	}

	for _, c := range cases {
		ingressEndpoint, err := getIngressEndpointWithFallback(c.input, logr.Discard())
		assert.Equal(t, c.expectedEndpoint, ingressEndpoint)
		if c.shouldErr {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
	}
}

func TestLoadTLSCertificate(t *testing.T) {
	certPEM, keyPEM, err := generateTestTLSMaterial()
	assert.NoError(t, err)

	scheme := runtime.NewScheme()
	assert.NoError(t, v1.AddToScheme(scheme))
	assert.NoError(t, ngrokv1alpha1.AddToScheme(scheme))

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tls-secret",
			Namespace: "default",
		},
		Type: v1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": certPEM,
			"tls.key": keyPEM,
		},
	}

	reconciler := &ForwarderReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(),
	}

	cert, err := reconciler.loadTLSCertificate(context.Background(), "default", "tls-secret")
	assert.NoError(t, err)
	assert.NotEmpty(t, cert.Certificate)
}

func generateTestTLSMaterial() (certPEM, keyPEM []byte, err error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), crand.Reader)
	if err != nil {
		return nil, nil, err
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}

	der, err := x509.CreateCertificate(crand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	keyBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, nil, err
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	return certPEM, keyPEM, nil
}

var _ = Describe("podIdentityFromPod", func() {
	var (
		pod *v1.Pod = &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				UID:         "uid123",
				Name:        "pod1",
				Namespace:   "default",
				Annotations: map[string]string{},
			},
		}
	)

	It("prunes non-prefixed annotations and returns PodIdentity", func() {
		pod.Annotations = map[string]string{
			"k8s.ngrok.com/keep": "yes",
			"some.other/strip":   "no",
		}

		pid := podIdentityFromPod(pod)
		Expect(pid).To(Not(BeNil()))
		Expect(pid.Uid).To(Equal("uid123"))
		Expect(pid.Name).To(Equal("pod1"))
		Expect(pid.Namespace).To(Equal("default"))
		Expect(pid.Annotations).To(Not(BeNil()))
		Expect(pid.Annotations).To(HaveKey("k8s.ngrok.com/keep"))
		Expect(pid.Annotations).To(Not(HaveKey("some.other/strip")))
	})

	It("keeps both canonical and legacy prefixed annotations", func() {
		pod.Annotations = map[string]string{
			"ngrok.com/foo":     "new",
			"k8s.ngrok.com/bar": "old",
			"unrelated.io/baz":  "skip",
		}

		pid := podIdentityFromPod(pod)
		Expect(pid).To(Not(BeNil()))
		Expect(pid.Annotations).To(Equal(map[string]string{
			"ngrok.com/foo":     "new",
			"k8s.ngrok.com/bar": "old",
		}))
	})
})

var _ = Describe("ForwarderReconciler field indexer integration", func() {
	const ip = "10.2.2.2"

	It("registers the pod IP field indexer in SetupWithManager and lists pods by IP", func() {
		// create namespace and pod via mgr client so the manager's cache can observe them
		ns := &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-" + utilrand.String(6)},
		}
		Expect(k8sManager.GetClient().Create(ctx, ns)).To(Succeed())

		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pod-" + utilrand.String(6), Namespace: ns.Name},
			Spec:       v1.PodSpec{Containers: []v1.Container{{Name: "test-container", Image: "nginx"}}},
		}
		Expect(k8sManager.GetClient().Create(ctx, pod)).To(Succeed())

		// set Pod status explicitly so the field indexer (which indexes pod.Status.PodIP) can observe the IP.
		podRetrieved := &v1.Pod{}
		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Namespace: pod.Namespace, Name: pod.Name}, podRetrieved)).To(Succeed())
		podRetrieved.Status = v1.PodStatus{PodIP: ip}
		Expect(k8sManager.GetClient().Status().Update(ctx, podRetrieved)).To(Succeed())

		// Wait until the manager cache/indexer exposes the pod via the field index
		Eventually(func() bool {
			list := &v1.PodList{}
			if err := k8sManager.GetClient().List(ctx, list, client.MatchingFields{"status.podIP": ip}); err != nil {
				return false
			}
			return len(list.Items) > 0 && list.Items[0].Status.PodIP == ip
		}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())
	})
})
