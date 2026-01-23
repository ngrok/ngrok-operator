/*
MIT License

Copyright (c) 2022 ngrok, Inc.

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

package gateway

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-operator/internal/controller/ingress"
	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
	"github.com/ngrok/ngrok-operator/internal/testutils"
	"github.com/ngrok/ngrok-operator/pkg/managerdriver"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg          *rest.Config
	k8sClient    client.Client
	testEnv      *envtest.Environment
	driver       *managerdriver.Driver
	domainClient *nmockapi.DomainClient

	ctx    context.Context
	cancel context.CancelFunc
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(
		zap.New(
			zap.WriteTo(GinkgoWriter),
			zap.UseDevMode(true),
			zap.Level(zapcore.Level(-5)),
		),
	)

	ctx, cancel = context.WithCancel(GinkgoT().Context())

	By("bootstrapping test environment")
	gwAPIs := filepath.Join(".", "testdata", "gatewayapi-crds.yaml")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			testutils.OperatorCRDPath("..", "..", ".."),
			gwAPIs,
		},
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = scheme.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = gatewayv1.Install(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = gatewayv1alpha2.Install(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = ingressv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = ngrokv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	driver = managerdriver.NewDriver(
		logr.New(logr.Discard().GetSink()),
		scheme.Scheme,
		testutils.DefaultControllerName,
		types.NamespacedName{
			Name:      "test-manager-name",
			Namespace: "test-manager-namespace",
		},
		managerdriver.WithGatewayEnabled(true),
		managerdriver.WithSyncAllowConcurrent(true),
	)

	domainClient = nmockapi.NewDomainClient()

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: server.Options{
			// Set to 0 to disable the metrics server for tests
			BindAddress: "0",
		},
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sManager).NotTo(BeNil())

	// Run Domain reconciler with a mock domain client so that when we create Domain CRs
	// they are reconciled and we can test that addresses are
	// assigned to the Gateway resources.
	err = (&ingress.DomainReconciler{
		Client:        k8sManager.GetClient(),
		Log:           logf.Log.WithName("controllers").WithName("Domain"),
		Recorder:      k8sManager.GetEventRecorderFor("domain-controller"),
		Scheme:        k8sManager.GetScheme(),
		DomainsClient: domainClient,
	}).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	err = (&GatewayClassReconciler{
		Client:         k8sManager.GetClient(),
		Log:            logf.Log.WithName("controllers").WithName("GatewayClass"),
		Recorder:       k8sManager.GetEventRecorderFor("gatewayclass-controller"),
		Scheme:         k8sManager.GetScheme(),
		ControllerName: testutils.DefaultGatewayControllerName,
	}).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	err = (&GatewayReconciler{
		Client:         k8sManager.GetClient(),
		Log:            logf.Log.WithName("controllers").WithName("Gateway"),
		Scheme:         k8sManager.GetScheme(),
		Recorder:       k8sManager.GetEventRecorderFor("gateway-controller"),
		Driver:         driver,
		ControllerName: testutils.DefaultGatewayControllerName,
	}).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	err = (&HTTPRouteReconciler{
		Client:         k8sManager.GetClient(),
		Log:            logf.Log.WithName("controllers").WithName("HTTPRoute"),
		Scheme:         k8sManager.GetScheme(),
		Recorder:       k8sManager.GetEventRecorderFor("httproute-controller"),
		Driver:         driver,
		ControllerName: testutils.DefaultGatewayControllerName,
	}).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	err = (&TCPRouteReconciler{
		Client:   k8sManager.GetClient(),
		Log:      logf.Log.WithName("controllers").WithName("TCPRoute"),
		Recorder: k8sManager.GetEventRecorderFor("tcproute-controller"),
		Scheme:   k8sManager.GetScheme(),
		Driver:   driver,
	}).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	err = (&TLSRouteReconciler{
		Client:   k8sManager.GetClient(),
		Log:      logf.Log.WithName("controllers").WithName("TLSRoute"),
		Recorder: k8sManager.GetEventRecorderFor("tlsroute-controller"),
		Scheme:   k8sManager.GetScheme(),
		Driver:   driver,
	}).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).NotTo(HaveOccurred())
	}()
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

var _ = BeforeEach(func() {
	domainClient.Reset()
})

func CreateGatewayAndWaitForAcceptance(ctx SpecContext, gw *gatewayv1.Gateway, timeout time.Duration, interval time.Duration) {
	GinkgoHelper()
	Expect(k8sClient.Create(ctx, gw)).To(Succeed())
	ExpectGatewayAccepted(ctx, gw, timeout, interval)
}

func DeleteGatewayAndWaitForDeletion(ctx SpecContext, gw *gatewayv1.Gateway, timeout time.Duration, interval time.Duration) {
	GinkgoHelper()
	Expect(k8sClient.Delete(ctx, gw)).To(Succeed())

	// Wait for the gateway to be completely deleted
	Eventually(func(g Gomega) {
		obj := &gatewayv1.Gateway{}
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(gw), obj)
		g.Expect(errors.IsNotFound(err)).To(BeTrue())
	}).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
}

func CreateGatewayClassAndWaitForAcceptance(ctx SpecContext, gwc *gatewayv1.GatewayClass, timeout time.Duration, interval time.Duration) {
	GinkgoHelper()
	Expect(k8sClient.Create(ctx, gwc)).To(Succeed())
	ExpectGatewayClassAccepted(ctx, gwc, timeout, interval)
}

func DeleteAllGatewayClasses(ctx SpecContext, timeout, interval time.Duration) {
	GinkgoHelper()
	Expect(k8sClient.DeleteAllOf(ctx, &gatewayv1.GatewayClass{})).To(Succeed())

	// Wait for all the gateway classes to be deleted
	Eventually(func(g Gomega) {
		gatewayClasses := &gatewayv1.GatewayClassList{}
		g.Expect(k8sClient.List(ctx, gatewayClasses)).To(Succeed())
		g.Expect(gatewayClasses.Items).To(BeEmpty())
	}).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
}

func ExpectGatewayClassAccepted(ctx SpecContext, gwc *gatewayv1.GatewayClass, timeout time.Duration, interval time.Duration) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		obj := &gatewayv1.GatewayClass{}
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(gwc), obj)).To(Succeed())
		g.Expect(gatewayClassIsAccepted(obj)).To(BeTrue())
	}).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
}

func ExpectGatewayAccepted(ctx SpecContext, gw *gatewayv1.Gateway, timeout time.Duration, interval time.Duration) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		obj := &gatewayv1.Gateway{}
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(gw), obj)).To(Succeed())
		cond := meta.FindStatusCondition(obj.Status.Conditions, string(gatewayv1.GatewayConditionAccepted))
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	}).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
}

func ExpectGatewayNotAccepted(ctx SpecContext, gw *gatewayv1.Gateway) AsyncAssertion {
	GinkgoHelper()
	return Eventually(func(g Gomega) {
		obj := &gatewayv1.Gateway{}
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(gw), obj)).To(Succeed())

		cond := meta.FindStatusCondition(obj.Status.Conditions, string(gatewayv1.GatewayConditionAccepted))
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	})
}

func ExpectListenerStatus(ctx SpecContext, gw *gatewayv1.Gateway, listenerName gatewayv1.SectionName, t gatewayv1.ListenerConditionType, status metav1.ConditionStatus, reason gatewayv1.ListenerConditionReason) AsyncAssertion {
	GinkgoHelper()
	return Eventually(func(g Gomega) {
		obj := &gatewayv1.Gateway{}
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(gw), obj)).To(Succeed())

		// Find the listener with the given name
		var listener *gatewayv1.ListenerStatus
		for _, l := range obj.Status.Listeners {
			if l.Name == listenerName {
				listener = &l
				break
			}
		}
		g.Expect(listener).NotTo(BeNil())

		cnd := meta.FindStatusCondition(listener.Conditions, string(t))
		g.Expect(cnd).NotTo(BeNil())

		g.Expect(cnd.Status).To(Equal(status))
		g.Expect(cnd.Reason).To(Equal(string(reason)))
	})
}
