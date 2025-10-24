package testutils

import (
	"context"
	"time"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultTimeout  = 20 * time.Second
	DefaultInterval = 500 * time.Millisecond
)

// KGinkgo is a helper for Ginkgo tests that interact with Kubernetes resources.
// It provides methods to assert conditions on Kubernetes objects using Gomega matchers, especially in conjunction with Eventually.
type KGinkgo struct {
	client client.Client
}

// NewKGinkgo creates a new KGinkgo instance
func NewKGinkgo(c client.Client) *KGinkgo {
	return &KGinkgo{
		client: c,
	}
}

type expectOptions struct {
	timeout  time.Duration
	interval time.Duration
}

type KGinkgoOpt func(*expectOptions)

func WithTimeout(timeout time.Duration) KGinkgoOpt {
	return func(o *expectOptions) {
		o.timeout = timeout
	}
}

func WithInterval(interval time.Duration) KGinkgoOpt {
	return func(o *expectOptions) {
		o.interval = interval
	}
}

// ConsistentlyWithCloudEndpoints continually fetches the CloudEndpoints in the given namespace and invokes the inner function with the list.
// The function uses Gomega's Consistently internally, so it should only be used in Ginkgo tests.
// You can pass optional KGinkgoOpt parameters to customize the timeout and polling interval.
//
// Example usage:
//
//	    check := func(g Gomega, cleps []ngrokv1alpha1.CloudEndpoint) {
//	        g.Expect(len(cleps)).To(Equal(2))
//		}
//
//		kginkgo := testutils.NewKGinkgo(k8sClient)
//	    kginkgo.ConsistentlyWithCloudEndpoints(ctx, "test-namespace", check)
func (k *KGinkgo) ConsistentlyWithCloudEndpoints(ctx context.Context, namespace string, inner func(g Gomega, cleps []ngrokv1alpha1.CloudEndpoint), opts ...KGinkgoOpt) {
	GinkgoHelper()
	eo := makeKGinkgoOptions(opts...)

	Consistently(func(g Gomega) {
		// List CloudEndpoints in the namespace
		cleps, err := k.getCloudEndpoints(ctx, namespace)
		g.Expect(err).NotTo(HaveOccurred())

		inner(g, cleps)
	}).WithTimeout(eo.timeout).WithPolling(eo.interval).Should(Succeed())
}

// ConsistentlyExpectResourceVersionNotToChange asserts that the resource version of the given Kubernetes object does not change over time.
// The function uses Gomega's Consistently internally, so it should only be used in Ginkgo tests.
// You can pass optional KGinkgoOpt parameters to customize the timeout and polling interval.
//
// Example usage:
//
//		kginkgo := testutils.NewKGinkgo(k8sClient)
//	    kginkgo.ConsistentlyExpectResourceVersionNotToChange(ctx, myObject)
func (k *KGinkgo) ConsistentlyExpectResourceVersionNotToChange(ctx context.Context, obj client.Object, opts ...KGinkgoOpt) {
	GinkgoHelper()

	eo := makeKGinkgoOptions(opts...)
	objKey := client.ObjectKeyFromObject(obj)

	initialResourceVersion := ""

	Consistently(func(g Gomega) {
		fetched := obj.DeepCopyObject().(client.Object)
		g.Expect(k.client.Get(ctx, objKey, fetched)).NotTo(HaveOccurred())

		if initialResourceVersion == "" {
			initialResourceVersion = fetched.GetResourceVersion()
		}

		g.Expect(fetched.GetResourceVersion()).To(Equal(initialResourceVersion))
	}).WithTimeout(eo.timeout).WithPolling(eo.interval).Should(Succeed())
}

// ExpectCreateNamespace creates a namespace with the given name.
// The function uses Gomega's Expect internally, so it should only be used in Ginkgo tests.
//
// Example usage:
//
//	kginkgo := testutils.NewKGinkgo(k8sClient)
//	namespace := "test-namespace"
//	kginkgo.ExpectCreateNamespace(ctx, namespace)
//	defer testutils.ExpectDeleteNamespace(namespace)
func (k *KGinkgo) ExpectCreateNamespace(ctx context.Context, name string) {
	GinkgoHelper()

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	Expect(k.client.Create(ctx, ns)).To(Succeed())
}

