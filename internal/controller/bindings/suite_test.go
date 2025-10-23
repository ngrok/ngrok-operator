/*
MIT License

Copyright (c) 2025 ngrok, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package bindings

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ngrok/ngrok-api-go/v7"
	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
	"github.com/ngrok/ngrok-operator/internal/testutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment

	ctx    context.Context
	cancel context.CancelFunc

	// Mock clientset for ngrok API
	mockClientset *nmockapi.Clientset

	// Test manager and reconcilers
	k8sManager       ctrl.Manager
	pollerController *BoundEndpointPoller

	kginkgo *testutils.KGinkgo
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "BoundEndpoint Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.Background())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "helm", "ngrok-operator", "templates", "crds")},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = bindingsv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = ngrokv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = ingressv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Initialize test helper closures
	kginkgo = testutils.NewKGinkgo(k8sClient)

	// Create the operator namespace that the poller will use
	operatorNs := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ngrok-op",
		},
	}
	err = k8sClient.Create(ctx, operatorNs)
	Expect(err).NotTo(HaveOccurred())

	// Create mock clientset
	mockClientset = nmockapi.NewClientset()

	// Create manager for controller runtime tests
	k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: server.Options{
			BindAddress: "0",
		},
	})
	Expect(err).NotTo(HaveOccurred())

	// Setup BoundEndpoint controller
	controllerReconciler := &BoundEndpointReconciler{
		Client:        k8sManager.GetClient(),
		Scheme:        k8sManager.GetScheme(),
		Log:           logf.Log.WithName("boundendpoint-controller"),
		Recorder:      k8sManager.GetEventRecorderFor("boundendpoint-controller"),
		ClusterDomain: "cluster.local",
	}
	err = controllerReconciler.SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	// Setup BoundEndpoint poller with very long interval (we'll trigger manually)
	pollerController = &BoundEndpointPoller{
		Client:                       k8sManager.GetClient(),
		Log:                          logf.Log.WithName("boundendpoint-poller"),
		Recorder:                     k8sManager.GetEventRecorderFor("boundendpoint-poller"),
		Namespace:                    "ngrok-op",
		KubernetesOperatorConfigName: "test-k8sop",
		NgrokClientset:               mockClientset,
		PollingInterval:              1 * time.Hour, // Long interval - we'll trigger manually
		PortRange: PortRangeConfig{
			Min: 10000,
			Max: 20000,
		},
		portAllocator: newPortBitmap(10000, 20000),
		koId:          "ko_test123", // Set K8s operator ID for testing
	}

	// Create a default KubernetesOperator in the mock for the poller to use
	// The poller needs a K8s operator to exist to query bound endpoints
	_, err = mockClientset.KubernetesOperators().Create(ctx, &ngrok.KubernetesOperatorCreate{})
	Expect(err).NotTo(HaveOccurred())

	// Start the manager
	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).NotTo(HaveOccurred())
	}()

	// Wait for manager cache to sync
	Eventually(func() bool {
		cacheCtx, cacheCancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cacheCancel()
		return k8sManager.GetCache().WaitForCacheSync(cacheCtx)
	}, 30*time.Second, 100*time.Millisecond).Should(BeTrue())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

// Test helper functions

// triggerPoller manually triggers a poll cycle
func triggerPoller(ctx context.Context) error {
	return pollerController.reconcileBoundEndpointsFromAPI(ctx)
}

// setMockEndpoints sets the endpoints that the mock API will return
func setMockEndpoints(endpoints []ngrok.Endpoint) {
	mockClientset.KubernetesOperators().(*nmockapi.KubernetesOperatorsClient).SetBoundEndpoints(endpoints)
}

// resetMockEndpoints clears all mock endpoints
func resetMockEndpoints() {
	mockClientset.KubernetesOperators().(*nmockapi.KubernetesOperatorsClient).ResetBoundEndpoints()
}
