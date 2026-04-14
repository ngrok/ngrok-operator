package ngrok

import (
	"context"
	"testing"
	"time"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
	"github.com/ngrok/ngrok-operator/internal/testutils"
	"github.com/ngrok/ngrok-operator/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// TestCalculateFeaturesEnabled is a pure unit test for the calculateFeaturesEnabled function.
func TestCalculateFeaturesEnabled(t *testing.T) {
	tc := []struct {
		name     string
		in       *ngrokv1alpha1.KubernetesOperator
		expected []string
	}{
		{
			name: "no features enabled",
			in: &ngrokv1alpha1.KubernetesOperator{
				Spec: ngrokv1alpha1.KubernetesOperatorSpec{},
			},
			expected: []string{},
		},
		{
			name: "all features enabled",
			in: &ngrokv1alpha1.KubernetesOperator{
				Spec: ngrokv1alpha1.KubernetesOperatorSpec{
					EnabledFeatures: []string{
						ngrokv1alpha1.KubernetesOperatorFeatureBindings,
						ngrokv1alpha1.KubernetesOperatorFeatureIngress,
						ngrokv1alpha1.KubernetesOperatorFeatureGateway,
					},
				},
			},
			expected: []string{"bindings", "ingress", "gateway"},
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, calculateFeaturesEnabled(tt.in))
		})
	}
}

var _ = Describe("KubernetesOperator Controller", Ordered, func() {
	const (
		timeout  = 15 * time.Second
		interval = 500 * time.Millisecond
	)

	var kginkgo *testutils.KGinkgo

	BeforeAll(func() {
		kginkgo = testutils.NewKGinkgo(k8sClient)

		// Ensure the controller namespace exists
		ns := &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: controllerNamespace,
			},
		}
		err := k8sClient.Create(context.Background(), ns)
		if err != nil {
			Expect(client.IgnoreAlreadyExists(err)).To(Succeed())
		}
	})

	// forceDeleteKO removes the finalizer and deletes the KubernetesOperator to
	// avoid triggering the drain workflow during test cleanup.
	forceDeleteKO := func(ctx context.Context) {
		ko := &ngrokv1alpha1.KubernetesOperator{}
		err := k8sClient.Get(ctx, client.ObjectKey{
			Namespace: controllerNamespace,
			Name:      k8sOpName,
		}, ko)
		if apierrors.IsNotFound(err) {
			return
		}
		Expect(err).NotTo(HaveOccurred())

		if controllerutil.RemoveFinalizer(ko, util.FinalizerName) {
			Expect(k8sClient.Update(ctx, ko)).To(Succeed())
		}
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, ko))).To(Succeed())

		// Wait for it to actually be gone
		Eventually(func() bool {
			err := k8sClient.Get(ctx, client.ObjectKey{
				Namespace: controllerNamespace,
				Name:      k8sOpName,
			}, &ngrokv1alpha1.KubernetesOperator{})
			return apierrors.IsNotFound(err)
		}).WithTimeout(timeout).WithPolling(interval).Should(BeTrue())
	}

	AfterEach(func() {
		mockClientset.KubernetesOperators().(*nmockapi.KubernetesOperatorsClient).Reset()
		forceDeleteKO(context.Background())
	})

	It("should register successfully with ingress feature enabled", func(ctx SpecContext) {
		ko := &ngrokv1alpha1.KubernetesOperator{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k8sOpName,
				Namespace: controllerNamespace,
			},
			Spec: ngrokv1alpha1.KubernetesOperatorSpec{
				Description:     "test operator",
				Metadata:        `{"owned-by":"test"}`,
				EnabledFeatures: []string{ngrokv1alpha1.KubernetesOperatorFeatureIngress},
				Region:          "global",
			},
		}

		By("Creating the KubernetesOperator")
		Expect(k8sClient.Create(ctx, ko)).To(Succeed())

		By("Expecting the finalizer to be added")
		kginkgo.ExpectFinalizerToBeAdded(ctx, ko, util.FinalizerName, testutils.WithTimeout(timeout))

		By("Expecting registration to succeed")
		kginkgo.EventuallyWithObject(ctx, ko.DeepCopy(), func(g Gomega, fetched client.Object) {
			koFetched := fetched.(*ngrokv1alpha1.KubernetesOperator)
			g.Expect(koFetched.Status.ID).NotTo(BeEmpty())
			g.Expect(koFetched.Status.RegistrationStatus).To(Equal(ngrokv1alpha1.KubernetesOperatorRegistrationStatusSuccess))
		}, testutils.WithTimeout(timeout))
	})

	It("should not panic with bindings enabled but nil Binding spec", func(ctx SpecContext) {
		ko := &ngrokv1alpha1.KubernetesOperator{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k8sOpName,
				Namespace: controllerNamespace,
			},
			Spec: ngrokv1alpha1.KubernetesOperatorSpec{
				Description:     "test operator with nil binding",
				Metadata:        `{"owned-by":"test"}`,
				EnabledFeatures: []string{ngrokv1alpha1.KubernetesOperatorFeatureBindings},
				Binding:         nil,
				Region:          "global",
			},
		}

		By("Creating the KubernetesOperator")
		Expect(k8sClient.Create(ctx, ko)).To(Succeed())

		By("Expecting the finalizer to be added (controller ran without panic)")
		kginkgo.ExpectFinalizerToBeAdded(ctx, ko, util.FinalizerName, testutils.WithTimeout(timeout))

		By("Expecting the status to reflect a pending state (not registered)")
		kginkgo.EventuallyWithObject(ctx, ko.DeepCopy(), func(g Gomega, fetched client.Object) {
			koFetched := fetched.(*ngrokv1alpha1.KubernetesOperator)
			// The nil binding guard prevents a panic and returns an error,
			// which updateStatus records. The ID stays empty because no
			// ngrok API object was created.
			g.Expect(koFetched.Status.ID).To(BeEmpty())
		}, testutils.WithTimeout(timeout))
	})

	It("should not panic with nil Deployment and should register successfully", func(ctx SpecContext) {
		ko := &ngrokv1alpha1.KubernetesOperator{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k8sOpName,
				Namespace: controllerNamespace,
			},
			Spec: ngrokv1alpha1.KubernetesOperatorSpec{
				Description:     "test operator with nil deployment",
				Metadata:        `{"owned-by":"test"}`,
				EnabledFeatures: []string{ngrokv1alpha1.KubernetesOperatorFeatureIngress},
				Deployment:      nil,
				Region:          "global",
			},
		}

		By("Creating the KubernetesOperator")
		Expect(k8sClient.Create(ctx, ko)).To(Succeed())

		By("Expecting the finalizer to be added")
		kginkgo.ExpectFinalizerToBeAdded(ctx, ko, util.FinalizerName, testutils.WithTimeout(timeout))

		By("Expecting registration to succeed even with nil Deployment")
		kginkgo.EventuallyWithObject(ctx, ko.DeepCopy(), func(g Gomega, fetched client.Object) {
			koFetched := fetched.(*ngrokv1alpha1.KubernetesOperator)
			g.Expect(koFetched.Status.ID).NotTo(BeEmpty())
			g.Expect(koFetched.Status.RegistrationStatus).To(Equal(ngrokv1alpha1.KubernetesOperatorRegistrationStatusSuccess))
		}, testutils.WithTimeout(timeout))
	})
})
