package bindings

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"

	"github.com/ngrok/ngrok-operator/pkg/bindingsdriver"
	"k8s.io/apimachinery/pkg/util/rand"
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

var _ = Describe("ForwarderReconciler field indexer integration", func() {
	const ip = "10.2.2.2"

	It("registers the pod IP field indexer in SetupWithManager and lists pods by IP", func() {
		if cfg == nil {
			Skip("envtest cfg not available; skipping integration-style test")
		}

		// Create a dedicated manager for this test so we can register the indexer
		// before the cache starts and avoid interference from the package manager.
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: k8sManager.GetScheme()})
		Expect(err).ToNot(HaveOccurred())

		fr := &ForwarderReconciler{
			Client:                 mgr.GetClient(),
			Log:                    logr.Discard(),
			Scheme:                 mgr.GetScheme(),
			Recorder:               mgr.GetEventRecorderFor("bindings-forwarder-controller-test"),
			BindingsDriver:         bindingsdriver.New(),
			KubernetesOperatorName: "test-op",
		}
		Expect(fr.SetupWithManager(mgr)).To(Succeed())

		// start the manager
		ctx2, cancel2 := context.WithCancel(context.Background())
		defer cancel2()
		go func() { _ = mgr.Start(ctx2) }()

		// wait for cache to sync
		Eventually(func() bool {
			cctx, ccancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer ccancel()
			return mgr.GetCache().WaitForCacheSync(cctx)
		}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())

		// create namespace and pod via mgr client so the manager's cache can observe them
		ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-" + rand.String(6)}}
		Expect(mgr.GetClient().Create(ctx, ns)).To(Succeed())

		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pod-" + rand.String(6), Namespace: ns.Name},
			Spec:       v1.PodSpec{Containers: []v1.Container{{Name: "test-container", Image: "nginx"}}},
		}
		Expect(mgr.GetClient().Create(ctx, pod)).To(Succeed())

		// set Pod status explicitly so the field indexer (which indexes pod.Status.PodIP)
		// can observe the IP.
		got := &v1.Pod{}
		// Use API reader to avoid cached client NotFound races immediately after Create
		Expect(mgr.GetAPIReader().Get(ctx, client.ObjectKey{Namespace: pod.Namespace, Name: pod.Name}, got)).To(Succeed())
		got.Status = v1.PodStatus{PodIP: ip}
		Expect(mgr.GetClient().Status().Update(ctx, got)).To(Succeed())

		// Wait until the manager cache/indexer exposes the pod via the field index
		Eventually(func() bool {
			list := &v1.PodList{}
			if err := mgr.GetClient().List(ctx, list, client.MatchingFields{"status.podIP": ip}); err != nil {
				return false
			}
			return len(list.Items) > 0 && list.Items[0].Status.PodIP == ip
		}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())
	})
})
