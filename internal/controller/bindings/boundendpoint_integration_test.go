package bindings

import (
	"context"
	"time"

	"github.com/ngrok/ngrok-api-go/v7"
	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/testutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("BoundEndpoint Controller", func() {
	const (
		timeout  = 30 * time.Second
		interval = 500 * time.Millisecond
	)

	var (
		testCtx context.Context
	)

	BeforeEach(func() {
		testCtx = ctx
		resetMockEndpoints()
	})

	AfterEach(func() {
		// Clean up all BoundEndpoints
		boundEndpoints := &bindingsv1alpha1.BoundEndpointList{}
		err := k8sClient.List(testCtx, boundEndpoints, &client.ListOptions{
			Namespace: pollerController.Namespace,
		})
		Expect(err).NotTo(HaveOccurred())

		for _, be := range boundEndpoints.Items {
			_ = k8sClient.Delete(testCtx, &be)
		}

		// Wait for cleanup
		Eventually(func(g Gomega) {
			list := &bindingsv1alpha1.BoundEndpointList{}
			err := k8sClient.List(testCtx, list, &client.ListOptions{
				Namespace: pollerController.Namespace,
			})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(list.Items).To(BeEmpty())
		}, timeout, interval).Should(Succeed())

		resetMockEndpoints()
	})

	Context("Single endpoint", func() {
		It("should create services and set conditions", func() {
			By("Creating target namespace")
			expectCreateNs("test-namespace")
			defer expectDeleteNs("test-namespace")

			By("Setting up mock API with one endpoint")
			setMockEndpoints([]ngrok.Endpoint{
				{
					ID:        "ep_abc123",
					URI:       "https://api.ngrok.com/endpoints/ep_abc123",
					PublicURL: "https://test-service.test-namespace:8080",
					Proto:     "https",
					Bindings:  []string{"public", "kubernetes://test-service.test-namespace:8080"},
				},
			})

			By("Triggering poller to create BoundEndpoint")
			err := triggerPoller(testCtx)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for BoundEndpoint to be created")
			var boundEndpointName string
			Eventually(func(g Gomega) {
				list := &bindingsv1alpha1.BoundEndpointList{}
				err := k8sClient.List(testCtx, list, &client.ListOptions{
					Namespace: pollerController.Namespace,
				})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(list.Items).To(HaveLen(1))

				be := list.Items[0]
				boundEndpointName = be.Name

				// Poller should have set these fields
				g.Expect(be.Status.Endpoints).To(HaveLen(1))
				g.Expect(be.Status.Endpoints[0].ID).To(Equal("ep_abc123"))
				g.Expect(be.Status.EndpointsSummary).To(Equal("1 endpoint"))
				g.Expect(be.Status.HashedName).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())

			By("Waiting for controller to create services and set conditions")
			Eventually(func(g Gomega) {
				be := &bindingsv1alpha1.BoundEndpoint{}
				err := k8sClient.Get(testCtx, types.NamespacedName{
					Name:      boundEndpointName,
					Namespace: pollerController.Namespace,
				}, be)
				g.Expect(err).NotTo(HaveOccurred())

				// Check ServicesCreated condition
				servicesCreatedCond := testutils.FindCondition(be.Status.Conditions, ConditionTypeServicesCreated)
				g.Expect(servicesCreatedCond).NotTo(BeNil(), "ServicesCreated condition should exist")
				g.Expect(servicesCreatedCond.Status).To(Equal(metav1.ConditionTrue), "ServicesCreated should be True")

				// Check service references are set
				g.Expect(be.Status.TargetServiceRef).NotTo(BeNil(), "TargetServiceRef should be set")
				g.Expect(be.Status.TargetServiceRef.Name).To(Equal("test-service"))
				g.Expect(be.Status.TargetServiceRef.Namespace).NotTo(BeNil())
				g.Expect(*be.Status.TargetServiceRef.Namespace).To(Equal("test-namespace"))

				g.Expect(be.Status.UpstreamServiceRef).NotTo(BeNil(), "UpstreamServiceRef should be set")
				g.Expect(be.Status.UpstreamServiceRef.Name).NotTo(BeEmpty())

				// NOTE: Ready condition will be False in test env because connectivity check fails
				// (no actual service to dial). We just verify the condition exists and services were created.
				readyCond := testutils.FindCondition(be.Status.Conditions, ConditionTypeReady)
				g.Expect(readyCond).NotTo(BeNil(), "Ready condition should exist")
			}, timeout, interval).Should(Succeed())

			By("Verifying target service was created in user namespace")
			targetSvc := &v1.Service{}
			err = k8sClient.Get(testCtx, types.NamespacedName{
				Name:      "test-service",
				Namespace: "test-namespace",
			}, targetSvc)
			Expect(err).NotTo(HaveOccurred())
			Expect(targetSvc.Spec.Type).To(Equal(v1.ServiceTypeExternalName))

			By("Verifying upstream service was created in operator namespace")
			be := &bindingsv1alpha1.BoundEndpoint{}
			err = k8sClient.Get(testCtx, types.NamespacedName{
				Name:      boundEndpointName,
				Namespace: pollerController.Namespace,
			}, be)
			Expect(err).NotTo(HaveOccurred())

			upstreamSvc := &v1.Service{}
			err = k8sClient.Get(testCtx, types.NamespacedName{
				Name:      be.Status.UpstreamServiceRef.Name,
				Namespace: pollerController.Namespace,
			}, upstreamSvc)
			Expect(err).NotTo(HaveOccurred())
			Expect(upstreamSvc.Spec.Type).To(Equal(v1.ServiceTypeClusterIP))
		})
	})

	Context("Multiple endpoints", func() {
		It("should aggregate endpoints targeting the same service", func() {
			By("Creating target namespace")
			expectCreateNs("multi-namespace")
			defer expectDeleteNs("multi-namespace")

			By("Setting up mock API with two endpoints pointing to same service")
			setMockEndpoints([]ngrok.Endpoint{
				{
					ID:        "ep_first123",
					URI:       "https://api.ngrok.com/endpoints/ep_first123",
					PublicURL: "https://my-service.multi-namespace:8080",
					Proto:     "https",
					Bindings:  []string{"public", "kubernetes://my-service.multi-namespace:8080"},
				},
				{
					ID:        "ep_second456",
					URI:       "https://api.ngrok.com/endpoints/ep_second456",
					PublicURL: "https://my-service.multi-namespace:8080",
					Proto:     "https",
					Bindings:  []string{"public", "kubernetes://my-service.multi-namespace:8080"},
				},
			})

			By("Triggering poller to create BoundEndpoint")
			err := triggerPoller(testCtx)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for BoundEndpoint with aggregated endpoints")
			Eventually(func(g Gomega) {
				list := &bindingsv1alpha1.BoundEndpointList{}
				err := k8sClient.List(testCtx, list, &client.ListOptions{
					Namespace: pollerController.Namespace,
				})
				g.Expect(err).NotTo(HaveOccurred())

				// Should be exactly one BoundEndpoint (both endpoints aggregated)
				g.Expect(list.Items).To(HaveLen(1))

				be := list.Items[0]

				// Both endpoints should be in the status
				g.Expect(be.Status.Endpoints).To(HaveLen(2))
				endpointIDs := []string{be.Status.Endpoints[0].ID, be.Status.Endpoints[1].ID}
				g.Expect(endpointIDs).To(ConsistOf("ep_first123", "ep_second456"))

				// Summary should show "2 endpoints"
				g.Expect(be.Status.EndpointsSummary).To(Equal("2 endpoints"))

				// Spec should point to the same target
				g.Expect(be.Spec.Target.Service).To(Equal("my-service"))
				g.Expect(be.Spec.Target.Namespace).To(Equal("multi-namespace"))
				g.Expect(be.Spec.Target.Port).To(Equal(int32(8080)))
			}, timeout, interval).Should(Succeed())

			By("Waiting for services to be created")
			Eventually(func(g Gomega) {
				list := &bindingsv1alpha1.BoundEndpointList{}
				err := k8sClient.List(testCtx, list, &client.ListOptions{
					Namespace: pollerController.Namespace,
				})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(list.Items).To(HaveLen(1))

				be := list.Items[0]

				// Check ServicesCreated condition
				servicesCreatedCond := testutils.FindCondition(be.Status.Conditions, ConditionTypeServicesCreated)
				g.Expect(servicesCreatedCond).NotTo(BeNil())
				g.Expect(servicesCreatedCond.Status).To(Equal(metav1.ConditionTrue))
			}, timeout, interval).Should(Succeed())

			By("Verifying only one target service was created (shared by both endpoints)")
			targetSvc := &v1.Service{}
			err = k8sClient.Get(testCtx, types.NamespacedName{
				Name:      "my-service",
				Namespace: "multi-namespace",
			}, targetSvc)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Status updates", func() {
		It("should not get stuck in provisioning when adding endpoints", func() {
			By("Creating target namespace")
			expectCreateNs("status-namespace")
			defer expectDeleteNs("status-namespace")

			By("Setting up mock API with one endpoint initially")
			setMockEndpoints([]ngrok.Endpoint{
				{
					ID:        "ep_initial",
					URI:       "https://api.ngrok.com/endpoints/ep_initial",
					PublicURL: "https://my-app.status-namespace:8080",
					Proto:     "https",
					Bindings:  []string{"public", "kubernetes://my-app.status-namespace:8080"},
				},
			})

			By("Triggering poller to create initial BoundEndpoint")
			err := triggerPoller(testCtx)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for services to be created")
			var boundEndpointName string
			Eventually(func(g Gomega) {
				list := &bindingsv1alpha1.BoundEndpointList{}
				err := k8sClient.List(testCtx, list, &client.ListOptions{
					Namespace: pollerController.Namespace,
				})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(list.Items).To(HaveLen(1))

				be := list.Items[0]
				boundEndpointName = be.Name

				servicesCreatedCond := testutils.FindCondition(be.Status.Conditions, ConditionTypeServicesCreated)
				g.Expect(servicesCreatedCond).NotTo(BeNil())
				g.Expect(servicesCreatedCond.Status).To(Equal(metav1.ConditionTrue))
			}, timeout, interval).Should(Succeed())

			By("Adding a second endpoint to the same service")
			setMockEndpoints([]ngrok.Endpoint{
				{
					ID:        "ep_initial",
					URI:       "https://api.ngrok.com/endpoints/ep_initial",
					PublicURL: "https://my-app.status-namespace:8080",
					Proto:     "https",
					Bindings:  []string{"public", "kubernetes://my-app.status-namespace:8080"},
				},
				{
					ID:        "ep_second",
					URI:       "https://api.ngrok.com/endpoints/ep_second",
					PublicURL: "https://my-app.status-namespace:8080",
					Proto:     "https",
					Bindings:  []string{"public", "kubernetes://my-app.status-namespace:8080"},
				},
			})

			By("Triggering poller to update BoundEndpoint")
			err = triggerPoller(testCtx)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying ServicesCreated condition stays True (not reset to provisioning)")
			Eventually(func(g Gomega) {
				be := &bindingsv1alpha1.BoundEndpoint{}
				err := k8sClient.Get(testCtx, types.NamespacedName{
					Name:      boundEndpointName,
					Namespace: pollerController.Namespace,
				}, be)
				g.Expect(err).NotTo(HaveOccurred())

				// Should now have 2 endpoints
				g.Expect(be.Status.Endpoints).To(HaveLen(2))
				g.Expect(be.Status.EndpointsSummary).To(Equal("2 endpoints"))

				// KEY TEST: ServicesCreated condition should remain True
				servicesCreatedCond := testutils.FindCondition(be.Status.Conditions, ConditionTypeServicesCreated)
				g.Expect(servicesCreatedCond).NotTo(BeNil())
				g.Expect(servicesCreatedCond.Status).To(Equal(metav1.ConditionTrue),
					"ServicesCreated should stay True after adding endpoint")
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("Error handling", func() {
		It("should set ServicesCreated condition to False when target namespace missing", func() {
			By("NOT creating target namespace - this will cause service creation to fail")

			By("Setting up mock API with endpoint pointing to non-existent namespace")
			setMockEndpoints([]ngrok.Endpoint{
				{
					ID:        "ep_missing_ns",
					URI:       "https://api.ngrok.com/endpoints/ep_missing_ns",
					PublicURL: "https://my-service.missing-namespace:8080",
					Proto:     "https",
					Bindings:  []string{"public", "kubernetes://my-service.missing-namespace:8080"},
				},
			})

			By("Triggering poller to create BoundEndpoint")
			err := triggerPoller(testCtx)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for BoundEndpoint to be created")
			var boundEndpointName string
			Eventually(func(g Gomega) {
				list := &bindingsv1alpha1.BoundEndpointList{}
				err := k8sClient.List(testCtx, list, &client.ListOptions{
					Namespace: pollerController.Namespace,
				})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(list.Items).To(HaveLen(1))
				boundEndpointName = list.Items[0].Name
			}, timeout, interval).Should(Succeed())

			By("Verifying ServicesCreated condition is False with namespace error")
			Eventually(func(g Gomega) {
				be := &bindingsv1alpha1.BoundEndpoint{}
				err := k8sClient.Get(testCtx, types.NamespacedName{
					Name:      boundEndpointName,
					Namespace: pollerController.Namespace,
				}, be)
				g.Expect(err).NotTo(HaveOccurred())

				servicesCreatedCond := testutils.FindCondition(be.Status.Conditions, ConditionTypeServicesCreated)
				g.Expect(servicesCreatedCond).NotTo(BeNil())
				g.Expect(servicesCreatedCond.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(servicesCreatedCond.Reason).To(Equal(ReasonServiceCreationFailed))
				g.Expect(servicesCreatedCond.Message).To(ContainSubstring("namespace"))

				// Ready should also be False
				readyCond := testutils.FindCondition(be.Status.Conditions, ConditionTypeReady)
				g.Expect(readyCond).NotTo(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			}, timeout, interval).Should(Succeed())
		})
	})
})
