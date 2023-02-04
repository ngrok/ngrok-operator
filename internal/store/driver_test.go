package store

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
)

var _ = Describe("Driver", func() {

	var driver *Driver
	var scheme = runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(ingressv1alpha1.AddToScheme(scheme))
	BeforeEach(func() {
		// create a fake logger to pass into the cachestore
		logger := logr.New(logr.Discard().GetSink())
		driver = NewDriver(logger, scheme)
		driver.bypassReentranceCheck = true
	})

	Describe("Seed", func() {
		It("Should not error", func() {
			err := driver.Seed(context.Background(), fake.NewFakeClientWithScheme(scheme))
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should add all the found items to the store", func() {
			i1 := NewTestIngressV1("test-ingress", "test-namespace")
			i2 := NewTestIngressV1("test-ingress-2", "test-namespace")
			ic1 := NewTestIngressClass("test-ingress-class", true, true)
			ic2 := NewTestIngressClass("test-ingress-class-2", true, true)
			d1 := NewDomainV1("test-domain.com", "test-namespace")
			d2 := NewDomainV1("test-domain-2.com", "test-namespace")
			e1 := NewHTTPSEdge("test-edge", "test-namespace", "test-domain.com")
			e2 := NewHTTPSEdge("test-edge-2", "test-namespace", "test-domain-2.com")
			obs := []runtime.Object{&ic1, &ic2, &i1, &i2, &d1, &d2, &e1, &e2}

			c := fake.NewFakeClientWithScheme(scheme, obs...)
			err := driver.Seed(context.Background(), c)
			Expect(err).ToNot(HaveOccurred())

			for _, obj := range obs {
				foundObj, found, err := driver.Get(obj)
				Expect(err).ToNot(HaveOccurred())
				Expect(found).To(BeTrue())
				Expect(foundObj).ToNot(BeNil())
				Expect(foundObj).To(Equal(obj))
			}
		})
	})

	Describe("DeleteIngress", func() {
		It("Should remove the ingress from the store", func() {
			i1 := NewTestIngressV1("test-ingress", "test-namespace")
			c := fake.NewFakeClientWithScheme(scheme, &i1)
			err := driver.Seed(context.Background(), c)
			Expect(err).ToNot(HaveOccurred())

			err = driver.DeleteIngress(types.NamespacedName{
				Namespace: "test-namespace",
				Name:      "test-ingress",
			})
			Expect(err).ToNot(HaveOccurred())

			foundObj, found, err := driver.Get(&i1)
			Expect(err).ToNot(HaveOccurred())
			Expect(found).To(BeFalse())
			Expect(foundObj).To(BeNil())
		})
	})

	Describe("Sync", func() {
		Context("When there are no ingresses in the store", func() {
			It("Should not create anything or error", func() {
				c := fake.NewFakeClientWithScheme(scheme)
				err := driver.Sync(context.Background(), c)
				Expect(err).ToNot(HaveOccurred())

				domains := &ingressv1alpha1.DomainList{}
				err = c.List(context.Background(), &ingressv1alpha1.DomainList{})
				Expect(err).ToNot(HaveOccurred())
				Expect(domains.Items).To(HaveLen(0))

				edges := &ingressv1alpha1.HTTPSEdgeList{}
				err = c.List(context.Background(), &ingressv1alpha1.HTTPSEdgeList{})
				Expect(err).ToNot(HaveOccurred())
				Expect(edges.Items).To(HaveLen(0))

				tunnels := &ingressv1alpha1.TunnelList{}
				err = c.List(context.Background(), &ingressv1alpha1.TunnelList{})
				Expect(err).ToNot(HaveOccurred())
				Expect(tunnels.Items).To(HaveLen(0))
			})
		})
		Context("When there are just ingresses and CRDs need to be created", func() {
			It("Should create the CRDs", func() {
				i1 := NewTestIngressV1("test-ingress", "test-namespace")
				i2 := NewTestIngressV1("test-ingress-2", "test-namespace")
				ic1 := NewTestIngressClass("test-ingress-class", true, true)
				ic2 := NewTestIngressClass("test-ingress-class-2", true, true)
				obs := []runtime.Object{&ic1, &ic2, &i1, &i2}
				c := fake.NewFakeClientWithScheme(scheme, obs...)

				for _, obj := range obs {
					err := driver.Update(obj)
					Expect(err).ToNot(HaveOccurred())
				}
				err := driver.Seed(context.Background(), c)
				Expect(err).ToNot(HaveOccurred())

				err = driver.Sync(context.Background(), c)
				Expect(err).ToNot(HaveOccurred())

				foundDomain := &ingressv1alpha1.Domain{}
				err = c.Get(context.Background(), types.NamespacedName{
					Namespace: "test-namespace",
					Name:      "example-com",
				}, foundDomain)
				Expect(err).ToNot(HaveOccurred())
				Expect(foundDomain.Spec.Domain).To(Equal(i1.Spec.Rules[0].Host))

				foundEdge := &ingressv1alpha1.HTTPSEdge{}
				err = c.Get(context.Background(), types.NamespacedName{
					Namespace: "test-namespace",
					Name:      "example-com",
				}, foundEdge)
				Expect(err).ToNot(HaveOccurred())
				Expect(foundEdge.Spec.Hostports[0]).To(ContainSubstring(i1.Spec.Rules[0].Host))

				foundTunnel := &ingressv1alpha1.Tunnel{}
				err = c.Get(context.Background(), types.NamespacedName{
					Namespace: "test-namespace",
					Name:      "example-80",
				}, foundTunnel)
				Expect(err).ToNot(HaveOccurred())
				Expect(foundTunnel).ToNot(BeNil())
			})
		})
	})
})