// ExpectDeleteNamespace deletes a namespace with the given name.
// The function uses Gomega's Expect internally and expects the delete to succeed or return NotFound.
// This is useful for cleaning up namespaces in defer statements.
//
// Example usage:
//
//	kginkgo := testutils.NewKGinkgo(k8sClient)
//	namespace := "test-namespace"
//	defer kginkgo.ExpectDeleteNamespace(ctx, namespace)
func (k *KGinkgo) ExpectDeleteNamespace(ctx context.Context, name string) {
	GinkgoHelper()

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	err := k.client.Delete(ctx, ns)
	if err != nil && !apierrors.IsNotFound(err) {
		Expect(err).NotTo(HaveOccurred())
	}
}

// ExpectFinalizerToBeAdded asserts that the specified finalizer is eventually added to the given Kubernetes object.
// It will continually update the object from the client to check for the finalizer
// The function uses Gomega's Eventually internally, so it should only be used in Ginkgo tests.
// You can pass optional KGinkgoOpt parameters to customize the timeout and polling interval.
//
// Example usage:
//
// kginkgo := testutils.NewKGinkgo(k8sClient)
// kginkgo.ExpectFinalizerToBeAdded(ctx, myObject, "my.finalizer.io")
func (k *KGinkgo) ExpectFinalizerToBeAdded(ctx context.Context, obj client.Object, finalizer string, opts ...KGinkgoOpt) {
	GinkgoHelper()

	eo := makeKGinkgoOptions(opts...)
	key := client.ObjectKeyFromObject(obj)

	Eventually(func(g Gomega) {
		fetched := &corev1.Service{}
		err := k.client.Get(ctx, key, fetched)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(fetched.GetFinalizers()).To(ContainElement(finalizer))
	}).WithTimeout(eo.timeout).WithPolling(eo.interval).Should(Succeed())
}

// ExpectFinalizerToExist asserts that the specified finalizer eventually exists on the given Kubernetes object.
// It will continually update the object from the client to check for the finalizer
// The function uses Gomega's Eventually internally, so it should only be used in Ginkgo tests.
// You can pass optional KGinkgoOpt parameters to customize the timeout and polling interval.
//
// Example usage:
//
// kginkgo := testutils.NewKGinkgo(k8sClient)
// kginkgo.ExpectFinalizerToBeRemoved(ctx, myObject, "my.finalizer.io")
func (k *KGinkgo) ExpectFinalizerToBeRemoved(ctx context.Context, obj client.Object, finalizer string, opts ...KGinkgoOpt) {
	GinkgoHelper()

	eo := makeKGinkgoOptions(opts...)
	key := client.ObjectKeyFromObject(obj)

	Eventually(func(g Gomega) {
		fetched := &corev1.Service{}
		err := k.client.Get(ctx, key, fetched)

		// If the object is not found, the finalizer has been removed and the object deleted
		if client.IgnoreNotFound(err) == nil {
			return
		}

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(fetched.GetFinalizers()).ToNot(ContainElement(finalizer))
	}).WithTimeout(eo.timeout).WithPolling(eo.interval).Should(Succeed())
}

// ExpectHasAnnotation asserts that the given Kubernetes object eventually has the specified annotation key.
// The function uses Gomega's Eventually internally, so it should only be used in Ginkgo tests.
// You can pass optional KGinkgoOpt parameters to customize the timeout and polling interval.
//
// Example usage:
//
// kginkgo := testutils.NewKGinkgo(k8sClient)
// kginkgo.ExpectHasAnnotation(ctx, myObject, "my.annotation/key")
func (k *KGinkgo) ExpectHasAnnotation(ctx context.Context, obj client.Object, key string, opts ...KGinkgoOpt) {
	GinkgoHelper()

	k.EventuallyWithObject(ctx, obj, func(g Gomega, fetched client.Object) {
		annotations := fetched.GetAnnotations()
		g.Expect(annotations).NotTo(BeEmpty())

		g.Expect(annotations).To(HaveKey(key))
	}, opts...)
}

