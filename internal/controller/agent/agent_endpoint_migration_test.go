package agent

// LEGACY-agentendpoint-kind: delete this whole file at cleanup.

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/testutils"
	trafficpolicypkg "github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	"github.com/ngrok/ngrok-operator/pkg/agent"
)

// These specs cover the twin rules in specs/crd-group-migration.md for an
// ngrok.k8s.ngrok.com/v1alpha1 AgentEndpoint and its ngrok.com/v1 twin. The
// two objects carry different descriptions, so the driver calls show which
// one drove the tunnel.
var _ = Describe("AgentEndpoint migration to ngrok.com/v1", func() {
	const (
		timeout  = 15 * time.Second
		interval = 250 * time.Millisecond
		name     = "twin"
		url      = "tcp://1.tcp.ngrok.io:20000"
	)

	var (
		namespace string
		tunnel    string
		legacy    *ngrokv1alpha1.AgentEndpoint
		canonical *ngrokv1.AgentEndpoint
	)

	createCallsWith := func(description string) int {
		n := 0
		for _, call := range envMockDriver.CreateCalls {
			if call.Name == tunnel && call.Spec.Description == description {
				n++
			}
		}
		return n
	}

	deleteCalls := func() int {
		n := 0
		for _, call := range envMockDriver.DeleteCalls {
			if call.Name == tunnel {
				n++
			}
		}
		return n
	}

	readyReason := func(g Gomega, obj client.Object) string {
		g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(obj), obj)).To(Succeed())
		var conditions []metav1.Condition
		switch o := obj.(type) {
		case *ngrokv1.AgentEndpoint:
			conditions = o.Status.Conditions
		case *ngrokv1alpha1.AgentEndpoint:
			conditions = o.Status.Conditions
		}
		cond := testutils.FindCondition(conditions, ConditionReady)
		g.Expect(cond).NotTo(BeNil())
		return cond.Reason
	}

	expectGone := func(ctx context.Context, obj client.Object) {
		Eventually(func() bool {
			return apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj))
		}, timeout, interval).Should(BeTrue())
	}

	// takeOver creates the v1 twin and waits until it drives the tunnel and
	// the legacy object is passive.
	takeOver := func(ctx context.Context) {
		Expect(k8sClient.Create(ctx, canonical)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(createCallsWith("v1")).To(BeNumerically(">", 0))
			g.Expect(readyReason(g, &ngrokv1.AgentEndpoint{ObjectMeta: canonical.ObjectMeta})).To(Equal(ReasonEndpointActive))
			g.Expect(readyReason(g, &ngrokv1alpha1.AgentEndpoint{ObjectMeta: legacy.ObjectMeta})).To(Equal(ReasonSupersededByV1))
		}, timeout, interval).Should(Succeed())
	}

	BeforeEach(func(ctx SpecContext) {
		namespace = "migrate-" + RandomString(8)
		Expect(k8sClient.Create(ctx, &corev1.Namespace{Name: namespace})).To(Succeed())
		envMockDriver.Reset()

		tunnel = namespace + "/" + name
		envMockDriver.SetEndpointResult(tunnel, &agent.EndpointResult{URL: url})

		legacy = &ngrokv1alpha1.AgentEndpoint{
			Name: name,
			Spec: ngrokv1alpha1.AgentEndpointSpec{
				URL:         url,
				Description: "legacy",
				Upstream:    ngrokv1alpha1.EndpointUpstream{URL: "http://svc:80"},
			},
		}
		legacy.Namespace = namespace
		canonical = &ngrokv1.AgentEndpoint{
			Name: name,
			Spec: ngrokv1.AgentEndpointSpec{
				URL:         url,
				Description: "v1",
				Upstream:    ngrokv1.EndpointUpstream{URL: "http://svc:80"},
			},
		}
		canonical.Namespace = namespace

		By("serving the legacy object on its own")
		Expect(k8sClient.Create(ctx, legacy)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(createCallsWith("legacy")).To(BeNumerically(">", 0))
			g.Expect(readyReason(g, &ngrokv1alpha1.AgentEndpoint{ObjectMeta: legacy.ObjectMeta})).To(Equal(ReasonEndpointActive))
		}, timeout, interval).Should(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		Expect(k8sClient.Delete(ctx, &corev1.Namespace{Name: namespace})).To(Succeed())
	})

	It("lets the v1 twin take over and leaves the legacy object passive", func(ctx SpecContext) {
		takeOver(ctx)

		By("warning about the deprecated object")
		Eventually(func(g Gomega) {
			events := &corev1.EventList{}
			g.Expect(k8sClient.List(ctx, events, client.InNamespace(namespace))).To(Succeed())
			g.Expect(events.Items).To(ContainElement(And(
				HaveField("Reason", trafficpolicypkg.EventDeprecatedAPIGroup),
				HaveField("InvolvedObject.APIVersion", ngrokv1alpha1.GroupVersion.String()),
			)))
		}, timeout, interval).Should(Succeed())

		By("ignoring edits to the legacy object")
		Eventually(func() error {
			obj := &ngrokv1alpha1.AgentEndpoint{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(legacy), obj); err != nil {
				return err
			}
			obj.Spec.Description = "legacy-edited"
			return k8sClient.Update(ctx, obj)
		}, timeout, interval).Should(Succeed())
		Consistently(func() int { return createCallsWith("legacy-edited") }, "2s", interval).Should(BeZero())
		Expect(deleteCalls()).To(BeZero())
	})

	It("stops writing the legacy object's status once it is passive", func(ctx SpecContext) {
		takeOver(ctx)
		Eventually(envDeprecationWarnings.n.Load, timeout, interval).Should(BeNumerically(">", 0))
		// Let the take-over's own reconciles settle.
		Eventually(func() int64 {
			before := envDeprecationWarnings.n.Load()
			time.Sleep(time.Second)
			return envDeprecationWarnings.n.Load() - before
		}, timeout, interval).Should(BeZero())
		warnings := envDeprecationWarnings.n.Load()

		By("making the v1 object reconcile again")
		Eventually(func() error {
			obj := &ngrokv1.AgentEndpoint{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(canonical), obj); err != nil {
				return err
			}
			obj.Spec.Description = "v1-edited"
			return k8sClient.Update(ctx, obj)
		}, timeout, interval).Should(Succeed())
		Eventually(func() int { return createCallsWith("v1-edited") }, timeout, interval).Should(BeNumerically(">", 0))

		Consistently(envDeprecationWarnings.n.Load, "2s", interval).Should(Equal(warnings))
	})

	It("keeps the tunnel when the legacy object is deleted", func(ctx SpecContext) {
		takeOver(ctx)

		Expect(k8sClient.Delete(ctx, legacy)).To(Succeed())
		expectGone(ctx, legacy)
		Consistently(deleteCalls, "2s", interval).Should(BeZero())
		Eventually(func(g Gomega) {
			g.Expect(readyReason(g, &ngrokv1.AgentEndpoint{ObjectMeta: canonical.ObjectMeta})).To(Equal(ReasonEndpointActive))
		}, timeout, interval).Should(Succeed())
	})

	It("keeps the tunnel when v1 is applied and legacy deleted back to back", func(ctx SpecContext) {
		Expect(k8sClient.Create(ctx, canonical)).To(Succeed())
		Expect(k8sClient.Delete(ctx, legacy)).To(Succeed())

		expectGone(ctx, legacy)
		Eventually(func() int { return createCallsWith("v1") }, timeout, interval).Should(BeNumerically(">", 0))
		Consistently(deleteCalls, "2s", interval).Should(BeZero())
	})

	It("hands the tunnel back to the legacy object when v1 is deleted", func(ctx SpecContext) {
		takeOver(ctx)
		legacyCalls := createCallsWith("legacy")

		Expect(k8sClient.Delete(ctx, canonical)).To(Succeed())
		expectGone(ctx, canonical)
		Eventually(func(g Gomega) {
			g.Expect(createCallsWith("legacy")).To(BeNumerically(">", legacyCalls))
			g.Expect(readyReason(g, &ngrokv1alpha1.AgentEndpoint{ObjectMeta: legacy.ObjectMeta})).To(Equal(ReasonEndpointActive))
		}, timeout, interval).Should(Succeed())
		Expect(deleteCalls()).To(BeZero())
	})

	It("closes the tunnel once both objects are deleted", func(ctx SpecContext) {
		takeOver(ctx)

		Expect(k8sClient.Delete(ctx, legacy)).To(Succeed())
		Expect(k8sClient.Delete(ctx, canonical)).To(Succeed())
		expectGone(ctx, legacy)
		expectGone(ctx, canonical)
		Eventually(deleteCalls, timeout, interval).Should(BeNumerically(">", 0))
	})
})

