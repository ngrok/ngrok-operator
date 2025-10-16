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

package ingress

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
	"github.com/ngrok/ngrok-operator/internal/testutils"
	"github.com/ngrok/ngrok-operator/pkg/managerdriver"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
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
	cfg                *rest.Config
	k8sClient          client.Client
	testEnv            *envtest.Environment
	driver             *managerdriver.Driver
	domainClient       *nmockapi.DomainClient
	ipPolicyClient     *nmockapi.IPPolicyClient
	ipPolicyRuleClient *nmockapi.IPPolicyRuleClient

	ctx    context.Context
	cancel context.CancelFunc
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(GinkgoT().Context())

	By("bootstrapping test environment")
	operatorAPIs := filepath.Join("..", "..", "..", "helm", "ngrok-operator", "templates", "crds")
	gwAPIs := filepath.Join(".", "testdata", "gatewayapi-crds.yaml")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{operatorAPIs, gwAPIs},
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	schemeAdders := []func(*runtime.Scheme) error{
		scheme.AddToScheme,
		ngrokv1alpha1.AddToScheme,
		ingressv1alpha1.AddToScheme,
		gatewayv1.Install,
		gatewayv1alpha2.Install,
	}
	for _, addFunc := range schemeAdders {
		Expect(addFunc(scheme.Scheme)).NotTo(HaveOccurred())
	}
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

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: server.Options{
			// Set to 0 to disable the metrics server for tests
			BindAddress: "0",
		},
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sManager).NotTo(BeNil())

	domainClient = nmockapi.NewDomainClient()

	err = (&DomainReconciler{
		Client:        k8sManager.GetClient(),
		Log:           logf.Log.WithName("controllers").WithName("Domain"),
		Recorder:      k8sManager.GetEventRecorderFor("domain-controller"),
		Scheme:        k8sManager.GetScheme(),
		DomainsClient: domainClient,
	}).SetupWithManager(k8sManager)

	Expect(err).NotTo(HaveOccurred())

	ipPolicyClient = nmockapi.NewIPPolicyClient()
	ipPolicyRuleClient = nmockapi.NewIPPolicyRuleClient(ipPolicyClient)

	err = (&IPPolicyReconciler{
		Client:              k8sManager.GetClient(),
		Log:                 logf.Log.WithName("controllers").WithName("IPPolicy"),
		Recorder:            k8sManager.GetEventRecorderFor("ippolicy-controller"),
		Scheme:              k8sManager.GetScheme(),
		IPPoliciesClient:    ipPolicyClient,
		IPPolicyRulesClient: ipPolicyRuleClient,
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
