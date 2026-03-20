package gateway

import (
	testutils "github.com/ngrok/ngrok-operator/internal/testutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("parentRefIsGateway", func() {
	DescribeTable("returns true only when the ref resolves to a Gateway",
		func(group *gatewayv1.Group, kind *gatewayv1.Kind, want bool) {
			ref := gatewayv1.ParentReference{
				Name:  "some-gateway",
				Group: group,
				Kind:  kind,
			}
			Expect(parentRefIsGateway(ref)).To(Equal(want))
		},
		Entry("nil Group and nil Kind defaults to Gateway", nil, nil, true),
		Entry("explicit gateway API group and Gateway kind", ptr.To(gatewayv1.Group(gatewayv1.GroupName)), ptr.To(gatewayv1.Kind("Gateway")), true),
		Entry("explicit gateway API group and nil Kind", ptr.To(gatewayv1.Group(gatewayv1.GroupName)), nil, true),
		Entry("nil Group and explicit Gateway kind", nil, ptr.To(gatewayv1.Kind("Gateway")), true),
		Entry("Service kind returns false", nil, ptr.To(gatewayv1.Kind("Service")), false),
		Entry("core group returns false", ptr.To(gatewayv1.Group("")), ptr.To(gatewayv1.Kind("Gateway")), false),
		Entry("unknown group returns false", ptr.To(gatewayv1.Group("some.other.io")), nil, false),
	)
})

var _ = Describe("routeReferencesNgrokGateway", Ordered, func() {
	var (
		managedGatewayClass   *gatewayv1.GatewayClass
		unmanagedGatewayClass *gatewayv1.GatewayClass
	)

	BeforeAll(func(ctx SpecContext) {
		By("creating a managed GatewayClass and waiting for acceptance")
		managedGatewayClass = testutils.NewGatewayClass(true)
		CreateGatewayClassAndWaitForAcceptance(ctx, managedGatewayClass, testutils.DefaultTimeout, testutils.DefaultInterval)

		By("creating an unmanaged GatewayClass")
		unmanagedGatewayClass = testutils.NewGatewayClass(false)
		Expect(k8sClient.Create(ctx, unmanagedGatewayClass)).To(Succeed())
	})

	AfterAll(func(ctx SpecContext) {
		DeleteAllGatewayClasses(ctx, testutils.DefaultTimeout, testutils.DefaultInterval)
	})

	When("parentRefs is empty", func() {
		It("returns false with no error", func(ctx SpecContext) {
			ok, err := routeReferencesNgrokGateway(ctx, k8sClient, "default", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
		})
	})

	When("the parentRef is not a Gateway kind", func() {
		It("returns false with no error", func(ctx SpecContext) {
			refs := []gatewayv1.ParentReference{
				{
					Group: ptr.To(gatewayv1.Group("")),
					Kind:  ptr.To(gatewayv1.Kind("Service")),
					Name:  "some-service",
				},
			}
			ok, err := routeReferencesNgrokGateway(ctx, k8sClient, "default", refs)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
		})
	})

	When("the parentRef points to a Gateway that does not exist", func() {
		It("returns false with no error (cannot confirm ownership)", func(ctx SpecContext) {
			refs := []gatewayv1.ParentReference{
				{Name: "nonexistent-gateway"},
			}
			ok, err := routeReferencesNgrokGateway(ctx, k8sClient, "default", refs)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
		})
	})

	When("the parentRef points to a Gateway with a managed GatewayClass", func() {
		var gw *gatewayv1.Gateway

		BeforeEach(func(ctx SpecContext) {
			gw = newGateway(managedGatewayClass)
			CreateGatewayAndWaitForAcceptance(ctx, gw, testutils.DefaultTimeout, testutils.DefaultInterval)
		})

		AfterEach(func(ctx SpecContext) {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, gw))).To(Succeed())
		})

		It("returns true when namespace is inferred from the route", func(ctx SpecContext) {
			refs := []gatewayv1.ParentReference{
				{Name: gatewayv1.ObjectName(gw.Name)},
			}
			ok, err := routeReferencesNgrokGateway(ctx, k8sClient, gw.Namespace, refs)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
		})

		It("returns true when the namespace is specified explicitly on the parentRef", func(ctx SpecContext) {
			refs := []gatewayv1.ParentReference{
				{
					Name:      gatewayv1.ObjectName(gw.Name),
					Namespace: new(gatewayv1.Namespace(gw.Namespace)),
				},
			}
			// Pass a different default namespace to confirm the explicit ref namespace takes precedence.
			ok, err := routeReferencesNgrokGateway(ctx, k8sClient, "other-namespace", refs)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
		})
	})

	When("the parentRef points to a Gateway with an unmanaged GatewayClass", func() {
		var gw *gatewayv1.Gateway

		BeforeEach(func(ctx SpecContext) {
			gw = &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testutils.RandomName("gw"),
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: gatewayv1.ObjectName(unmanagedGatewayClass.Name),
					Listeners: []gatewayv1.Listener{
						{Name: "http", Port: 80, Protocol: gatewayv1.HTTPProtocolType},
					},
				},
			}
			Expect(k8sClient.Create(ctx, gw)).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, gw))).To(Succeed())
		})

		It("returns false", func(ctx SpecContext) {
			refs := []gatewayv1.ParentReference{
				{Name: gatewayv1.ObjectName(gw.Name)},
			}
			ok, err := routeReferencesNgrokGateway(ctx, k8sClient, gw.Namespace, refs)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
		})
	})

	When("multiple parentRefs include one that references an ngrok Gateway", func() {
		var managedGw *gatewayv1.Gateway

		BeforeEach(func(ctx SpecContext) {
			managedGw = newGateway(managedGatewayClass)
			CreateGatewayAndWaitForAcceptance(ctx, managedGw, testutils.DefaultTimeout, testutils.DefaultInterval)
		})

		AfterEach(func(ctx SpecContext) {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, managedGw))).To(Succeed())
		})

		It("returns true", func(ctx SpecContext) {
			refs := []gatewayv1.ParentReference{
				{Name: "nonexistent-gateway"},
				{Name: gatewayv1.ObjectName(managedGw.Name)},
			}
			ok, err := routeReferencesNgrokGateway(ctx, k8sClient, managedGw.Namespace, refs)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
		})
	})

	When("the parentRef GatewayClass does not exist", func() {
		var gw *gatewayv1.Gateway

		BeforeEach(func(ctx SpecContext) {
			// Create a Gateway referencing a GatewayClass that we'll use the kginkgo helper to verify is accepted first,
			// then manually verify the GatewayClass was accepted to demonstrate the helper.
			gw = newGateway(managedGatewayClass)
			CreateGatewayAndWaitForAcceptance(ctx, gw, testutils.DefaultTimeout, testutils.DefaultInterval)

			// Use the kginkgo EventuallyWithGatewayClass helper to confirm the GatewayClass is accepted.
			kginkgo.EventuallyWithGatewayClass(ctx, managedGatewayClass, func(g Gomega, fetched *gatewayv1.GatewayClass) {
				cond := meta.FindStatusCondition(fetched.Status.Conditions, string(gatewayv1.GatewayClassConditionStatusAccepted))
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			})
		})

		AfterEach(func(ctx SpecContext) {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, gw))).To(Succeed())
		})

		It("returns true when the Gateway's GatewayClass is managed and accepted", func(ctx SpecContext) {
			refs := []gatewayv1.ParentReference{
				{Name: gatewayv1.ObjectName(gw.Name)},
			}
			ok, err := routeReferencesNgrokGateway(ctx, k8sClient, gw.Namespace, refs)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
		})
	})
})
