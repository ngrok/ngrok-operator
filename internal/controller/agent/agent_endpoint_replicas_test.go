package agent

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	"github.com/ngrok/ngrok-operator/internal/controller/labels"
	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
	trafficpolicypkg "github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	"github.com/ngrok/ngrok-operator/pkg/agent"
)

// Every agent replica binds every AgentEndpoint, but only the one that
// removes the finalizer sees the object being deleted. The suite's manager is
// one replica; a second reconciler with its own driver is another.
var _ = Describe("AgentEndpoint with several agent replicas", func() {
	const (
		timeout  = 15 * time.Second
		interval = 250 * time.Millisecond
		name     = "multi-replica"
		url      = "tcp://1.tcp.ngrok.io:20003"
	)

	It("closes the tunnel on every replica when the object is deleted", func(ctx SpecContext) {
		namespace := "replicas-" + RandomString(8)
		Expect(k8sClient.Create(ctx, &corev1.Namespace{Name: namespace})).To(Succeed())
		DeferCleanup(func(ctx SpecContext) {
			Expect(k8sClient.Delete(ctx, &corev1.Namespace{Name: namespace})).To(Succeed())
		})

		key := client.ObjectKey{Namespace: namespace, Name: name}
		tunnel := namespace + "/" + name
		envMockDriver.SetEndpointResult(tunnel, &agent.EndpointResult{URL: url})

		otherDriver := agent.NewMockAgentDriver()
		otherDriver.SetEndpointResult(tunnel, &agent.EndpointResult{URL: url})
		recorder := envMgr.GetEventRecorder("other-replica")
		controllerLabels := labels.NewControllerLabelValues(controllerNamespace, controllerName)
		dm, err := domainpkg.NewManager(k8sClient, recorder, domainpkg.WithControllerLabels(controllerLabels))
		Expect(err).NotTo(HaveOccurred())
		other := &AgentEndpointReconciler{
			Client:               k8sClient,
			APIReader:            k8sClient,
			Log:                  logf.Log.WithName("other-replica"),
			Recorder:             recorder,
			AgentDriver:          otherDriver,
			ControllerLabels:     controllerLabels,
			DomainManager:        dm,
			TrafficPolicyManager: trafficpolicypkg.NewManager(k8sClient, recorder),
		}

		aep := &ngrokv1.AgentEndpoint{
			Name:      name,
			Namespace: namespace,
			Spec: ngrokv1.AgentEndpointSpec{
				URL:      url,
				Upstream: ngrokv1.EndpointUpstream{URL: "http://svc:80"},
			},
		}
		Expect(k8sClient.Create(ctx, aep)).To(Succeed())

		By("binding the tunnel on the other replica too")
		Eventually(func() error {
			_, err := other.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			return err
		}, timeout, interval).Should(Succeed())
		Expect(otherDriver.CreateCalls).NotTo(BeEmpty())

		By("letting the suite's replica process the delete")
		Expect(k8sClient.Delete(ctx, aep)).To(Succeed())
		Eventually(func() bool {
			return apierrors.IsNotFound(k8sClient.Get(ctx, key, &ngrokv1.AgentEndpoint{}))
		}, timeout, interval).Should(BeTrue())

		_, err = other.Reconcile(ctx, ctrl.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		Expect(otherDriver.DeleteCalls).To(ContainElement(agent.DeleteCall{Name: tunnel}))
	})
})
