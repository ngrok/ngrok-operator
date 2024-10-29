package store

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/util"
)

const defaultManagerName = "ngrok-ingress-controller"

var _ = Describe("Driver", func() {

	var driver *Driver
	var scheme = runtime.NewScheme()
	cname := "cnametarget.com"
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(ingressv1alpha1.AddToScheme(scheme))
	utilruntime.Must(gatewayv1.AddToScheme(scheme))
	utilruntime.Must(ngrokv1alpha1.AddToScheme(scheme))
	BeforeEach(func() {
		// create a fake logger to pass into the cachestore
		logger := logr.New(logr.Discard().GetSink())
		driver = NewDriver(
			logger,
			scheme,
			defaultControllerName,
			types.NamespacedName{Name: defaultManagerName},
			WithGatewayEnabled(false),
			WithSyncAllowConcurrent(true),
		)
	})

	Describe("Seed", func() {
		It("Should not error", func() {
			err := driver.Seed(context.Background(), fake.NewClientBuilder().WithScheme(scheme).Build())
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

			c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(obs...).Build()
			err := driver.Seed(context.Background(), c)
			Expect(err).ToNot(HaveOccurred())

			for _, obj := range obs {
				foundObj, found, err := driver.store.Get(obj)
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
			c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(&i1).Build()
			err := driver.Seed(context.Background(), c)
			Expect(err).ToNot(HaveOccurred())

			err = driver.DeleteNamedIngress(types.NamespacedName{
				Namespace: "test-namespace",
				Name:      "test-ingress",
			})
			Expect(err).ToNot(HaveOccurred())

			foundObj, found, err := driver.store.Get(&i1)
			Expect(err).ToNot(HaveOccurred())
			Expect(found).To(BeFalse())
			Expect(foundObj).To(BeNil())
		})
	})

	Describe("Sync", func() {
		Context("When there are no ingresses in the store", func() {
			It("Should not create anything or error", func() {
				c := fake.NewClientBuilder().WithScheme(scheme).Build()
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
				s := NewTestServiceV1("example", "test-namespace")
				obs := []runtime.Object{&ic1, &ic2, &i1, &i2, &s}
				c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(obs...).Build()

				for _, obj := range obs {
					err := driver.store.Update(obj)
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

				foundEdges := &ingressv1alpha1.HTTPSEdgeList{}
				err = c.List(context.Background(), foundEdges)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(foundEdges.Items)).To(Equal(1))
				foundEdge := foundEdges.Items[0]
				Expect(err).ToNot(HaveOccurred())
				Expect(foundEdge.Spec.Hostports[0]).To(ContainSubstring(i1.Spec.Rules[0].Host))
				Expect(foundEdge.Namespace).To(Equal("test-namespace"))
				Expect(foundEdge.Name).To(HavePrefix("example-com-"))
				Expect(foundEdge.Labels["k8s.ngrok.com/controller-name"]).To(Equal(defaultManagerName))

				foundTunnels := &ingressv1alpha1.TunnelList{}
				err = c.List(context.Background(), foundTunnels)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(foundTunnels.Items)).To(Equal(1))
				foundTunnel := foundTunnels.Items[0]
				Expect(foundTunnel.Namespace).To(Equal("test-namespace"))
				Expect(foundTunnel.Name).To(HavePrefix("example-80-"))
				Expect(foundTunnel.Labels["k8s.ngrok.com/controller-name"]).To(Equal(defaultManagerName))
			})
		})
	})

	Describe("calculateIngressLoadBalancerIPStatus", func() {
		var domains []ingressv1alpha1.Domain
		var ingress netv1.Ingress
		var c client.WithWatch
		var status []netv1.IngressLoadBalancerIngress

		JustBeforeEach(func() {
			c = fake.NewClientBuilder().
				WithLists(
					&ingressv1alpha1.DomainList{
						Items: domains,
					},
				).
				WithScheme(scheme).
				Build()
			status = driver.calculateIngressLoadBalancerIPStatus(&ingress, c)
		})

		addIngressHostname := func(i *netv1.Ingress, hostname string) {
			if i.Spec.Rules == nil {
				i.Spec.Rules = []netv1.IngressRule{}
			}
			i.Spec.Rules = append(i.Spec.Rules, netv1.IngressRule{
				Host: hostname,
			})
		}
		newTestDomain := func(name, domain string, cnameTarget *string) ingressv1alpha1.Domain {
			return ingressv1alpha1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
				Spec: ingressv1alpha1.DomainSpec{
					Domain: domain,
				},
				Status: ingressv1alpha1.DomainStatus{
					Domain:      domain,
					CNAMETarget: cnameTarget,
				},
			}
		}
		newTestDomainList := func(domains ...ingressv1alpha1.Domain) []ingressv1alpha1.Domain {
			return domains
		}

		When("the CNAME is present", func() {
			BeforeEach(func() {
				ingress = NewTestIngressV1("test-ingress", "test-namespace")
				domains = newTestDomainList(
					newTestDomain(
						"example-com",
						"example.com",
						&cname,
					),
				)
			})

			It("should return the CNAME as the status", func() {
				Expect(len(status)).To(Equal(1))
				Expect(status[0].Hostname).To(Equal(cname))
			})
		})

		When("no matching domain is found", func() {
			BeforeEach(func() {
				ingress = NewTestIngressV1("test-ingress", "test-namespace")
				domains = newTestDomainList(
					newTestDomain(
						"another-domain-com",
						"another-domain.com",
						&cname,
					),
				)
			})

			It("should return an empty status", func() {
				Expect(len(status)).To(Equal(0))
			})
		})

		When("the CNAME target is nil and the domain.status.domain is empty", func() {
			BeforeEach(func() {
				ingress = NewTestIngressV1("test-ingress", "test-namespace")
				domains = newTestDomainList(
					ingressv1alpha1.Domain{
						ObjectMeta: metav1.ObjectMeta{
							Name: "example-com",
						},
						Spec: ingressv1alpha1.DomainSpec{
							Domain: "example.com",
						},
						Status: ingressv1alpha1.DomainStatus{},
					},
				)
			})

			It("should return an empty status", func() {
				Expect(len(status)).To(Equal(0))
			})
		})

		When("the domain is a non-wildcard ngrok managed domain", func() {
			BeforeEach(func() {
				ingress = NewTestIngressV1("test-ingress", "test-namespace")
				ingress.Spec = netv1.IngressSpec{
					Rules: []netv1.IngressRule{
						{
							Host: "example.ngrok.io",
						},
					},
				}
				domains = newTestDomainList(
					newTestDomain(
						"example-ngrok-io",
						"example.ngrok.io",
						nil,
					),
				)
			})

			It("should have a status hostname matching the domain", func() {
				Expect(len(status)).To(Equal(1))
				Expect(status[0].Hostname).To(Equal("example.ngrok.io"))
			})
		})

		When("the domain is a wildcard ngrok managed domain", func() {
			BeforeEach(func() {
				ingress = NewTestIngressV1("test-ingress", "test-namespace")
				ingress.Spec = netv1.IngressSpec{
					Rules: []netv1.IngressRule{
						{
							Host: "*.example.ngrok.io",
						},
					},
				}
				domains = newTestDomainList(
					newTestDomain(
						"wildcard-example-ngrok-io",
						"*.example.ngrok.io",
						nil,
					),
				)
			})

			It("should have a .Status[].Hostname equal to the domain without the wildcard", func() {
				Expect(len(status)).To(Equal(1))
				Expect(status[0].Hostname).To(Equal("example.ngrok.io"))
			})
		})

		When("There are multiple domains", func() {
			cname1 := "cnametarget1.com"
			cname2 := "cnametarget2.com"

			BeforeEach(func() {
				ingress = NewTestIngressV1("test-ingress", "test-namespace")
				addIngressHostname(&ingress, "test-domain1.com")
				addIngressHostname(&ingress, "test-domain2.com")
				domains = newTestDomainList(
					newTestDomain(
						"test-domain1-com",
						"test-domain1.com",
						&cname1,
					),
					newTestDomain(
						"test-domain2-com",
						"test-domain2.com",
						&cname2,
					),
				)
			})

			It("should return multiple statuses with those domains", func() {
				Expect(status).Should(ConsistOf(
					HaveField("Hostname", cname1),
					HaveField("Hostname", cname2),
				))
			})
		})

		When("The ingress has multiple duplicate hostnames", func() {
			cname1 := "cnametarget1.com"
			cname2 := "cnametarget2.com"

			BeforeEach(func() {
				ingress = NewTestIngressV1("test-ingress", "test-namespace")
				addIngressHostname(&ingress, "test-domain1.com")
				addIngressHostname(&ingress, "test-domain1.com")
				domains = newTestDomainList(
					newTestDomain(
						"test-domain1-com",
						"test-domain1.com",
						&cname1,
					),
					newTestDomain(
						"test-domain2-com",
						"test-domain2.com",
						&cname2,
					),
				)
			})

			It("should only have a single status the unique domain", func() {
				Expect(status).Should(ConsistOf(
					HaveField("Hostname", cname1),
				))
			})
		})
	})

	Describe("getNgrokModuleSetForIngress", func() {
		var ms1, ms2, ms3 *ingressv1alpha1.NgrokModuleSet

		BeforeEach(func() {
			ms1 = &ingressv1alpha1.NgrokModuleSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ms1", Namespace: "test"},
				Modules: ingressv1alpha1.NgrokModuleSetModules{
					Compression: &ingressv1alpha1.EndpointCompression{
						Enabled: true,
					},
				},
			}
			ms2 = &ingressv1alpha1.NgrokModuleSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ms2", Namespace: "test"},
				Modules: ingressv1alpha1.NgrokModuleSetModules{
					Compression: &ingressv1alpha1.EndpointCompression{
						Enabled: false,
					},
					IPRestriction: &ingressv1alpha1.EndpointIPPolicy{
						IPPolicies: []string{"policy1", "policy2"},
					},
				},
			}
			ms3 = &ingressv1alpha1.NgrokModuleSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ms3", Namespace: "test"},
				Modules: ingressv1alpha1.NgrokModuleSetModules{
					Compression: &ingressv1alpha1.EndpointCompression{
						Enabled: true,
					},
				},
			}
			Expect(driver.store.Add(ms1)).To(BeNil())
			Expect(driver.store.Add(ms2)).To(BeNil())
			Expect(driver.store.Add(ms3)).To(BeNil())
		})

		It("Should return an empty module set if the ingress has no modules annotaion", func() {
			ing := NewTestIngressV1("test-ingress", "test")
			Expect(driver.store.Add(&ing)).To(BeNil())

			ms, err := driver.getNgrokModuleSetForIngress(&ing)
			Expect(err).To(BeNil())
			Expect(ms.Modules.Compression).To(BeNil())
			Expect(ms.Modules.Headers).To(BeNil())
			Expect(ms.Modules.IPRestriction).To(BeNil())
			Expect(ms.Modules.TLSTermination).To(BeNil())
			Expect(ms.Modules.WebhookVerification).To(BeNil())
		})

		It("Should return the matching module set if the ingress has a modules annotaion", func() {
			ing := NewTestIngressV1("test-ingress", "test")
			ing.SetAnnotations(map[string]string{"k8s.ngrok.com/modules": "ms1"})
			Expect(driver.store.Add(&ing)).To(BeNil())

			ms, err := driver.getNgrokModuleSetForIngress(&ing)
			Expect(err).To(BeNil())
			Expect(ms.Modules).To(Equal(ms1.Modules))
		})

		It("merges modules with the last one winning if multiple module sets are specified", func() {
			ing := NewTestIngressV1("test-ingress", "test")
			ing.SetAnnotations(map[string]string{"k8s.ngrok.com/modules": "ms1,ms2,ms3"})
			Expect(driver.store.Add(&ing)).To(BeNil())

			ms, err := driver.getNgrokModuleSetForIngress(&ing)
			Expect(err).To(BeNil())
			Expect(ms.Modules).To(Equal(
				ingressv1alpha1.NgrokModuleSetModules{
					Compression: &ingressv1alpha1.EndpointCompression{
						Enabled: true, // From ms3
					},
					IPRestriction: &ingressv1alpha1.EndpointIPPolicy{
						IPPolicies: []string{"policy1", "policy2"}, // From ms2
					},
				},
			))
		})
	})

	Describe("createEndpointPolicyForGateway", func() {
		var rule *gatewayv1.HTTPRouteRule
		var namespace string
		var policyCrd *ngrokv1alpha1.NgrokTrafficPolicy
		var legacyPolicyCrd *ngrokv1alpha1.NgrokTrafficPolicy

		BeforeEach(func() {
			rule = &gatewayv1.HTTPRouteRule{}
			namespace = "test"

			policyCrd = &ngrokv1alpha1.NgrokTrafficPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-policy",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
					Policy: []byte(`{"on_http_request": [{"name":"t","actions":[{"type":"deny"}]}]}`),
				},
			}
			Expect(driver.store.Add(policyCrd)).To(BeNil())

			legacyPolicyCrd = &ngrokv1alpha1.NgrokTrafficPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "legacy-test-policy",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
					Policy: []byte(`{"inbound": [{"name":"t","actions":[{"type":"deny"}]}], "outbound": []}`),
				},
			}
			Expect(driver.store.Add(legacyPolicyCrd)).To(BeNil())
		})

		It("Should return an empty policy if the rule has nothing in it", func() {
			policy, err := driver.createEndpointPolicyForGateway(rule, namespace)
			Expect(err).To(BeNil())
			Expect(policy).ToNot(BeNil())
		})

		It("Should return a merged policy if there rules with extensionRef", func() {
			hostname := gatewayv1.PreciseHostname("test-hostname.com")
			replacePrefixMatch := "/paprika"

			rule.Filters = []gatewayv1.HTTPRouteFilter{
				{
					Type: "RequestHeaderModifier",
					RequestHeaderModifier: &gatewayv1.HTTPHeaderFilter{
						Add: []gatewayv1.HTTPHeader{
							{
								Name:  "test-header",
								Value: "test-value",
							},
						},
					},
				},
				{
					Type: "ExtensionRef",
					ExtensionRef: &gatewayv1.LocalObjectReference{
						Name:  "test-policy",
						Kind:  "NgrokTrafficPolicy",
						Group: "ngrok.k8s.ngrok.com",
					},
				},
				{
					Type: "URLRewrite",
					URLRewrite: &gatewayv1.HTTPURLRewriteFilter{
						Hostname: &hostname,
						Path: &gatewayv1.HTTPPathModifier{
							Type:               "ReplacePrefixMatch",
							ReplacePrefixMatch: &replacePrefixMatch,
						},
					},
				},
			}

			expectedPolicy := `{"on_http_request":[{"name":"Inbound HTTPRouteRule 1","actions":[{"type":"add-headers","config":{"headers":{"test-header":"test-value"}}}]},{"actions":[{"type":"deny"}],"name":"t"},{"name":"Inbound HTTPRouteRule 2","actions":[{"type":"add-headers","config":{"headers":{"Host":"test-hostname.com"}}}]}]}`

			policy, err := driver.createEndpointPolicyForGateway(rule, namespace)
			Expect(err).To(BeNil())
			Expect(policy).ToNot(BeNil())

			jsonString, err := json.Marshal(policy)
			Expect(err).To(BeNil())
			Expect(string(jsonString)).To(Equal(expectedPolicy))
		})

		It("Should return a merged policy if there rules with extensionRef, legacy policy is remapped", func() {
			hostname := gatewayv1.PreciseHostname("test-hostname.com")
			replacePrefixMatch := "/paprika"

			rule.Filters = []gatewayv1.HTTPRouteFilter{
				{
					Type: "RequestHeaderModifier",
					RequestHeaderModifier: &gatewayv1.HTTPHeaderFilter{
						Add: []gatewayv1.HTTPHeader{
							{
								Name:  "test-header",
								Value: "test-value",
							},
						},
					},
				},
				{
					Type: "ExtensionRef",
					ExtensionRef: &gatewayv1.LocalObjectReference{
						Name:  "legacy-test-policy",
						Kind:  "NgrokTrafficPolicy",
						Group: "ngrok.k8s.ngrok.com",
					},
				},
				{
					Type: "URLRewrite",
					URLRewrite: &gatewayv1.HTTPURLRewriteFilter{
						Hostname: &hostname,
						Path: &gatewayv1.HTTPPathModifier{
							Type:               "ReplacePrefixMatch",
							ReplacePrefixMatch: &replacePrefixMatch,
						},
					},
				},
			}

			expectedPolicy := `{"on_http_request":[{"name":"Inbound HTTPRouteRule 1","actions":[{"type":"add-headers","config":{"headers":{"test-header":"test-value"}}}]},{"actions":[{"type":"deny"}],"name":"t"},{"name":"Inbound HTTPRouteRule 2","actions":[{"type":"add-headers","config":{"headers":{"Host":"test-hostname.com"}}}]}]}`

			policy, err := driver.createEndpointPolicyForGateway(rule, namespace)
			Expect(err).To(BeNil())
			Expect(policy).ToNot(BeNil())

			jsonString, err := json.Marshal(policy)
			Expect(err).To(BeNil())
			Expect(string(jsonString)).To(Equal(expectedPolicy))
		})
	})

	Describe("When not running concurrently", func() {
		It("starts one", func() {
			proceed, wait := driver.syncStart(false)
			Expect(proceed).To(BeTrue())
			Expect(wait).To(BeNil())
			driver.syncDone()
		})

		It("second waits, then returns error", func() {
			firstProceed, _ := driver.syncStart(false)
			Expect(firstProceed).To(BeTrue())

			secondProceed, secondWait := driver.syncStart(false)
			Expect(secondProceed).To(BeFalse())
			Expect(secondWait).To(Not(BeNil()))

			driver.syncDone()

			err := secondWait(context.Background())
			Expect(err).To(Equal(errSyncDone))
		})

		It("third releases second, no error", func() {
			firstProceed, _ := driver.syncStart(false)
			Expect(firstProceed).To(BeTrue())

			secondProceed, secondWait := driver.syncStart(false)
			Expect(secondProceed).To(BeFalse())
			Expect(secondWait).To(Not(BeNil()))

			thirdProceed, thirdWait := driver.syncStart(false)
			Expect(thirdProceed).To(BeFalse())
			Expect(thirdWait).To(Not(BeNil()))

			secondErr := secondWait(context.Background())
			Expect(secondErr).To(BeNil())

			driver.syncDone()

			err := thirdWait(context.Background())
			Expect(err).To(Equal(errSyncDone))
		})

		It("partial third does not wait, no error", func() {
			firstProceed, _ := driver.syncStart(true)
			Expect(firstProceed).To(BeTrue())

			secondProceed, secondWait := driver.syncStart(false)
			Expect(secondProceed).To(BeFalse())
			Expect(secondWait).To(Not(BeNil()))

			thirdProceed, thirdWait := driver.syncStart(true)
			Expect(thirdProceed).To(BeFalse())
			Expect(thirdWait).To(Not(BeNil()))

			thirdErr := thirdWait(context.Background())
			Expect(thirdErr).To(BeNil())

			driver.syncDone()

			err := secondWait(context.Background())
			Expect(err).To(Equal(errSyncDone))
		})
	})
})

