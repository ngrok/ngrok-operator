package ngrok

import (
	"context"
	"time"

	"github.com/ngrok/ngrok-api-go/v9"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
	"github.com/ngrok/ngrok-operator/internal/testutils"
	"github.com/ngrok/ngrok-operator/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("KubernetesOperator Controller", Ordered, func() {
	const (
		timeout  = 15 * time.Second
		interval = 500 * time.Millisecond
	)

	var kginkgo *testutils.KGinkgo

	BeforeAll(func() {
		kginkgo = testutils.NewKGinkgo(k8sClient)

		kginkgo.ExpectCreateNamespace(context.Background(), controllerNamespace)
	})

	// forceDeleteKO removes the finalizer and deletes the KubernetesOperator to
	// avoid triggering the drain workflow during test cleanup.
	forceDeleteKO := func(ctx context.Context) {
		ko := &ngrokv1.KubernetesOperator{}
		err := k8sClient.Get(ctx, client.ObjectKey{
			Namespace: controllerNamespace,
			Name:      k8sOpName,
		}, ko)
		if apierrors.IsNotFound(err) {
			return
		}
		Expect(err).NotTo(HaveOccurred())

		if util.RemoveFinalizer(ko) {
			Expect(k8sClient.Update(ctx, ko)).To(Succeed())
		}
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, ko))).To(Succeed())

		// Wait for it to actually be gone
		Eventually(func() bool {
			err := k8sClient.Get(ctx, client.ObjectKey{
				Namespace: controllerNamespace,
				Name:      k8sOpName,
			}, &ngrokv1.KubernetesOperator{})
			return apierrors.IsNotFound(err)
		}).WithTimeout(timeout).WithPolling(interval).Should(BeTrue())
	}

	AfterEach(func() {
		mockKubernetesOperators := mockClientset.KubernetesOperators().(*nmockapi.KubernetesOperatorsClient)
		mockKubernetesOperators.ClearErrors()
		mockKubernetesOperators.Reset()
		forceDeleteKO(context.Background())
	})

	It("should register successfully with ingress feature enabled", func(ctx SpecContext) {
		ko := &ngrokv1.KubernetesOperator{
			Name:      k8sOpName,
			Namespace: controllerNamespace,
			Spec: ngrokv1.KubernetesOperatorSpec{
				Description:     "test operator",
				Metadata:        map[string]string{"owned-by": "test"},
				EnabledFeatures: []string{ngrokv1.KubernetesOperatorFeatureIngress},
				Region:          "global",
			},
		}

		By("Creating the KubernetesOperator")
		Expect(k8sClient.Create(ctx, ko)).To(Succeed())

		By("Expecting the finalizer to be added")
		kginkgo.ExpectFinalizerToBeAdded(ctx, ko, util.FinalizerName, testutils.WithTimeout(timeout))

		By("Expecting registration to succeed")
		kginkgo.EventuallyWithObject(ctx, ko.DeepCopy(), func(g Gomega, fetched client.Object) {
			koFetched := fetched.(*ngrokv1.KubernetesOperator)
			g.Expect(koFetched.Status.ID).NotTo(BeEmpty())
			g.Expect(koFetched.Status.ObservedGeneration).To(Equal(koFetched.Generation))

			registered := meta.FindStatusCondition(koFetched.Status.Conditions, ngrokv1.KubernetesOperatorConditionRegistered)
			g.Expect(registered).NotTo(BeNil())
			g.Expect(registered.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(registered.Reason).To(Equal(ngrokv1.KubernetesOperatorReasonRegistered))
			g.Expect(registered.ObservedGeneration).To(Equal(koFetched.Generation))

			ready := meta.FindStatusCondition(koFetched.Status.Conditions, ngrokv1.KubernetesOperatorConditionReady)
			g.Expect(ready).NotTo(BeNil())
			g.Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(ready.Reason).To(Equal(ngrokv1.KubernetesOperatorReasonRegistered))
			g.Expect(ready.ObservedGeneration).To(Equal(koFetched.Generation))
		}, testutils.WithTimeout(timeout))

		By("Updating the registered KubernetesOperator")
		current := &ngrokv1.KubernetesOperator{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ko), current)).To(Succeed())
		registeredGeneration := current.Generation
		current.Spec.Description = "updated test operator"
		Expect(k8sClient.Update(ctx, current)).To(Succeed())

		By("Expecting the remote registration and status to reflect the new generation")
		kginkgo.EventuallyWithObject(ctx, ko.DeepCopy(), func(g Gomega, fetched client.Object) {
			koFetched := fetched.(*ngrokv1.KubernetesOperator)
			g.Expect(koFetched.Generation).To(BeNumerically(">", registeredGeneration))
			g.Expect(koFetched.Status.ObservedGeneration).To(Equal(koFetched.Generation))

			registered := meta.FindStatusCondition(koFetched.Status.Conditions, ngrokv1.KubernetesOperatorConditionRegistered)
			g.Expect(registered).NotTo(BeNil())
			g.Expect(registered.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(registered.ObservedGeneration).To(Equal(koFetched.Generation))

			ready := meta.FindStatusCondition(koFetched.Status.Conditions, ngrokv1.KubernetesOperatorConditionReady)
			g.Expect(ready).NotTo(BeNil())
			g.Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(ready.ObservedGeneration).To(Equal(koFetched.Generation))

			remote, err := mockClientset.KubernetesOperators().Get(ctx, koFetched.Status.ID)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(remote.Description).To(Equal("updated test operator"))
		}, testutils.WithTimeout(timeout))
	})

	It("should report and recover from an invalid bindings configuration", func(ctx SpecContext) {
		ko := &ngrokv1.KubernetesOperator{
			Name:      k8sOpName,
			Namespace: controllerNamespace,
			Spec: ngrokv1.KubernetesOperatorSpec{
				Description:     "test operator with nil binding",
				Metadata:        map[string]string{"owned-by": "test"},
				EnabledFeatures: []string{ngrokv1.KubernetesOperatorFeatureBindings},
				Binding:         nil,
				Region:          "global",
			},
		}

		By("Creating the KubernetesOperator")
		Expect(k8sClient.Create(ctx, ko)).To(Succeed())

		By("Expecting the finalizer to be added")
		kginkgo.ExpectFinalizerToBeAdded(ctx, ko, util.FinalizerName, testutils.WithTimeout(timeout))

		By("Expecting the public failure condition contract")
		kginkgo.EventuallyWithObject(ctx, ko.DeepCopy(), func(g Gomega, fetched client.Object) {
			koFetched := fetched.(*ngrokv1.KubernetesOperator)
			g.Expect(koFetched.Status.ID).To(BeEmpty())
			g.Expect(koFetched.Status.ObservedGeneration).To(Equal(koFetched.Generation))

			registered := meta.FindStatusCondition(koFetched.Status.Conditions, ngrokv1.KubernetesOperatorConditionRegistered)
			g.Expect(registered).NotTo(BeNil())
			g.Expect(registered.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(registered.Reason).To(Equal(ngrokv1.KubernetesOperatorReasonRegistrationFailed))
			g.Expect(registered.ObservedGeneration).To(Equal(koFetched.Generation))

			ready := meta.FindStatusCondition(koFetched.Status.Conditions, ngrokv1.KubernetesOperatorConditionReady)
			g.Expect(ready).NotTo(BeNil())
			g.Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(ready.Reason).To(Equal(ngrokv1.KubernetesOperatorReasonConfigurationFailed))
			g.Expect(ready.ObservedGeneration).To(Equal(koFetched.Generation))
		}, testutils.WithTimeout(timeout))

		By("Fixing the configuration")
		current := &ngrokv1.KubernetesOperator{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ko), current)).To(Succeed())
		failedGeneration := current.Generation
		current.Spec.EnabledFeatures = []string{ngrokv1.KubernetesOperatorFeatureIngress}
		Expect(k8sClient.Update(ctx, current)).To(Succeed())

		By("Expecting registration and readiness to recover for the new generation")
		kginkgo.EventuallyWithObject(ctx, ko.DeepCopy(), func(g Gomega, fetched client.Object) {
			koFetched := fetched.(*ngrokv1.KubernetesOperator)
			g.Expect(koFetched.Generation).To(BeNumerically(">", failedGeneration))
			g.Expect(koFetched.Status.ID).NotTo(BeEmpty())
			g.Expect(koFetched.Status.ObservedGeneration).To(Equal(koFetched.Generation))

			registered := meta.FindStatusCondition(koFetched.Status.Conditions, ngrokv1.KubernetesOperatorConditionRegistered)
			g.Expect(registered).NotTo(BeNil())
			g.Expect(registered.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(registered.Reason).To(Equal(ngrokv1.KubernetesOperatorReasonRegistered))
			g.Expect(registered.ObservedGeneration).To(Equal(koFetched.Generation))

			ready := meta.FindStatusCondition(koFetched.Status.Conditions, ngrokv1.KubernetesOperatorConditionReady)
			g.Expect(ready).NotTo(BeNil())
			g.Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(ready.Reason).To(Equal(ngrokv1.KubernetesOperatorReasonRegistered))
			g.Expect(ready.ObservedGeneration).To(Equal(koFetched.Generation))
		}, testutils.WithTimeout(timeout))
	})

	Context("legacy ngrok.k8s.ngrok.com/v1alpha1 KubernetesOperator", func() {
		legacyKey := client.ObjectKey{Namespace: controllerNamespace, Name: k8sOpName}

		// createLegacyKO creates the legacy object an older operator would have
		// left behind, carrying the operator finalizer.
		createLegacyKO := func(ctx context.Context, finalizers ...string) *ngrokv1alpha1.KubernetesOperator {
			legacy := &ngrokv1alpha1.KubernetesOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name:       k8sOpName,
					Namespace:  controllerNamespace,
					Finalizers: finalizers,
				},
				Spec: ngrokv1alpha1.KubernetesOperatorSpec{
					Description: "legacy operator",
					Region:      "global",
				},
			}
			Expect(k8sClient.Create(ctx, legacy)).To(Succeed())
			DeferCleanup(func() {
				current := &ngrokv1alpha1.KubernetesOperator{}
				if err := k8sClient.Get(context.Background(), legacyKey, current); err != nil {
					return
				}
				if len(current.Finalizers) > 0 {
					current.Finalizers = nil
					_ = k8sClient.Update(context.Background(), current)
				}
				_ = client.IgnoreNotFound(k8sClient.Delete(context.Background(), current))
			})
			return legacy
		}

		// seedRemoteKO registers a KubernetesOperator in the mock ngrok API that
		// findExisting will adopt, as if the legacy operator had registered it.
		seedRemoteKO := func(ctx context.Context) string {
			ns := &v1.Namespace{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: controllerNamespace}, ns)).To(Succeed())
			remote, err := mockClientset.KubernetesOperators().Create(ctx, &ngrok.KubernetesOperatorCreate{
				Description: "legacy operator",
				Metadata:    `{"owned-by":"ngrok-operator","namespace.uid":"` + string(ns.UID) + `"}`,
			})
			Expect(err).NotTo(HaveOccurred())
			remote.Deployment = ngrok.KubernetesOperatorDeployment{Name: "ngrok-operator", Namespace: controllerNamespace}
			return remote.ID
		}

		canonicalKO := func() *ngrokv1.KubernetesOperator {
			return &ngrokv1.KubernetesOperator{
				ObjectMeta: metav1.ObjectMeta{Name: k8sOpName, Namespace: controllerNamespace},
				Spec: ngrokv1.KubernetesOperatorSpec{
					Description:     "canonical operator",
					EnabledFeatures: []string{ngrokv1.KubernetesOperatorFeatureIngress},
					Region:          "global",
					Deployment:      &ngrokv1.KubernetesOperatorDeployment{Name: "ngrok-operator", Namespace: controllerNamespace},
				},
			}
		}

		expectRemoteKOCount := func(ctx context.Context, n int) {
			count := 0
			iter := mockClientset.KubernetesOperators().List(&ngrok.Paging{})
			for iter.Next(ctx) {
				count++
			}
			Expect(iter.Err()).NotTo(HaveOccurred())
			Expect(count).To(Equal(n))
		}

		expectLegacyNeutralized := func(ctx context.Context) {
			Eventually(func(g Gomega) {
				legacy := &ngrokv1alpha1.KubernetesOperator{}
				g.Expect(k8sClient.Get(ctx, legacyKey, legacy)).To(Succeed())
				g.Expect(legacy.Finalizers).To(BeEmpty())
				g.Expect(legacy.Annotations).To(HaveKeyWithValue(AnnotationSupersededBy, "kubernetesoperators.ngrok.com/"+k8sOpName))
			}).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
		}

		It("strips the finalizer from a pre-existing legacy KO without draining", func(ctx SpecContext) {
			remoteID := seedRemoteKO(ctx)
			createLegacyKO(ctx, util.FinalizerName)

			Expect(k8sClient.Create(ctx, canonicalKO())).To(Succeed())

			By("adopting the legacy operator's ngrok KubernetesOperator")
			kginkgo.EventuallyWithObject(ctx, canonicalKO(), func(g Gomega, fetched client.Object) {
				g.Expect(fetched.(*ngrokv1.KubernetesOperator).Status.ID).To(Equal(remoteID))
			}, testutils.WithTimeout(timeout))

			By("neutralizing, not deleting, the legacy object")
			expectLegacyNeutralized(ctx)
			expectRemoteKOCount(ctx, 1)
			_, err := mockClientset.KubernetesOperators().Get(ctx, remoteID)
			Expect(err).NotTo(HaveOccurred())

			By("stripping the finalizer again when an older operator re-adds it")
			legacy := &ngrokv1alpha1.KubernetesOperator{}
			Expect(k8sClient.Get(ctx, legacyKey, legacy)).To(Succeed())
			legacy.Finalizers = []string{util.LegacyFinalizerName}
			Expect(k8sClient.Update(ctx, legacy)).To(Succeed())
			expectLegacyNeutralized(ctx)
		})

		It("strips the finalizer from a terminating legacy KO without draining", func(ctx SpecContext) {
			remoteID := seedRemoteKO(ctx)
			legacy := createLegacyKO(ctx, util.FinalizerName)
			Expect(k8sClient.Delete(ctx, legacy)).To(Succeed())

			Expect(k8sClient.Create(ctx, canonicalKO())).To(Succeed())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, legacyKey, &ngrokv1alpha1.KubernetesOperator{})
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
			_, err := mockClientset.KubernetesOperators().Get(ctx, remoteID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("leaves the legacy KO alone while the canonical KO is terminating", func(ctx SpecContext) {
			const holdFinalizer = "test.ngrok.com/hold"
			seedRemoteKO(ctx)

			canonical := canonicalKO()
			canonical.Finalizers = []string{holdFinalizer}
			Expect(k8sClient.Create(ctx, canonical)).To(Succeed())
			kginkgo.EventuallyWithObject(ctx, canonicalKO(), func(g Gomega, fetched client.Object) {
				g.Expect(fetched.(*ngrokv1.KubernetesOperator).Status.ID).NotTo(BeEmpty())
			}, testutils.WithTimeout(timeout))
			// Take the canonical KO out of the drain path: strip the operator
			// finalizer so deletion only waits on the test hold finalizer.
			current := &ngrokv1.KubernetesOperator{}
			Expect(k8sClient.Get(ctx, legacyKey, current)).To(Succeed())
			util.RemoveFinalizer(current)
			Expect(k8sClient.Update(ctx, current)).To(Succeed())
			Expect(k8sClient.Delete(ctx, current)).To(Succeed())

			createLegacyKO(ctx, util.FinalizerName)

			Consistently(func(g Gomega) {
				legacy := &ngrokv1alpha1.KubernetesOperator{}
				g.Expect(k8sClient.Get(ctx, legacyKey, legacy)).To(Succeed())
				g.Expect(legacy.Finalizers).To(ContainElement(util.FinalizerName))
			}).WithTimeout(3 * time.Second).WithPolling(interval).Should(Succeed())

			// Release the hold before AfterEach waits for the canonical KO to go.
			Expect(k8sClient.Get(ctx, legacyKey, current)).To(Succeed())
			controllerutil.RemoveFinalizer(current, holdFinalizer)
			Expect(k8sClient.Update(ctx, current)).To(Succeed())
		})
	})

	// Keep this last: drain state intentionally latches for the lifetime of the
	// manager process because a real operator exits after its KO is deleted.
	It("should publish drain conditions and clean up through a real deletion event", func(ctx SpecContext) {
		const holdFinalizer = "test.ngrok.com/hold"

		ko := &ngrokv1.KubernetesOperator{
			Name:      k8sOpName,
			Namespace: controllerNamespace,
			Spec: ngrokv1.KubernetesOperatorSpec{
				Description:     "drain lifecycle envtest",
				Metadata:        map[string]string{"owned-by": "test"},
				EnabledFeatures: []string{ngrokv1.KubernetesOperatorFeatureIngress},
				Region:          "global",
				Drain: &ngrokv1.DrainConfig{
					Policy: ngrokv1.DrainPolicyRetain,
				},
			},
		}
		service := &v1.Service{
			Name:       "drain-lifecycle",
			Namespace:  controllerNamespace,
			Finalizers: []string{util.LegacyFinalizerName},
			Spec: v1.ServiceSpec{
				Ports: []v1.ServicePort{{Port: 80}},
			},
		}

		DeferCleanup(func() {
			remainingKO := &ngrokv1.KubernetesOperator{}
			if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(ko), remainingKO); err == nil {
				if controllerutil.RemoveFinalizer(remainingKO, holdFinalizer) {
					_ = k8sClient.Update(context.Background(), remainingKO)
				}
			}
			_ = client.IgnoreNotFound(k8sClient.Delete(context.Background(), service))
		})

		By("Registering the KubernetesOperator")
		Expect(k8sClient.Create(ctx, ko)).To(Succeed())
		kginkgo.EventuallyWithObject(ctx, ko.DeepCopy(), func(g Gomega, fetched client.Object) {
			koFetched := fetched.(*ngrokv1.KubernetesOperator)
			g.Expect(koFetched.Status.ID).NotTo(BeEmpty())
			g.Expect(meta.IsStatusConditionTrue(koFetched.Status.Conditions, ngrokv1.KubernetesOperatorConditionReady)).To(BeTrue())
		}, testutils.WithTimeout(timeout))

		current := &ngrokv1.KubernetesOperator{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ko), current)).To(Succeed())
		remoteID := current.Status.ID
		controllerutil.AddFinalizer(current, holdFinalizer)
		Expect(k8sClient.Update(ctx, current)).To(Succeed())
		Expect(k8sClient.Create(ctx, service)).To(Succeed())

		By("Deleting the KubernetesOperator through the API server")
		Expect(k8sClient.Delete(ctx, current)).To(Succeed())

		By("Observing the in-progress drain conditions")
		kginkgo.EventuallyWithObject(ctx, ko.DeepCopy(), func(g Gomega, fetched client.Object) {
			koFetched := fetched.(*ngrokv1.KubernetesOperator)

			draining := meta.FindStatusCondition(koFetched.Status.Conditions, ngrokv1.KubernetesOperatorConditionDraining)
			g.Expect(draining).NotTo(BeNil())
			g.Expect(draining.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(draining.Reason).To(Equal(ngrokv1.KubernetesOperatorReasonDrainInProgress))

			ready := meta.FindStatusCondition(koFetched.Status.Conditions, ngrokv1.KubernetesOperatorConditionReady)
			g.Expect(ready).NotTo(BeNil())
			g.Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(ready.Reason).To(Equal(ngrokv1.KubernetesOperatorReasonDraining))
		}, testutils.WithTimeout(timeout), testutils.WithInterval(50*time.Millisecond))

		By("Observing the completed drain and deregistration contract")
		kginkgo.EventuallyWithObject(ctx, ko.DeepCopy(), func(g Gomega, fetched client.Object) {
			koFetched := fetched.(*ngrokv1.KubernetesOperator)
			g.Expect(koFetched.Status.ObservedGeneration).To(Equal(koFetched.Generation))
			g.Expect(koFetched.Status.ID).To(BeEmpty())
			g.Expect(util.HasFinalizer(koFetched)).To(BeFalse())
			g.Expect(controllerutil.ContainsFinalizer(koFetched, holdFinalizer)).To(BeTrue())

			draining := meta.FindStatusCondition(koFetched.Status.Conditions, ngrokv1.KubernetesOperatorConditionDraining)
			g.Expect(draining).NotTo(BeNil())
			g.Expect(draining.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(draining.Reason).To(Equal(ngrokv1.KubernetesOperatorReasonDrainCompleted))
			g.Expect(draining.ObservedGeneration).To(Equal(koFetched.Generation))

			ready := meta.FindStatusCondition(koFetched.Status.Conditions, ngrokv1.KubernetesOperatorConditionReady)
			g.Expect(ready).NotTo(BeNil())
			g.Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(ready.Reason).To(Equal(ngrokv1.KubernetesOperatorReasonDrainCompleted))
			g.Expect(ready.ObservedGeneration).To(Equal(koFetched.Generation))

			registered := meta.FindStatusCondition(koFetched.Status.Conditions, ngrokv1.KubernetesOperatorConditionRegistered)
			g.Expect(registered).NotTo(BeNil())
			g.Expect(registered.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(registered.Reason).To(Equal(ngrokv1.KubernetesOperatorReasonDeregistered))
			g.Expect(registered.ObservedGeneration).To(Equal(koFetched.Generation))

			g.Expect(koFetched.Status.Drain).NotTo(BeNil())
			g.Expect(koFetched.Status.Drain.TotalResources).To(BeNumerically(">=", 1))
			g.Expect(koFetched.Status.Drain.DrainedResources).To(Equal(koFetched.Status.Drain.TotalResources))
			g.Expect(koFetched.Status.Drain.FailedResources).To(Equal(0))
			g.Expect(koFetched.Status.Drain.Errors).To(BeEmpty())
		}, testutils.WithTimeout(timeout), testutils.WithInterval(50*time.Millisecond))

		By("Verifying Kubernetes and mock ngrok cleanup")
		fetchedService := &v1.Service{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(service), fetchedService)).To(Succeed())
		Expect(util.HasFinalizer(fetchedService)).To(BeFalse())
		_, err := mockClientset.KubernetesOperators().Get(ctx, remoteID)
		Expect(ngrok.IsNotFound(err)).To(BeTrue())

		By("Releasing the test hold finalizer")
		completed := &ngrokv1.KubernetesOperator{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ko), completed)).To(Succeed())
		Expect(controllerutil.RemoveFinalizer(completed, holdFinalizer)).To(BeTrue())
		Expect(k8sClient.Update(ctx, completed)).To(Succeed())
		Eventually(func() bool {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(ko), &ngrokv1.KubernetesOperator{})
			return apierrors.IsNotFound(err)
		}).WithTimeout(timeout).WithPolling(interval).Should(BeTrue())
	})
})
