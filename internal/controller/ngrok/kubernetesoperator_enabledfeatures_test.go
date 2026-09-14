package ngrok

import (
	"context"
	"time"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
	"github.com/ngrok/ngrok-operator/internal/testutils"
	"github.com/ngrok/ngrok-operator/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LEGACY-enabledfeatures-format: BEGIN

var _ = Describe("KubernetesOperator status.enabledFeatures format", Ordered, func() {
	const (
		timeout  = 15 * time.Second
		interval = 500 * time.Millisecond
	)

	BeforeAll(func() {
		testutils.NewKGinkgo(k8sClient).ExpectCreateNamespace(context.Background(), controllerNamespace)
	})

	// unstructuredKO reads the object without the typed decoder, so the raw
	// stored wire format of status.enabledFeatures is visible.
	unstructuredKO := func(ctx context.Context) *unstructured.Unstructured {
		GinkgoHelper()
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   ngrokv1alpha1.GroupVersion.Group,
			Version: ngrokv1alpha1.GroupVersion.Version,
			Kind:    "KubernetesOperator",
		})
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: controllerNamespace, Name: k8sOpName}, u)).To(Succeed())
		return u
	}

	AfterEach(func(ctx SpecContext) {
		ko := &ngrokv1alpha1.KubernetesOperator{}
		err := k8sClient.Get(ctx, client.ObjectKey{Namespace: controllerNamespace, Name: k8sOpName}, ko)
		if apierrors.IsNotFound(err) {
			return
		}
		Expect(err).NotTo(HaveOccurred())
		if util.RemoveFinalizer(ko) {
			Expect(k8sClient.Update(ctx, ko)).To(Succeed())
		}
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, ko))).To(Succeed())
		Eventually(func() bool {
			err := k8sClient.Get(ctx, client.ObjectKey{Namespace: controllerNamespace, Name: k8sOpName}, &ngrokv1alpha1.KubernetesOperator{})
			return apierrors.IsNotFound(err)
		}).WithTimeout(timeout).WithPolling(interval).Should(BeTrue())

		mocked := mockClientset.KubernetesOperators().(*nmockapi.KubernetesOperatorsClient)
		mocked.ClearErrors()
		mocked.Reset()
	})

	It("writes the array form and rewrites a stored legacy string on the next reconcile", func(ctx SpecContext) {
		ko := &ngrokv1alpha1.KubernetesOperator{
			Name:      k8sOpName,
			Namespace: controllerNamespace,
			Spec: ngrokv1alpha1.KubernetesOperatorSpec{
				Description:     "enabledFeatures format test",
				EnabledFeatures: []string{ngrokv1alpha1.KubernetesOperatorFeatureIngress},
				Region:          "global",
			},
		}

		By("Creating the KubernetesOperator")
		Expect(k8sClient.Create(ctx, ko)).To(Succeed())

		By("Expecting the operator to write status.enabledFeatures as an array")
		Eventually(func(g Gomega) {
			features, found, err := unstructured.NestedFieldNoCopy(unstructuredKO(ctx).Object, "status", "enabledFeatures")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(found).To(BeTrue())
			g.Expect(features).To(Equal([]any{ngrokv1alpha1.KubernetesOperatorFeatureIngress}))
		}).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(Succeed())

		By("Stamping the legacy comma-separated string a pre-migration operator would have written")
		legacy := unstructuredKO(ctx)
		Expect(unstructured.SetNestedField(legacy.Object, "ingress,bindings", "status", "enabledFeatures")).To(Succeed())
		Expect(k8sClient.Status().Update(ctx, legacy)).To(Succeed())

		stored, _, err := unstructured.NestedFieldNoCopy(unstructuredKO(ctx).Object, "status", "enabledFeatures")
		Expect(err).NotTo(HaveOccurred())
		Expect(stored).To(Equal("ingress,bindings"), "the CRD schema must still accept the legacy string")

		By("Triggering a reconcile")
		current := &ngrokv1alpha1.KubernetesOperator{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ko), current)).To(Succeed())
		current.Spec.Description = "enabledFeatures format test, touched"
		Expect(k8sClient.Update(ctx, current)).To(Succeed())

		By("Expecting the stored legacy string to be replaced by the array form")
		Eventually(func(g Gomega) {
			features, found, err := unstructured.NestedFieldNoCopy(unstructuredKO(ctx).Object, "status", "enabledFeatures")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(found).To(BeTrue())
			g.Expect(features).To(Equal([]any{ngrokv1alpha1.KubernetesOperatorFeatureIngress}))
		}).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
	})

	// The Schemaless/PreserveUnknownFields markers on the field outlive
	// MarshalJSON by one release: the previous release's operator still writes
	// the legacy string, and a strict `type: array` rejects that write on
	// create and on any change to the feature set. Tightening the schema is
	// only safe once UnmarshalJSON goes too, so fail here if the markers are
	// dropped while the decoder is still in the tree.
	It("keeps the CRD schema loose while UnmarshalJSON is still present", func(ctx SpecContext) {
		crd := &unstructured.Unstructured{}
		crd.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "apiextensions.k8s.io",
			Version: "v1",
			Kind:    "CustomResourceDefinition",
		})
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "kubernetesoperators.ngrok.k8s.ngrok.com"}, crd)).To(Succeed())

		versions, found, err := unstructured.NestedSlice(crd.Object, "spec", "versions")
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())

		var checked int
		for _, v := range versions {
			version, ok := v.(map[string]any)
			Expect(ok).To(BeTrue())

			features, found, err := unstructured.NestedMap(version,
				"schema", "openAPIV3Schema", "properties", "status", "properties", "enabledFeatures")
			Expect(err).NotTo(HaveOccurred())
			if !found {
				continue
			}
			checked++

			Expect(features).To(HaveKeyWithValue("x-kubernetes-preserve-unknown-fields", true))
			Expect(features).NotTo(HaveKey("type"),
				"status.enabledFeatures must stay schemaless until UnmarshalJSON is removed; see kubernetesoperator_status_compat.go")
		}
		Expect(checked).To(BeNumerically(">", 0), "status.enabledFeatures not found in any served version")
	})
})

// LEGACY-enabledfeatures-format: END