func TestExtractPolicy(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                  string
		msg                   json.RawMessage
		expectedTrafficPolicy map[string][]util.RawRule
		expectedErr           error
	}{
		{
			name: "legacy policy configuration",
			msg:  []byte(`{"inbound":[{"name":"test-inbound","actions":[{"type":"deny"}]}],"outbound":[{"name":"test-outbound","actions":[{"type":"some-action"}]}]}`),
			expectedTrafficPolicy: map[string][]util.RawRule{
				util.PhaseOnHttpRequest: {
					[]byte(`{"actions":[{"type":"deny"}],"name":"test-inbound"}`),
				},
				util.PhaseOnHttpResponse: {
					[]byte(`{"actions":[{"type":"some-action"}],"name":"test-outbound"}`),
				},
			},
		},
		{
			name: "phase-based policy config",
			msg:  []byte(`{"on_http_request":[{"name":"test-inbound","actions":[{"type":"deny"}]}],"on_http_response":[{"name":"test-outbound","actions":[{"type":"some-action"}]}]}`),
			expectedTrafficPolicy: map[string][]util.RawRule{
				util.PhaseOnHttpRequest: {
					[]byte(`{"actions":[{"type":"deny"}],"name":"test-inbound"}`),
				},
				util.PhaseOnHttpResponse: {
					[]byte(`{"actions":[{"type":"some-action"}],"name":"test-outbound"}`),
				},
			},
		},
		{
			name:        "invalid json message",
			msg:         []byte(`ngrok operates a global network where it accepts traffic to your upstream services from clients.`),
			expectedErr: fmt.Errorf("invalid character 'g' in literal null (expecting 'u')"),
		},
		{
			name:        "empty json message",
			msg:         []byte(""),
			expectedErr: fmt.Errorf("unexpected end of JSON input"),
		},
		{
			name:        "nil json message",
			expectedErr: fmt.Errorf("unexpected end of JSON input"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			trafficPolicy, err := extractPolicy(tc.msg)

			if tc.expectedTrafficPolicy == nil {
				assert.Nil(t, trafficPolicy)
			} else {
				assert.Equal(t, tc.expectedTrafficPolicy, trafficPolicy.Deconstruct())
			}

			if tc.expectedErr == nil {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				// Can't compare the exact error as we don't have access to json SyntaxError underlying `msg` field`
				assert.Equal(t, tc.expectedErr.Error(), err.Error())
			}
		})
	}
}