// ExpectAnnotationValue asserts that the given Kubernetes object eventually has the specified annotation key with the expected value.
// It will continually update the object from the client to check for the annotation and its value.
// The function uses Gomega's Eventually internally, so it should only be used in Ginkgo tests.
// You can pass optional KGinkgoOpt parameters to customize the timeout and polling interval.
//
// Example usage:
//
// kginkgo := testutils.NewKGinkgo(k8sClient)
// kginkgo.ExpectAnnotationValue(ctx, myObject, "my.annotation/key", "expected-value")
func (k *KGinkgo) ExpectAnnotationValue(ctx context.Context, obj client.Object, key, expectedValue string, opts ...KGinkgoOpt) {
	GinkgoHelper()

	k.EventuallyWithObject(ctx, obj, func(g Gomega, fetched client.Object) {
		annotations := fetched.GetAnnotations()
		g.Expect(annotations).NotTo(BeEmpty())

		actualValue, exists := annotations[key]
		g.Expect(exists).To(BeTrue(), "expected annotation %q to exist", key)
		g.Expect(actualValue).To(Equal(expectedValue), "expected annotation %q to have value %q but got %q", key, expectedValue, actualValue)
	}, opts...)
}

// EventuallyWithObject continually fetches the given Kubernetes object and invokes the inner function with the fetched object.
// The function uses Gomega's Eventually internally, so it should only be used in Ginkgo tests.
// You can pass optional KGinkgoOpt parameters to customize the timeout and polling interval.
//
// Example usage:
//
//	    check := func(g Gomega, fetched client.Object) {
//	        g.Expect(fetched.GetAnnotations()).To(HaveKey("my.annotation/key"))
//	    }
//
//		kginkgo := testutils.NewKGinkgo(k8sClient)
//		kginkgo.EventuallyWithObject(ctx, myObject, check)
func (k *KGinkgo) EventuallyWithObject(ctx context.Context, obj client.Object, inner func(g Gomega, fetched client.Object), opts ...KGinkgoOpt) {
	GinkgoHelper()

	eo := makeKGinkgoOptions(opts...)
	objKey := client.ObjectKeyFromObject(obj)

	Eventually(func(g Gomega) {
		fetched := obj.DeepCopyObject().(client.Object)
		g.Expect(k.client.Get(ctx, objKey, fetched)).NotTo(HaveOccurred())

		inner(g, fetched)
	}).WithTimeout(eo.timeout).WithPolling(eo.interval).Should(Succeed())
}

// EventuallyWithCloudEndpoints continually fetches the CloudEndpoints in the given namespace and invokes the inner function with the list.
// The function uses Gomega's Eventually internally, so it should only be used in Ginkgo tests.
// You can pass optional KGinkgoOpt parameters to customize the timeout and polling interval.
//
// Example usage:
//
//	    check := func(g Gomega, cleps []ngrokv1alpha1.CloudEndpoint) {
//	        g.Expect(len(cleps)).To(Equal(2))
//		}
//
//		kginkgo := testutils.NewKGinkgo(k8sClient)
//	    kginkgo.EventuallyWithCloudEndpoints(ctx, "test-namespace", check)
func (k *KGinkgo) EventuallyWithCloudEndpoints(ctx context.Context, namespace string, inner func(g Gomega, cleps []ngrokv1alpha1.CloudEndpoint), opts ...KGinkgoOpt) {
	GinkgoHelper()
	eo := makeKGinkgoOptions(opts...)

	Eventually(func(g Gomega) {
		// List CloudEndpoints in the namespace
		cleps, err := k.getCloudEndpoints(ctx, namespace)
		g.Expect(err).NotTo(HaveOccurred())

		inner(g, cleps)
	}).WithTimeout(eo.timeout).WithPolling(eo.interval).Should(Succeed())
}

// EventuallyWithAgentEndpoints continually fetches the AgentEndpoints in the given namespace and invokes the inner function with the list.
// The function uses Gomega's Eventually internally, so it should only be used in Ginkgo tests.
// You can pass optional KGinkgoOpt parameters to customize the timeout and polling interval.
//
// Example usage:
//
//	    check := func(g Gomega, aeps []ngrokv1alpha1.AgentEndpoint) {
//	        g.Expect(len(aeps)).To(Equal(2))
//		}
//
//		kginkgo := testutils.NewKGinkgo(k8sClient)
//	    kginkgo.EventuallyWithAgentEndpoints(ctx, "test-namespace", check)
func (k *KGinkgo) EventuallyWithAgentEndpoints(ctx context.Context, namespace string, inner func(g Gomega, aeps []ngrokv1alpha1.AgentEndpoint), opts ...KGinkgoOpt) {
	GinkgoHelper()
	eo := makeKGinkgoOptions(opts...)

	Eventually(func(g Gomega) {
		// List AgentEndpoints in the namespace
		aeps, err := k.getAgentEndpoints(ctx, namespace)
		g.Expect(err).NotTo(HaveOccurred())

		inner(g, aeps)
	}).WithTimeout(eo.timeout).WithPolling(eo.interval).Should(Succeed())
}

