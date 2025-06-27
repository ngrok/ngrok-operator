package store

import (
	"encoding/json"
	"testing"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-operator/internal/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	netv1 "k8s.io/api/networking/v1"

	"github.com/ngrok/ngrok-operator/internal/testutils"
)

const ngrokIngressClass = "ngrok"

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
		store = New(cacheStores, testutils.DefaultControllerName, logger)
	})

	var _ = Describe("GetIngressClassV1", func() {
		Context("when the ingress class exists", func() {
			BeforeEach(func() {
				ic := testutils.NewTestIngressClass(ngrokIngressClass, true, true)
				Expect(store.Add(ic)).To(BeNil())
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
				ing := testutils.NewTestIngressV1("test-ingress", "test-namespace")
				Expect(store.Add(ing)).To(BeNil())
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
				svc := testutils.NewTestServiceV1("test-service", "test-namespace")
				Expect(store.Add(svc)).To(BeNil())
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

	var _ = Describe("GetGateway", func() {
		Context("when the Gateway exists", func() {
			BeforeEach(func() {
				gw := testutils.NewGateway("test-gateway", "test-namespace")
				Expect(store.Add(&gw)).To(BeNil())
			})
			It("returns the Gateway", func() {
				gw, err := store.GetGateway("test-gateway", "test-namespace")
				Expect(err).ToNot(HaveOccurred())
				Expect(gw.Name).To(Equal("test-gateway"))
			})
		})
		Context("when the Gateway does not exist", func() {
			It("returns an error", func() {
				gw, err := store.GetGateway("does-not-exist", "does-not-exist")
				Expect(err).To(HaveOccurred())
				Expect(gw).To(BeNil())
			})
		})
	})

	var _ = Describe("GetHTTPRoute", func() {
		Context("when the HTTPRoute exists", func() {
			BeforeEach(func() {
				r := testutils.NewHTTPRoute("test-httproute", "test-namespace")
				Expect(store.Add(&r)).To(BeNil())
			})
			It("returns the HTTPRoute", func() {
				r, err := store.GetHTTPRoute("test-httproute", "test-namespace")
				Expect(err).ToNot(HaveOccurred())
				Expect(r.Name).To(Equal("test-httproute"))
			})
		})
		Context("when the HTTPRoute does not exist", func() {
			It("returns an error", func() {
				r, err := store.GetHTTPRoute("does-not-exist", "does-not-exist")
				Expect(err).To(HaveOccurred())
				Expect(r).To(BeNil())
			})
		})
	})

	var _ = Describe("GetTLSRoute", func() {
		Context("when the TLSRoute exists", func() {
			BeforeEach(func() {
				r := testutils.NewTLSRoute("test-tlsroute", "test-namespace")
				Expect(store.Add(&r)).To(BeNil())
			})
			It("returns the TLSRoute", func() {
				r, err := store.GetTLSRoute("test-tlsroute", "test-namespace")
				Expect(err).ToNot(HaveOccurred())
				Expect(r.Name).To(Equal("test-tlsroute"))
			})
		})
		Context("when the TLSRoute does not exist", func() {
			It("returns an error", func() {
				r, err := store.GetTLSRoute("does-not-exist", "does-not-exist")
				Expect(err).To(HaveOccurred())
				Expect(r).To(BeNil())
			})
		})
	})

	var _ = Describe("GetTCPRoute", func() {
		Context("when the TCPRoute exists", func() {
			BeforeEach(func() {
				r := testutils.NewTCPRoute("test-tcproute", "test-namespace")
				Expect(store.Add(&r)).To(BeNil())
			})
			It("returns the TCPRoute", func() {
				r, err := store.GetTCPRoute("test-tcproute", "test-namespace")
				Expect(err).ToNot(HaveOccurred())
				Expect(r.Name).To(Equal("test-tcproute"))
			})
		})
		Context("when the TCPRoute does not exist", func() {
			It("returns an error", func() {
				r, err := store.GetTCPRoute("does-not-exist", "does-not-exist")
				Expect(err).To(HaveOccurred())
				Expect(r).To(BeNil())
			})
		})
	})

	var _ = Describe("GetNgrokIngressV1", func() {
		Context("when the ngrok ingress exists", func() {
			BeforeEach(func() {
				ing := testutils.NewTestIngressV1WithClass("test-ingress", "test-namespace", ngrokIngressClass)
				Expect(store.Add(ing)).To(BeNil())
				ic := testutils.NewTestIngressClass(ngrokIngressClass, true, true)
				Expect(store.Add(ic)).To(BeNil())
			})
			It("returns the ngrok ingress", func() {
				ing, err := store.GetNgrokIngressV1("test-ingress", "test-namespace")
				Expect(err).ToNot(HaveOccurred())
				Expect(ing.Name).To(Equal("test-ingress"))
			})
			It("Filters out ingresses that don't match the ngrok ingress class", func() {
				ingNotNgrok := testutils.NewTestIngressV1WithClass("ingNotNgrok", "test-namespace", "not-ngrok")
				Expect(store.Add(ingNotNgrok)).To(BeNil())

				ing, err := store.GetNgrokIngressV1("ingNotNgrok", "test-namespace")
				Expect(err).To(HaveOccurred())
				Expect(ing).To(BeNil())
			})
			It("Filters finds ones without a class if we are default", func() {
				ingNoClass := testutils.NewTestIngressV1("ingNoClass", "test-namespace")
				Expect(store.Add(ingNoClass)).To(BeNil())

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
				ic1 := testutils.NewTestIngressClass("ngrok1", true, true)
				Expect(store.Add(ic1)).To(BeNil())
				ic2 := testutils.NewTestIngressClass("ngrok2", true, true)
				Expect(store.Add(ic2)).To(BeNil())
				ic3 := testutils.NewTestIngressClass("different", true, false)
				Expect(store.Add(ic3)).To(BeNil())
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

	var _ = Describe("ListGateways", func() {
		Context("when there are Gateways", func() {
			BeforeEach(func() {
				gw1 := testutils.NewGateway("gateway-1", "test-namespace")
				Expect(store.Add(&gw1)).To(BeNil())
				gw2 := testutils.NewGateway("gateway-2", "test-namespace")
				Expect(store.Add(&gw2)).To(BeNil())
				gw3 := testutils.NewGateway("gateway-3", "test-namespace")
				Expect(store.Add(&gw3)).To(BeNil())
			})
			It("returns the Gateways", func() {
				expectedNames := []string{
					"gateway-1",
					"gateway-2",
					"gateway-3",
				}
				gws := store.ListGateways()
				Expect(len(gws)).To(Equal(3))

				for _, expectedName := range expectedNames {
					found := false
					for _, gw := range gws {
						if gw.Name == expectedName {
							found = true
							break
						}
					}
					Expect(found, true)
				}
			})
		})
		Context("when there are no Gateways", func() {
			It("doesn't error", func() {
				gws := store.ListGateways()
				Expect(len(gws)).To(Equal(0))
			})
		})
	})

	var _ = Describe("ListHTTPRoutes", func() {
		Context("when there are HTTPRoutes", func() {
			BeforeEach(func() {
				r1 := testutils.NewHTTPRoute("httproute-1", "test-namespace")
				Expect(store.Add(&r1)).To(BeNil())
				r2 := testutils.NewHTTPRoute("httproute-2", "test-namespace")
				Expect(store.Add(&r2)).To(BeNil())
				r3 := testutils.NewHTTPRoute("httproute-3", "test-namespace")
				Expect(store.Add(&r3)).To(BeNil())
			})
			It("returns the HTTPRoutes", func() {
				expectedNames := []string{
					"httproute-1",
					"httproute-2",
					"httproute-3",
				}
				rs := store.ListHTTPRoutes()
				Expect(len(rs)).To(Equal(3))

				for _, expectedName := range expectedNames {
					found := false
					for _, r := range rs {
						if r.Name == expectedName {
							found = true
							break
						}
					}
					Expect(found, true)
				}
			})
		})
		Context("when there are no HTTPRoutes", func() {
			It("doesn't error", func() {
				rs := store.ListHTTPRoutes()
				Expect(len(rs)).To(Equal(0))
			})
		})
	})

	var _ = Describe("ListTCPRoutes", func() {
		Context("when there are TCPRoutes", func() {
			BeforeEach(func() {
				r1 := testutils.NewTCPRoute("tcproute-1", "test-namespace")
				Expect(store.Add(&r1)).To(BeNil())
				r2 := testutils.NewTCPRoute("tcproute-2", "test-namespace")
				Expect(store.Add(&r2)).To(BeNil())
				r3 := testutils.NewTCPRoute("tcproute-3", "test-namespace")
				Expect(store.Add(&r3)).To(BeNil())
			})
			It("returns the TCPRoutes", func() {
				expectedNames := []string{
					"tcproute-1",
					"tcproute-2",
					"tcproute-3",
				}
				rs := store.ListTCPRoutes()
				Expect(len(rs)).To(Equal(3))

				for _, expectedName := range expectedNames {
					found := false
					for _, r := range rs {
						if r.Name == expectedName {
							found = true
							break
						}
					}
					Expect(found, true)
				}
			})
		})
		Context("when there are no TCPRoutes", func() {
			It("doesn't error", func() {
				rs := store.ListTCPRoutes()
				Expect(len(rs)).To(Equal(0))
			})
		})
	})

	var _ = Describe("ListTLSRoutes", func() {
		Context("when there are TLSRoutes", func() {
			BeforeEach(func() {
				r1 := testutils.NewTLSRoute("tlsroute-1", "test-namespace")
				Expect(store.Add(&r1)).To(BeNil())
				r2 := testutils.NewTLSRoute("tlsroute-2", "test-namespace")
				Expect(store.Add(&r2)).To(BeNil())
				r3 := testutils.NewTLSRoute("tlsroute-3", "test-namespace")
				Expect(store.Add(&r3)).To(BeNil())
			})
			It("returns the TLSRoutes", func() {
				expectedNames := []string{
					"tlsroute-1",
					"tlsroute-2",
					"tlsroute-3",
				}
				rs := store.ListTLSRoutes()
				Expect(len(rs)).To(Equal(3))

				for _, expectedName := range expectedNames {
					found := false
					for _, r := range rs {
						if r.Name == expectedName {
							found = true
							break
						}
					}
					Expect(found, true)
				}
			})
		})
		Context("when there are no TLSRoutes", func() {
			It("doesn't error", func() {
				rs := store.ListTLSRoutes()
				Expect(len(rs)).To(Equal(0))
			})
		})
	})

	var _ = Describe("ListReferenceGrants", func() {
		Context("when there are ReferenceGrants", func() {
			BeforeEach(func() {
				r1 := testutils.NewReferenceGrant("grant-1", "test-namespace")
				Expect(store.Add(&r1)).To(BeNil())
				r2 := testutils.NewReferenceGrant("grant-2", "test-namespace")
				Expect(store.Add(&r2)).To(BeNil())
				r3 := testutils.NewReferenceGrant("grant-3", "test-namespace")
				Expect(store.Add(&r3)).To(BeNil())
			})
			It("returns the ReferenceGrants", func() {
				expectedNames := []string{
					"grant-1",
					"grant-2",
					"grant-3",
				}
				rs := store.ListReferenceGrants()
				Expect(len(rs)).To(Equal(3))

				for _, expectedName := range expectedNames {
					found := false
					for _, r := range rs {
						if r.Name == expectedName {
							found = true
							break
						}
					}
					Expect(found, true)
				}
			})
		})
		Context("when there are no ReferenceGrants", func() {
			It("doesn't error", func() {
				rs := store.ListReferenceGrants()
				Expect(len(rs)).To(Equal(0))
			})
		})
	})

	var _ = Describe("ListNgrokIngressesV1", func() {
		icUsDefault := testutils.NewTestIngressClass("ngrok", true, true)
		icUsNotDefault := testutils.NewTestIngressClass("ngrok", false, true)
		icOtherDefault := testutils.NewTestIngressClass("test", true, false)
		icOtherNotDefault := testutils.NewTestIngressClass("test", false, false)

		var _ = DescribeTable("IngressClassFiltering", func(ingressClasses []*netv1.IngressClass, expectedMatchingIngressesCount int) {
			iMatching := testutils.NewTestIngressV1WithClass("test1", "test", "ngrok")
			iNotMatching := testutils.NewTestIngressV1WithClass("test2", "test", "test")
			iNoClass := testutils.NewTestIngressV1("test3", "test")
			Expect(store.Add(iMatching)).To(BeNil())
			Expect(store.Add(iNotMatching)).To(BeNil())
			Expect(store.Add(iNoClass)).To(BeNil())
			for _, ic := range ingressClasses {
				Expect(store.Add(ic)).To(BeNil())
			}
			ings := store.ListNgrokIngressesV1()
			Expect(len(ings)).To(Equal(expectedMatchingIngressesCount))
		},
			Entry("No ingress classes", []*netv1.IngressClass{}, 0),
			Entry("just us not as default", []*netv1.IngressClass{icUsNotDefault}, 1),
			Entry("just us as default", []*netv1.IngressClass{icUsDefault}, 2),
			Entry("just another not as default", []*netv1.IngressClass{icOtherNotDefault}, 0),
			Entry("just another as default", []*netv1.IngressClass{icOtherDefault}, 0),
			Entry("us and another neither default", []*netv1.IngressClass{icUsNotDefault, icOtherNotDefault}, 1),
			Entry("us and another them default", []*netv1.IngressClass{icUsNotDefault, icOtherDefault}, 1),
			Entry("us and another us default", []*netv1.IngressClass{icUsDefault, icOtherNotDefault}, 2),
			Entry("us and another both default", []*netv1.IngressClass{icUsDefault, icOtherDefault}, 2),
		)
	})

	var _ = Describe("GetNgrokTrafficPolicyV1", func() {
		Context("when the NgrokTrafficPolicy exists", func() {
			BeforeEach(func() {
				tp := testutils.NewTestNgrokTrafficPolicy("ngrok", "test", "{\"inbound\": \"you know this can be anything though\"}")
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
		var multiRuleIngress *netv1.Ingress

		BeforeEach(func() {
			ngrokClass := testutils.NewTestIngressClass(ngrokIngressClass, true, true)
			otherClass := testutils.NewTestIngressClass("other", false, false)
			Expect(store.Add(ngrokClass)).ToNot(HaveOccurred())
			Expect(store.Add(otherClass)).ToNot(HaveOccurred())
			multiRuleIngress = testutils.NewTestIngressV1WithClass("multi-rule-ingress", "test-namespace", ngrokClass.Name)
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
				Expect(store.Add(multiRuleIngress)).ToNot(HaveOccurred())
			})

			It("should return the ngrok ingress when queried by name & namespace", func() {
				Expect(store.Add(multiRuleIngress)).ToNot(HaveOccurred())
				ing, err := store.GetNgrokIngressV1(multiRuleIngress.Name, multiRuleIngress.Namespace)
				Expect(err).ToNot(HaveOccurred())
				Expect(ing).ToNot(BeNil())
				Expect(ing.Spec.Rules).To(HaveLen(2))
			})

			It("should return the ngrok ingress when listed", func() {
				Expect(store.Add(multiRuleIngress)).ToNot(HaveOccurred())
				ings := store.ListNgrokIngressesV1()
				Expect(ings).To(HaveLen(1))
				Expect(ings[0]).To(Equal(multiRuleIngress))
			})
		})
	})

	var _ = Describe("Store Validation", func() {
		var store Store
		var logger logr.Logger

		BeforeEach(func() {
			// Setup the Store directly instead of through the Storer interface
			logger = logr.New(logr.Discard().GetSink())
			cacheStores := NewCacheStores(logger)
			store = Store{
				stores:         cacheStores,
				controllerName: testutils.DefaultControllerName,
				log:            logger,
			}
			ngrokClass := testutils.NewTestIngressClass("ngrok", true, true)
			Expect(store.Add(ngrokClass)).To(BeNil())
		})

		Context("when ingress has missing HTTP rules", func() {
			It("returns an error without crashing", func() {
				ing := testutils.NewTestIngressV1("ingress-no-rules", "test-namespace")
				ing.Spec.Rules = []netv1.IngressRule{{
					Host: "test.com",
				}}
				ok, err := store.shouldHandleIngressIsValid(ing)
				Expect(ok).To(BeFalse())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("HTTP rules are required for ingress"))
			})
		})

		Context("when ingress has unsupported default backend", func() {
			It("ignores the ingress with default backend and returns an error with mapping-strategy: edges", func() {
				ing := testutils.NewTestIngressV1("ingress-default-backend", "test-namespace")
				if ing.Annotations == nil {
					ing.Annotations = map[string]string{}
				}
				ing.Annotations["k8s.ngrok.com/mapping-strategy"] = "edges"
				ing.Spec.DefaultBackend = &netv1.IngressBackend{
					Service: &netv1.IngressServiceBackend{
						Name: "default-service",
						Port: netv1.ServiceBackendPort{Number: 80},
					},
				}
				ok, err := store.shouldHandleIngressIsValid(ing)
				Expect(ok).To(BeTrue())
				Expect(err).To(Not(HaveOccurred()))
			})
		})

		Context("when ingress rule is missing hostname", func() {
			It("flags the ingress as invalid", func() {
				ing := testutils.NewTestIngressV1("ingress-no-host", "test-namespace")
				ing.Spec.Rules = []netv1.IngressRule{
					{
						Host: "a-hostname.com",
						IngressRuleValue: netv1.IngressRuleValue{
							HTTP: &netv1.HTTPIngressRuleValue{
								Paths: []netv1.HTTPIngressPath{
									{
										Path: "/",
										Backend: netv1.IngressBackend{
											Service: &netv1.IngressServiceBackend{
												Name: "test-service",
												Port: netv1.ServiceBackendPort{Number: 80},
											},
										},
									},
								},
							},
						},
					},
					{
						Host: "", // Missing hostname
						IngressRuleValue: netv1.IngressRuleValue{
							HTTP: &netv1.HTTPIngressRuleValue{
								Paths: []netv1.HTTPIngressPath{
									{
										Path: "/",
										Backend: netv1.IngressBackend{
											Service: &netv1.IngressServiceBackend{
												Name: "test-service",
												Port: netv1.ServiceBackendPort{Number: 80},
											},
										},
									},
								},
							},
						},
					},
				}
				ok, err := store.shouldHandleIngressIsValid(ing)
				Expect(ok).To(BeFalse())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("A host is required to be set"))
			})
		})

		Context("when ingress uses deprecated ingress annotation", func() {
			It("logs a warning about the deprecated annotation", func() {
				ing := testutils.NewTestIngressV1("ingress-deprecated-annotation", "test-namespace")
				ingressClassName := "not-ngrok"
				ing.Spec.IngressClassName = &ingressClassName
				ing.Annotations = map[string]string{
					"kubernetes.io/ingress.class": "ngrok",
				}
				ok, err := store.shouldHandleIngress(ing)
				Expect(ok).To(BeFalse())
				Expect(err).ToNot(BeNil())
			})
		})

		Context("when ingress class does not match", func() {
			It("returns an error message showing the ingress class name", func() {
				ing := testutils.NewTestIngressV1WithClass("ingress-wrong-class", "test-namespace", "not-ngrok")
				ok, err := store.shouldHandleIngressCheckClass(ing)
				Expect(ok).To(BeFalse())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("ingress class mismatching"))
				Expect(err.Error()).To(ContainSubstring("not-ngrok"))
			})
		})

		Context("when no matching ingress classes are configured", func() {
			It("lists known ingress classes in the error message", func() {
				ing := testutils.NewTestIngressV1WithClass("ingress-no-match", "test-namespace", "no-match-class")
				ok, err := store.shouldHandleIngressCheckClass(ing)
				Expect(ok).To(BeFalse())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no-match-class"))
				Expect(err.Error()).To(ContainSubstring("ngrok"))
			})
		})

		Context("when configured ingress class cannot be found", func() {
			BeforeEach(func() {
				// Delete the ngrok ingress class to simulate missing configuration
				ngrokClass := testutils.NewTestIngressClass("ngrok", true, true)
				Expect(store.Delete(ngrokClass)).To(BeNil())
			})

			It("emits a warning or event about the missing class", func() {
				ing := testutils.NewTestIngressV1WithClass("ingress-missing-class", "test-namespace", "ngrok")
				ok, err := store.shouldHandleIngressCheckClass(ing)
				Expect(ok).To(BeFalse())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no default ingress class found"))
			})
		})
	})
})