var _ = Describe("Operator-generated legacy AgentEndpoint", func() {
	It("goes passive without a deprecation warning", func(ctx SpecContext) {
		const name = "generated"
		namespace := "migrate-" + RandomString(8)
		Expect(k8sClient.Create(ctx, &corev1.Namespace{Name: namespace})).To(Succeed())
		DeferCleanup(func(ctx SpecContext) {
			Expect(k8sClient.Delete(ctx, &corev1.Namespace{Name: namespace})).To(Succeed())
		})
		envMockDriver.SetEndpointResult(namespace+"/"+name, &agent.EndpointResult{URL: "tcp://1.tcp.ngrok.io:20001"})

		legacy := &ngrokv1alpha1.AgentEndpoint{
			Name: name,
			Spec: ngrokv1alpha1.AgentEndpointSpec{
				URL:      "tcp://1.tcp.ngrok.io:20001",
				Upstream: ngrokv1alpha1.EndpointUpstream{URL: "http://svc:80"},
			},
		}
		legacy.Namespace = namespace
		// The managerdriver stamps the api-manager's name, not the agent's.
		legacy.Labels = map[string]string{
			"ngrok.com/controller-namespace": controllerNamespace,
			"ngrok.com/controller-name":      controllerName + "-api-manager",
		}
		canonical := &ngrokv1.AgentEndpoint{
			Name: name,
			Spec: ngrokv1.AgentEndpointSpec{
				URL:      "tcp://1.tcp.ngrok.io:20001",
				Upstream: ngrokv1.EndpointUpstream{URL: "http://svc:80"},
			},
		}
		canonical.Namespace = namespace

		Expect(k8sClient.Create(ctx, legacy)).To(Succeed())
		Expect(k8sClient.Create(ctx, canonical)).To(Succeed())

		Eventually(func(g Gomega) {
			obj := &ngrokv1alpha1.AgentEndpoint{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(legacy), obj)).To(Succeed())
			cond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
			g.Expect(cond).NotTo(BeNil())
			g.Expect(cond.Reason).To(Equal(ReasonSupersededByV1))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())

		Consistently(func(g Gomega) {
			events := &corev1.EventList{}
			g.Expect(k8sClient.List(ctx, events, client.InNamespace(namespace))).To(Succeed())
			g.Expect(events.Items).NotTo(ContainElement(HaveField("Reason", trafficpolicypkg.EventDeprecatedAPIGroup)))
		}, "2s", 250*time.Millisecond).Should(Succeed())
	})
})