// EventuallyWithCloudAndAgentEndpoints continually fetches both CloudEndpoints and AgentEndpoints in the given namespace
// and invokes the inner function with both lists.
// The function uses Gomega's Eventually internally, so it should only be used in Ginkgo tests.
// You can pass optional KGinkgoOpt parameters to customize the timeout and polling interval.
//
// Example usage:
//
//	    check := func(g Gomega, cleps []ngrokv1alpha1.CloudEndpoint, aeps []ngrokv1alpha1.AgentEndpoint) {
//	        g.Expect(len(cleps)).To(Equal(2))
//	        g.Expect(len(aeps)).To(Equal(3))
//		}
//
//		kginkgo := testutils.NewKGinkgo(k8sClient)
//	    kginkgo.EventuallyWithCloudAndAgentEndpoints(ctx, "test-namespace", check)
func (k *KGinkgo) EventuallyWithCloudAndAgentEndpoints(ctx context.Context, namespace string, inner func(g Gomega, cleps []ngrokv1alpha1.CloudEndpoint, aeps []ngrokv1alpha1.AgentEndpoint), opts ...KGinkgoOpt) {
	GinkgoHelper()
	eo := makeKGinkgoOptions(opts...)

	Eventually(func(g Gomega) {
		// List CloudEndpoints in the namespace
		cleps, err := k.getCloudEndpoints(ctx, namespace)
		g.Expect(err).NotTo(HaveOccurred())

		// List AgentEndpoints in the namespace
		aeps, err := k.getAgentEndpoints(ctx, namespace)
		g.Expect(err).NotTo(HaveOccurred())

		inner(g, cleps, aeps)
	}).WithTimeout(eo.timeout).WithPolling(eo.interval).Should(Succeed())
}

// EventuallyExpectNoEndpoints asserts that there are eventually no CloudEndpoints or AgentEndpoints in the given namespace.
// It uses Gomega's Eventually internally, so it should only be used in Ginkgo tests.
// You can pass optional KGinkgoOpt parameters to customize the timeout and polling interval.
//
// Example usage:
//
//		kginkgo := testutils.NewKGinkgo(k8sClient)
//	    kginkgo.EventuallyExpectNoEndpoints(ctx, "test-namespace")
func (k *KGinkgo) EventuallyExpectNoEndpoints(ctx context.Context, namespace string, opts ...KGinkgoOpt) {
	GinkgoHelper()

	By("verifying no cloud or agent endpoints remain")
	k.EventuallyWithCloudAndAgentEndpoints(ctx, namespace, func(g Gomega, cleps []ngrokv1alpha1.CloudEndpoint, aeps []ngrokv1alpha1.AgentEndpoint) {
		By("verifying no cloud endpoints remain")
		g.Expect(cleps).To(BeEmpty())

		By("verifying no agent endpoints remain")
		g.Expect(aeps).To(BeEmpty())
	}, opts...)
}

func (k *KGinkgo) getCloudEndpoints(ctx context.Context, namespace string) ([]ngrokv1alpha1.CloudEndpoint, error) {
	GinkgoHelper()

	clepList := &ngrokv1alpha1.CloudEndpointList{}
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
	}
	if err := k.client.List(ctx, clepList, listOpts...); err != nil {
		return nil, err
	}
	return clepList.Items, nil
}

func (k *KGinkgo) getAgentEndpoints(ctx context.Context, namespace string) ([]ngrokv1alpha1.AgentEndpoint, error) {
	GinkgoHelper()

	aepList := &ngrokv1alpha1.AgentEndpointList{}
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
	}
	if err := k.client.List(ctx, aepList, listOpts...); err != nil {
		return nil, err
	}
	return aepList.Items, nil
}

func makeKGinkgoOptions(opts ...KGinkgoOpt) *expectOptions {
	eo := &expectOptions{
		timeout:  DefaultTimeout,
		interval: DefaultInterval,
	}
	for _, o := range opts {
		o(eo)
	}
	return eo
}
