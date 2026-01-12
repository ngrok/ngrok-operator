package agent

import (
	"context"
	"testing"
	"time"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller/labels"
	"github.com/ngrok/ngrok-operator/internal/testutils"
	"github.com/ngrok/ngrok-operator/pkg/agent"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

// Env test manager and mock driver for controller runtime tests
var envMgr ctrl.Manager
var envCtx context.Context
var envCancel context.CancelFunc
var envMockDriver *agent.MockAgentDriver

// Namespace filter test manager - tests namespace filtering behavior
var nsMgr ctrl.Manager
var nsMgrCtx context.Context
var nsMgrCancel context.CancelFunc
var nsMockDriver *agent.MockAgentDriver

const (
	watchedNamespace    = "test-watched"
	unwatchedNamespace  = "test-unwatched"
	controllerNamespace = "test-controller-namespace"
	controllerName      = "test-agent-controller"
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AgentEndpoint Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			testutils.OperatorCRDPath("..", "..", ".."),
		},
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
		Client:           envMgr.GetClient(),
		Log:              logf.Log.WithName("env-agent-endpoint-controller"),
		Scheme:           envMgr.GetScheme(),
		Recorder:         envMgr.GetEventRecorderFor("env-agent-endpoint-controller"),
		AgentDriver:      envMockDriver,
		ControllerLabels: labels.NewControllerLabelValues(controllerNamespace, controllerName),
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

	// Set up namespace filter test manager
	By("setting up namespace filter test manager")

	// Create namespaces for namespace filtering tests
	for _, ns := range []string{watchedNamespace, unwatchedNamespace} {
		namespace := &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: ns,
			},
		}
		err = k8sClient.Create(context.Background(), namespace)
		if err != nil {
			Expect(client.IgnoreAlreadyExists(err)).To(Succeed())
		}
	}

	// Initialize mock driver for namespace filter tests
	nsMockDriver = agent.NewMockAgentDriver()

	// Create a manager with namespace filtering
	nsMgr, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme:         scheme.Scheme,
		LeaderElection: false,
		Metrics: server.Options{
			BindAddress: "0",
		},
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				watchedNamespace: {},
			},
		},
	})
	Expect(err).NotTo(HaveOccurred())

	// Setup reconciler with mock driver for namespace filter tests
	nsReconciler := &AgentEndpointReconciler{
		Client:           nsMgr.GetClient(),
		Log:              logf.Log.WithName("ns-filter-test-controller"),
		Scheme:           nsMgr.GetScheme(),
		Recorder:         nsMgr.GetEventRecorderFor("ns-filter-test-controller"),
		AgentDriver:      nsMockDriver,
		ControllerLabels: labels.NewControllerLabelValues(controllerNamespace, controllerName),
	}

	Expect(nsReconciler.SetupWithManagerNamed(nsMgr, "ns-filter-agentendpoint")).To(Succeed())

	// Start the namespace filter manager
	nsMgrCtx, nsMgrCancel = context.WithCancel(context.Background())
	go func() {
		defer GinkgoRecover()
		err := nsMgr.Start(nsMgrCtx)
		if err != nil && nsMgrCtx.Err() == nil {
			logf.Log.Error(err, "Namespace filter test manager failed to start")
		}
	}()

	// Wait for namespace filter manager to be ready
	Eventually(func() bool {
		testCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		return nsMgr.GetCache().WaitForCacheSync(testCtx)
	}, 30*time.Second, 100*time.Millisecond).Should(BeTrue())
})

var _ = AfterSuite(func() {
	By("stopping namespace filter test manager")
	if nsMgrCancel != nil {
		nsMgrCancel()
	}

	By("stopping env test manager")
	if envCancel != nil {
		envCancel()
	}

	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
