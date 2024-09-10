package store

import (
	"encoding/json"
	"testing"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-operator/internal/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	netv1 "k8s.io/api/networking/v1"
)

const ngrokIngressClass = "ngrok"
const defaultControllerName = "k8s.ngrok.com/ingress-controller"

func TestStore(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Store package Test Suite")
}

var _ = Describe("Store", func() {

	var store Storer
	BeforeEach(func() {
		// create a fake logger to pass into the cachestore
		logger := logr.New(logr.Discard().GetSink())
		cacheStores := NewCacheStores(logger)
		store = New(cacheStores, defaultControllerName, logger)
	})

	var _ = Describe("GetIngressClassV1", func() {
		Context("when the ingress class exists", func() {
			BeforeEach(func() {
				ic := NewTestIngressClass(ngrokIngressClass, true, true)
				Expect(store.Add(&ic)).To(BeNil())
			})
			It("returns the ingress class", func() {
				ic, err := store.GetIngressClassV1(ngrokIngressClass)
				Expect(err).ToNot(HaveOccurred())
				Expect(ic.Name).To(Equal(ngrokIngressClass))
			})
		})
		Context("when the ingress class does not exist", func() {
			It("returns an error", func() {
				ic, err := store.GetIngressClassV1("does-not-exist")
				Expect(err).To(HaveOccurred())
				Expect(ic).To(BeNil())
			})
		})
	})

	var _ = Describe("GetIngressV1", func() {
		Context("when the ingress exists", func() {
			BeforeEach(func() {
				ing := NewTestIngressV1("test-ingress", "test-namespace")
				Expect(store.Add(&ing)).To(BeNil())
			})
			It("returns the ingress", func() {
				ing, err := store.GetIngressV1("test-ingress", "test-namespace")
				Expect(err).ToNot(HaveOccurred())
				Expect(ing.Name).To(Equal("test-ingress"))
			})
		})
		Context("when the ingress does not exist", func() {
			It("returns an error", func() {
				ing, err := store.GetIngressV1("does-not-exist", "does-not-exist")
				Expect(err).To(HaveOccurred())
				Expect(ing).To(BeNil())
			})
		})
	})

	var _ = Describe("GetServiceV1", func() {
		Context("when the service exists", func() {
			BeforeEach(func() {
				svc := NewTestServiceV1("test-service", "test-namespace")
				Expect(store.Add(&svc)).To(BeNil())
			})
			It("returns the service", func() {
				svc, err := store.GetServiceV1("test-service", "test-namespace")
				Expect(err).ToNot(HaveOccurred())
				Expect(svc.Name).To(Equal("test-service"))
			})
		})
		Context("when the service does not exist", func() {
			It("returns an error", func() {
				svc, err := store.GetServiceV1("does-not-exist", "does-not-exist")
				Expect(err).To(HaveOccurred())
				Expect(svc).To(BeNil())
			})
		})
	})

	var _ = Describe("GetNgrokIngressV1", func() {
		Context("when the ngrok ingress exists", func() {
			BeforeEach(func() {
				ing := NewTestIngressV1WithClass("test-ingress", "test-namespace", ngrokIngressClass)
				Expect(store.Add(&ing)).To(BeNil())
				ic := NewTestIngressClass(ngrokIngressClass, true, true)
				Expect(store.Add(&ic)).To(BeNil())
			})
			It("returns the ngrok ingress", func() {
				ing, err := store.GetNgrokIngressV1("test-ingress", "test-namespace")
				Expect(err).ToNot(HaveOccurred())
				Expect(ing.Name).To(Equal("test-ingress"))
			})
			It("Filters out ingresses that don't match the ngrok ingress class", func() {
				ingNotNgrok := NewTestIngressV1WithClass("ingNotNgrok", "test-namespace", "not-ngrok")
				Expect(store.Add(&ingNotNgrok)).To(BeNil())

				ing, err := store.GetNgrokIngressV1("ingNotNgrok", "test-namespace")
				Expect(err).To(HaveOccurred())
				Expect(ing).To(BeNil())
			})
			It("Filters finds ones without a class if we are default", func() {
				ingNoClass := NewTestIngressV1("ingNoClass", "test-namespace")
				Expect(store.Add(&ingNoClass)).To(BeNil())

				ing, err := store.GetNgrokIngressV1("ingNoClass", "test-namespace")
				Expect(err).ToNot(HaveOccurred())
				Expect(ing.Name).To(Equal("ingNoClass"))
			})
		})
		Context("when the ngrok ingress does not exist", func() {
			It("returns an error", func() {
				ing, err := store.GetNgrokIngressV1("does-not-exist", "does-not-exist")
				Expect(err).To(HaveOccurred())
				Expect(ing).To(BeNil())
			})
		})
	})

	var _ = Describe("ListNgrokIngressClassesV1", func() {
		Context("when there are ngrok ingress classes", func() {
			BeforeEach(func() {
				ic1 := NewTestIngressClass("ngrok1", true, true)
				Expect(store.Add(&ic1)).To(BeNil())
				ic2 := NewTestIngressClass("ngrok2", true, true)
				Expect(store.Add(&ic2)).To(BeNil())
				ic3 := NewTestIngressClass("different", true, false)
				Expect(store.Add(&ic3)).To(BeNil())
			})
			It("returns the ngrok ingress classes and doesn't return the different one", func() {
				ics := store.ListNgrokIngressClassesV1()
				Expect(len(ics)).To(Equal(2))
			})
		})
		Context("when there are no ngrok ingress classes", func() {
			It("doesn't error", func() {
				ics := store.ListNgrokIngressClassesV1()
				Expect(len(ics)).To(Equal(0))
			})
		})
	})

	var _ = Describe("ListNgrokIngressesV1", func() {
		icUsDefault := NewTestIngressClass("ngrok", true, true)
		icUsNotDefault := NewTestIngressClass("ngrok", false, true)
		icOtherDefault := NewTestIngressClass("test", true, false)
		icOtherNotDefault := NewTestIngressClass("test", false, false)

		var _ = DescribeTable("IngressClassFiltering", func(ingressClasses []netv1.IngressClass, expectedMatchingIngressesCount int) {
			iMatching := NewTestIngressV1WithClass("test1", "test", "ngrok")
			iNotMatching := NewTestIngressV1WithClass("test2", "test", "test")
			iNoClass := NewTestIngressV1("test3", "test")
			Expect(store.Add(&iMatching)).To(BeNil())
			Expect(store.Add(&iNotMatching)).To(BeNil())
			Expect(store.Add(&iNoClass)).To(BeNil())
			for _, ic := range ingressClasses {
				Expect(store.Add(&ic)).To(BeNil())
			}
			ings := store.ListNgrokIngressesV1()
			Expect(len(ings)).To(Equal(expectedMatchingIngressesCount))
		},
			Entry("No ingress classes", []netv1.IngressClass{}, 0),
			Entry("just us not as default", []netv1.IngressClass{icUsNotDefault}, 1),
			Entry("just us as default", []netv1.IngressClass{icUsDefault}, 2),
			Entry("just another not as default", []netv1.IngressClass{icOtherNotDefault}, 0),
			Entry("just another as default", []netv1.IngressClass{icOtherDefault}, 0),
			Entry("us and another neither default", []netv1.IngressClass{icUsNotDefault, icOtherNotDefault}, 1),
			Entry("us and another them default", []netv1.IngressClass{icUsNotDefault, icOtherDefault}, 1),
			Entry("us and another us default", []netv1.IngressClass{icUsDefault, icOtherNotDefault}, 2),
			Entry("us and another both default", []netv1.IngressClass{icUsDefault, icOtherDefault}, 2),
		)
	})

	var _ = Describe("ListNgrokModulesV1", func() {
		Context("when there are NgrokModuleSets", func() {
			BeforeEach(func() {
				m1 := NewTestNgrokModuleSet("ngrok", "test", true)
				Expect(store.Add(&m1)).To(BeNil())
				m2 := NewTestNgrokModuleSet("ngrok", "test2", true)
				Expect(store.Add(&m2)).To(BeNil())
				m3 := NewTestNgrokModuleSet("test", "test", true)
				Expect(store.Add(&m3)).To(BeNil())
			})
			It("returns the NgrokModuleSet", func() {
				modules := store.ListNgrokModuleSetsV1()
				Expect(len(modules)).To(Equal(3))
			})
		})
		Context("when there are no NgrokModuleSets", func() {
			It("doesn't error", func() {
				modules := store.ListNgrokModuleSetsV1()
				Expect(len(modules)).To(Equal(0))
			})
		})
	})

	var _ = Describe("GetNgrokModuleSetV1", func() {
		Context("when the NgrokModuleSet exists", func() {
			BeforeEach(func() {
				m := NewTestNgrokModuleSet("ngrok", "test", true)
				Expect(store.Add(&m)).To(BeNil())
			})
			It("returns the NgrokModuleSet", func() {
				modset, err := store.GetNgrokModuleSetV1("ngrok", "test")
				Expect(err).ToNot(HaveOccurred())
				Expect(modset.Modules.Compression.Enabled).To(Equal(true))
			})
		})
		Context("when the NgrokModuleSet does not exist", func() {
			It("returns an error", func() {
				modset, err := store.GetNgrokModuleSetV1("does-not-exist", "does-not-exist")
				Expect(err).To(HaveOccurred())
				Expect(errors.IsErrorNotFound(err)).To(Equal(true))
				Expect(modset).To(BeNil())
			})
		})
	})

	var _ = Describe("GetNgrokTrafficPolicyV1", func() {
		Context("when the NgrokTrafficPolicy exists", func() {
			BeforeEach(func() {
				tp := NewTestNgrokTrafficPolicy("ngrok", "test", "{\"inbound\": \"you know this can be anything though\"}")
				Expect(store.Add(&tp)).To(BeNil())
			})
			It("returns the NgrokTrafficPolicy", func() {
				tp, err := store.GetNgrokTrafficPolicyV1("ngrok", "test")
				Expect(err).ToNot(HaveOccurred())
				Expect(tp.Spec.Policy).To(Equal(json.RawMessage("{\"inbound\": \"you know this can be anything though\"}")))
			})
		})
		Context("when the NgrokTrafficPolicy does not exist", func() {
			It("returns an error", func() {
				tp, err := store.GetNgrokTrafficPolicyV1("does-not-exist", "does-not-exist")
				Expect(err).To(HaveOccurred())
				Expect(errors.IsErrorNotFound(err)).To(Equal(true))
				Expect(tp).To(BeNil())
			})
		})
	})

	var _ = Describe("Issue #56", func() {
		var multiRuleIngress netv1.Ingress

		BeforeEach(func() {
			ngrokClass := NewTestIngressClass(ngrokIngressClass, true, true)
			otherClass := NewTestIngressClass("other", false, false)
			Expect(store.Add(&ngrokClass)).ToNot(HaveOccurred())
			Expect(store.Add(&otherClass)).ToNot(HaveOccurred())
			multiRuleIngress = NewTestIngressV1WithClass("multi-rule-ingress", "test-namespace", ngrokClass.Name)
			multiRuleIngress.Spec.Rules = []netv1.IngressRule{
				{
					Host: "test1.com",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path: "/",
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: "test-service",
											Port: netv1.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
								{
									Path: "/api",
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: "api-service",
											Port: netv1.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Host: "test2.com",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path: "/",
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: "test-service",
											Port: netv1.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			}
		})

		Context("when an ngrok ingress has multiple rules", func() {
			It("should not error", func() {
				Expect(store.Add(&multiRuleIngress)).ToNot(HaveOccurred())
			})

			It("should return the ngrok ingress when queried by name & namespace", func() {
				Expect(store.Add(&multiRuleIngress)).ToNot(HaveOccurred())
				ing, err := store.GetNgrokIngressV1(multiRuleIngress.Name, multiRuleIngress.Namespace)
				Expect(err).ToNot(HaveOccurred())
				Expect(ing).ToNot(BeNil())
				Expect(ing.Spec.Rules).To(HaveLen(2))
			})

			It("should return the ngrok ingress when listed", func() {
				Expect(store.Add(&multiRuleIngress)).ToNot(HaveOccurred())
				ings := store.ListNgrokIngressesV1()
				Expect(ings).To(HaveLen(1))
				Expect(*ings[0]).To(Equal(multiRuleIngress))
			})
		})
	})
})
