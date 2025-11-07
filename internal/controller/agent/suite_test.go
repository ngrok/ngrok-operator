package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/testutils"
	"github.com/ngrok/ngrok-operator/pkg/agent"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

	// Env test manager and mock driver for controller runtime tests
	envMgr        ctrl.Manager
	envCtx        context.Context
	envCancel     context.CancelFunc
	envMockDriver *agent.MockAgentDriver

	kginkgo *testutils.KGinkgo
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AgentEndpoint Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(
		zap.New(
			zap.WriteTo(GinkgoWriter),
			zap.UseDevMode(true),
		),
	)

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "helm", "ngrok-operator", "templates", "crds")},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = ngrokv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = ingressv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
	kginkgo = testutils.NewKGinkgo(k8sClient)

	// Set up environment test manager for controller runtime tests
	envCtx, envCancel = context.WithCancel(context.Background())

	// Initialize mock driver
	envMockDriver = agent.NewMockAgentDriver()

	// Create dedicated manager for env tests
	envMgr, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme:         scheme.Scheme,
		LeaderElection: false,
		Metrics: server.Options{
			BindAddress: "0",
		},
	})
	Expect(err).NotTo(HaveOccurred())

	// Setup reconciler with mock driver and different controller name
	envReconciler := &AgentEndpointReconciler{
		Client:      envMgr.GetClient(),
		Log:         logf.Log.WithName("env-agent-endpoint-controller"),
		Scheme:      envMgr.GetScheme(),
		Recorder:    envMgr.GetEventRecorderFor("env-agent-endpoint-controller"),
		AgentDriver: envMockDriver,
	}

	// Register controller with manager
	Expect(envReconciler.SetupWithManager(envMgr)).To(Succeed())

	// Start env test manager
	go func() {
		defer GinkgoRecover()
		err := envMgr.Start(envCtx)
		if err != nil && envCtx.Err() == nil {
			logf.Log.Error(err, "Env test manager failed to start")
		}
	}()

	// Wait for env manager to be ready
	Eventually(func() bool {
		testCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		return envMgr.GetCache().WaitForCacheSync(testCtx)
	}, 30*time.Second, 100*time.Millisecond).Should(BeTrue())
})

var _ = AfterSuite(func() {
	By("stopping env test manager")
	if envCancel != nil {
		envCancel()
	}

	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
