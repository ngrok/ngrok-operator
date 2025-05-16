package managerdriver

import (
	"encoding/json"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	common "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/testutils"
	"github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	"github.com/ngrok/ngrok-operator/internal/util"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const defaultManagerName = "ngrok-ingress-controller"

var _ = Describe("Driver", func() {

	var driver *Driver
	var scheme = runtime.NewScheme()
	cname := "cnametarget.com"
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(ingressv1alpha1.AddToScheme(scheme))
	utilruntime.Must(gatewayv1.Install(scheme))
	utilruntime.Must(gatewayv1alpha2.Install(scheme))
	utilruntime.Must(ngrokv1alpha1.AddToScheme(scheme))

	BeforeEach(func() {
		driver = NewDriver(
			GinkgoLogr,
			scheme,
			testutils.DefaultControllerName,
			types.NamespacedName{Name: defaultManagerName},
			WithGatewayEnabled(false),
			WithSyncAllowConcurrent(true),
		)
	})

	Describe("Seed", func() {
		It("Should not error", func() {
			err := driver.Seed(GinkgoT().Context(), fake.NewClientBuilder().WithScheme(scheme).Build())
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should add all the found items to the store", func() {
			i1 := testutils.NewTestIngressV1("test-ingress", "test-namespace")
			i2 := testutils.NewTestIngressV1("test-ingress-2", "test-namespace")
			ic1 := testutils.NewTestIngressClass("test-ingress-class", true, true)
			ic2 := testutils.NewTestIngressClass("test-ingress-class-2", true, true)
			d1 := testutils.NewDomainV1("test-domain.com", "test-namespace")
			d2 := testutils.NewDomainV1("test-domain-2.com", "test-namespace")
			e1 := testutils.NewHTTPSEdge("test-edge", "test-namespace")
			e2 := testutils.NewHTTPSEdge("test-edge-2", "test-namespace")
			obs := []runtime.Object{ic1, ic2, i1, i2, d1, d2, &e1, &e2}

			c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(obs...).Build()
			err := driver.Seed(GinkgoT().Context(), c)
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
			i1 := testutils.NewTestIngressV1("test-ingress", "test-namespace")
			c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(i1).Build()
			err := driver.Seed(GinkgoT().Context(), c)
			Expect(err).ToNot(HaveOccurred())

			err = driver.DeleteNamedIngress(types.NamespacedName{
				Namespace: "test-namespace",
				Name:      "test-ingress",
			})
			Expect(err).ToNot(HaveOccurred())

			foundObj, found, err := driver.store.Get(i1)
			Expect(err).ToNot(HaveOccurred())
			Expect(found).To(BeFalse())
			Expect(foundObj).To(BeNil())
		})
	})

	Describe("Sync", func() {
		Context("When there are no ingresses in the store", func() {
			It("Should not create anything or error", func() {
				c := fake.NewClientBuilder().WithScheme(scheme).Build()
				err := driver.Sync(GinkgoT().Context(), c)
				Expect(err).ToNot(HaveOccurred())

				domains := &ingressv1alpha1.DomainList{}
				err = c.List(GinkgoT().Context(), &ingressv1alpha1.DomainList{})
				Expect(err).ToNot(HaveOccurred())
				Expect(domains.Items).To(HaveLen(0))

				edges := &ingressv1alpha1.HTTPSEdgeList{}
				err = c.List(GinkgoT().Context(), &ingressv1alpha1.HTTPSEdgeList{})
				Expect(err).ToNot(HaveOccurred())
				Expect(edges.Items).To(HaveLen(0))

				tunnels := &ingressv1alpha1.TunnelList{}
				err = c.List(GinkgoT().Context(), &ingressv1alpha1.TunnelList{})
				Expect(err).ToNot(HaveOccurred())
				Expect(tunnels.Items).To(HaveLen(0))
			})
		})
		Context("When there are just edge ingresses and edges need to be created", func() {
			It("Should create the CRDs", func() {
				i1 := testutils.NewTestIngressV1("test-ingress", "test-namespace")
				if i1.Annotations == nil {
					i1.Annotations = map[string]string{}
				}
				i1.Annotations["k8s.ngrok.com/mapping-strategy"] = "edges"
				i2 := testutils.NewTestIngressV1("test-ingress-2", "test-namespace")
				if i2.Annotations == nil {
					i2.Annotations = map[string]string{}
				}
				i2.Annotations["k8s.ngrok.com/mapping-strategy"] = "edges"

				ic1 := testutils.NewTestIngressClass("test-ingress-class", true, true)
				ic2 := testutils.NewTestIngressClass("test-ingress-class-2", true, true)
				s := testutils.NewTestServiceV1("example", "test-namespace")
				obs := []runtime.Object{ic1, ic2, i1, i2, s}
				c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(obs...).Build()

				for _, obj := range obs {
					err := driver.store.Update(obj)
					Expect(err).ToNot(HaveOccurred())
				}
				err := driver.Seed(GinkgoT().Context(), c)
				Expect(err).ToNot(HaveOccurred())

				err = driver.Sync(GinkgoT().Context(), c)
				Expect(err).ToNot(HaveOccurred())

				foundDomain := &ingressv1alpha1.Domain{}
				err = c.Get(GinkgoT().Context(), types.NamespacedName{
					Namespace: "test-namespace",
					Name:      "example-com",
				}, foundDomain)
				Expect(err).ToNot(HaveOccurred())
				Expect(foundDomain.Spec.Domain).To(Equal(i1.Spec.Rules[0].Host))

				foundEdges := &ingressv1alpha1.HTTPSEdgeList{}
				err = c.List(GinkgoT().Context(), foundEdges)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(foundEdges.Items)).To(Equal(1))
				foundEdge := foundEdges.Items[0]
				Expect(err).ToNot(HaveOccurred())
				Expect(foundEdge.Spec.Hostports[0]).To(ContainSubstring(i1.Spec.Rules[0].Host))
				Expect(foundEdge.Namespace).To(Equal("test-namespace"))
				Expect(foundEdge.Name).To(HavePrefix("example-com-"))
				Expect(foundEdge.Labels["k8s.ngrok.com/controller-name"]).To(Equal(defaultManagerName))

				foundTunnels := &ingressv1alpha1.TunnelList{}
				err = c.List(GinkgoT().Context(), foundTunnels)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(foundTunnels.Items)).To(Equal(1))
				foundTunnel := foundTunnels.Items[0]
				Expect(foundTunnel.Namespace).To(Equal("test-namespace"))
				Expect(foundTunnel.Name).To(HavePrefix("example-80-"))
				Expect(foundTunnel.Labels["k8s.ngrok.com/controller-name"]).To(Equal(defaultManagerName))
			})
		})

		When("A service specifies an appProtocol", func() {
			var (
				httpService             *v1.Service
				httpsService            *v1.Service
				ingress                 *netv1.Ingress
				c                       client.WithWatch
				namespace               = "app-proto-namespace"
				foundTunnels            *ingressv1alpha1.TunnelList
				ic                      = testutils.NewTestIngressClass("app-proto-ingress-class", true, true)
				setIngressTargetService = func(i *netv1.Ingress, s *v1.Service) {
					// Modify the ingress to include the service
					i.Spec.Rules = []netv1.IngressRule{
						{
							Host: "foo.ngrok.io",
							IngressRuleValue: netv1.IngressRuleValue{
								HTTP: &netv1.HTTPIngressRuleValue{
									Paths: []netv1.HTTPIngressPath{
										{
											Path:     "/",
											PathType: ptr.To(netv1.PathTypePrefix),
											Backend: netv1.IngressBackend{
												Service: &netv1.IngressServiceBackend{
													Name: s.Name,
													Port: netv1.ServiceBackendPort{
														Name: s.Spec.Ports[0].Name,
													},
												},
											},
										},
									},
								},
							},
						},
					}
				}
			)

			BeforeEach(func() {
				httpService = &v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "http-service",
						Namespace: namespace,
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{
							{
								Port:       80,
								Name:       "http",
								TargetPort: intstr.FromInt(80),
							},
						},
					},
				}
				httpsService = &v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "https-service",
						Namespace: namespace,
						Annotations: map[string]string{
							"k8s.ngrok.com/app-protocols": `{"https": "https"}`,
						},
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{
							{
								Port:       443,
								Name:       "https",
								TargetPort: intstr.FromInt(443),
							},
						},
					},
				}
				ingress = &netv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "test-ingress",
						Namespace:   namespace,
						Annotations: map[string]string{"k8s.ngrok.com/mapping-strategy": "edges"},
					},
					Spec: netv1.IngressSpec{
						IngressClassName: &ic.Name,
						Rules:            []netv1.IngressRule{},
					},
				}
			})

			JustBeforeEach(func() {
				// Add the services and ingress to the fake client and the store
				objs := []runtime.Object{ic, httpService, httpsService, ingress}
				c = fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

				for _, obj := range objs {
					Expect(driver.store.Update(obj)).To(BeNil())
				}

				// Seed & Sync
				Expect(driver.Seed(GinkgoT().Context(), c)).To(BeNil())
				Expect(driver.Sync(GinkgoT().Context(), c)).To(BeNil())

				// Find the tunnels in this namespace
				foundTunnels = &ingressv1alpha1.TunnelList{}
				err := c.List(GinkgoT().Context(), foundTunnels, client.InNamespace(namespace))
				Expect(err).ToNot(HaveOccurred())
			})

			When("The appProtocol is unknown", func() {
				BeforeEach(func() {
					// Set an unknown appProtocol on the httpService
					httpService.Spec.Ports[0].AppProtocol = ptr.To("unknown")
					// Modify the ingress to include the httpService
					setIngressTargetService(ingress, httpService)
				})

				It("Should ignore the unknown appProtocol", func() {
					// We expect one tunnel to be created
					Expect(len(foundTunnels.Items)).To(Equal(1))

					By("Creating a tunnel with no appProtocol and the correct upstream")
					foundTunnel := foundTunnels.Items[0]
					Expect(foundTunnel.Spec.AppProtocol).To(BeNil())
					Expect(foundTunnel.Spec.ForwardsTo).To(Equal("http-service.app-proto-namespace.svc.cluster.local:80"))
					Expect(foundTunnel.Spec.BackendConfig.Protocol).To(Equal("HTTP"))
				})
			})

			When("The appProtocol is http", func() {
				BeforeEach(func() {
					// Set the appProtocol on the httpService
					httpService.Spec.Ports[0].AppProtocol = ptr.To("http")
					// Modify the ingress to include the httpService
					setIngressTargetService(ingress, httpService)
				})

				It("Should create a tunnel with appProtocol http1", func() {
					// We expect one tunnel to be created
					Expect(len(foundTunnels.Items)).To(Equal(1))

					By("Creating a tunnel with appProtocol http1")
					foundTunnel := foundTunnels.Items[0]
					Expect(foundTunnel.Spec.AppProtocol).To(Equal(ptr.To(common.ApplicationProtocol_HTTP1)))
					Expect(foundTunnel.Spec.ForwardsTo).To(Equal("http-service.app-proto-namespace.svc.cluster.local:80"))
					Expect(foundTunnel.Spec.BackendConfig.Protocol).To(Equal("HTTP"))
				})
			})

			When("The appProtocol is k8s.ngrok.com/http2", func() {
				BeforeEach(func() {
					// Set the appProtocol on the httpService
					httpsService.Spec.Ports[0].AppProtocol = ptr.To("k8s.ngrok.com/http2")

					// Modify the ingress to include the httpsService
					setIngressTargetService(ingress, httpsService)
				})

				It("Should create a tunnel with appProtocol http2", func() {
					// We expect one tunnel to be created
					Expect(len(foundTunnels.Items)).To(Equal(1))

					By("Creating a tunnel with appProtocol http2")
					foundTunnel := foundTunnels.Items[0]
					Expect(foundTunnel.Spec.AppProtocol).To(Equal(ptr.To(common.ApplicationProtocol_HTTP2)))
					Expect(foundTunnel.Spec.ForwardsTo).To(Equal("https-service.app-proto-namespace.svc.cluster.local:443"))
					Expect(foundTunnel.Spec.BackendConfig.Protocol).To(Equal("HTTPS"))
				})
			})

			When("The appProtocol is kubernetes.io/h2c", func() {
				BeforeEach(func() {
					// Set the appProtocol on the httpService
					httpsService.Spec.Ports[0].AppProtocol = ptr.To("kubernetes.io/h2c")

					// Modify the ingress to include the httpsService
					setIngressTargetService(ingress, httpsService)
				})

				It("Should create a tunnel with appProtocol http2", func() {
					// We expect one tunnel to be created
					Expect(len(foundTunnels.Items)).To(Equal(1))

					By("Creating a tunnel with appProtocol http2")
					foundTunnel := foundTunnels.Items[0]
					Expect(foundTunnel.Spec.AppProtocol).To(Equal(ptr.To(common.ApplicationProtocol_HTTP2)))
					Expect(foundTunnel.Spec.ForwardsTo).To(Equal("https-service.app-proto-namespace.svc.cluster.local:443"))
					Expect(foundTunnel.Spec.BackendConfig.Protocol).To(Equal("HTTPS"))
				})
			})

			When("The Ingress does not specify mapping-strategy: edges", func() {
				BeforeEach(func() {
					ingress.Annotations = map[string]string{} // Removing the edges annotation should cause the translation to be endpoints instead.
				})

				It("Should not have any tunnels", func() {
					Expect(len(foundTunnels.Items)).To(Equal(0))
				})
			})
		})

		When("An ingress specifies a traffic policy", func() {
			var (
				c                   client.WithWatch
				namespace           = "edge-tp-test-namespace"
				httpService         *v1.Service
				ingress             *netv1.Ingress
				trafficPolicy       *ngrokv1alpha1.NgrokTrafficPolicy
				foundHTTPEdges      *ingressv1alpha1.HTTPSEdgeList
				foundAgentEndpoints *ngrokv1alpha1.AgentEndpointList
				foundCloudEndpoints *ngrokv1alpha1.CloudEndpointList
				ic                  = testutils.NewTestIngressClass("edge-tp-ingress-class", true, true)
			)

			BeforeEach(func() {
				pol := trafficpolicy.NewTrafficPolicy()
				pol.AddRuleOnHTTPRequest(trafficpolicy.Rule{
					Name: "test-name",
					Actions: []trafficpolicy.Action{
						trafficpolicy.NewCompressResponseAction(nil),
					},
				})
				rawPolicy, err := json.Marshal(pol)
				Expect(err).ToNot(HaveOccurred())

				trafficPolicy = &ngrokv1alpha1.NgrokTrafficPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-policy",
						Namespace: namespace,
					},
					Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{

						Policy: rawPolicy,
					},
				}
				httpService = &v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "http-service",
						Namespace: namespace,
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{
							{
								Port:       80,
								Name:       "http",
								TargetPort: intstr.FromInt(80),
							},
						},
					},
				}
				ingress = &netv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ingress",
						Namespace: namespace,
					},
					Spec: netv1.IngressSpec{
						IngressClassName: &ic.Name,
						Rules: []netv1.IngressRule{
							{
								Host: "foo.ngrok.io",
								IngressRuleValue: netv1.IngressRuleValue{
									HTTP: &netv1.HTTPIngressRuleValue{
										Paths: []netv1.HTTPIngressPath{
											{
												Path:     "/",
												PathType: ptr.To(netv1.PathTypePrefix),
												Backend: netv1.IngressBackend{
													Service: &netv1.IngressServiceBackend{
														Name: httpService.Name,
														Port: netv1.ServiceBackendPort{
															Name: httpService.Spec.Ports[0].Name,
														},
													},
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

			JustBeforeEach(func() {
				// Add the services and ingress to the fake client and the store
				objs := []runtime.Object{ic, trafficPolicy, httpService, ingress}
				c = fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

				for _, obj := range objs {
					Expect(driver.store.Update(obj)).To(BeNil())
				}

				// Seed & Sync
				Expect(driver.Seed(GinkgoT().Context(), c)).To(Succeed())
				Expect(driver.Sync(GinkgoT().Context(), c)).To(Succeed())

				// Find the HTTPSEdges in this namespace
				foundHTTPEdges = &ingressv1alpha1.HTTPSEdgeList{}
				Expect(c.List(GinkgoT().Context(), foundHTTPEdges, client.InNamespace(namespace))).To(Succeed())

				// Find the AgentEndpoints in this namespace
				foundAgentEndpoints = &ngrokv1alpha1.AgentEndpointList{}
				Expect(c.List(GinkgoT().Context(), foundAgentEndpoints, client.InNamespace(namespace))).To(Succeed())

				// Find the CloudEndpoints in this namespace
				foundCloudEndpoints = &ngrokv1alpha1.CloudEndpointList{}
				Expect(c.List(GinkgoT().Context(), foundCloudEndpoints, client.InNamespace(namespace))).To(Succeed())
			})

			When("The the ingress is using the edges mapping strategy", func() {
				BeforeEach(func() {
					controller.AddAnnotations(ingress, map[string]string{
						"k8s.ngrok.com/mapping-strategy": "edges",
					})
				})

				It("Should only create an HTTPSEdge", func() {
					Expect(len(foundHTTPEdges.Items)).To(Equal(1))
					Expect(len(foundAgentEndpoints.Items)).To(Equal(0))
					Expect(len(foundCloudEndpoints.Items)).To(Equal(0))
				})

				When("The traffic policy exists", func() {
					BeforeEach(func() {
						controller.AddAnnotations(ingress, map[string]string{
							"k8s.ngrok.com/traffic-policy": trafficPolicy.Name,
						})
					})

					It("Should use the traffic policy", func() {
						edge := foundHTTPEdges.Items[0]
						Expect(edge.Spec.Routes).To(HaveLen(1))

						By("Having the traffic policy on all routes on the edge")
						for _, route := range edge.Spec.Routes {
							Expect(route.Policy).To(Equal(trafficPolicy.Spec.Policy))
						}
					})
				})
			})

			When("The ingress is using the default mapping strategy", func() {

				It("Should only create an AgentEndpoint", func() {
					Expect(len(foundAgentEndpoints.Items)).To(Equal(1))
					Expect(len(foundCloudEndpoints.Items)).To(Equal(0))
					Expect(len(foundHTTPEdges.Items)).To(Equal(0))
				})

				When("The traffic policy exists", func() {
					BeforeEach(func() {
						controller.AddAnnotations(ingress, map[string]string{
							"k8s.ngrok.com/traffic-policy": trafficPolicy.Name,
						})
					})

					It("Should include the traffic policy", func() {
						agentEndpoint := foundAgentEndpoints.Items[0]

						pol, err := trafficpolicy.NewTrafficPolicyFromJSON(agentEndpoint.Spec.TrafficPolicy.Inline)
						Expect(err).ToNot(HaveOccurred())

						Expect(pol.OnHTTPRequest).To(ContainElement(
							trafficpolicy.Rule{
								Name: "test-name",
								Actions: []trafficpolicy.Action{
									{
										Type:   "compress-response",
										Config: map[string]interface{}{},
									},
								},
							},
						))
					})
				})
			})
		})

		When("The defaultDomainReclaimPolicy is set", func() {
			var (
				defaultDomainReclaimPolicy ingressv1alpha1.DomainReclaimPolicy
				objs                       []runtime.Object
				c                          client.WithWatch
			)

			BeforeEach(func() {
				objs = []runtime.Object{
					testutils.NewTestIngressClass("test-ingress-class", true, true),
					testutils.NewTestIngressV1("test-ingress", "test-namespace"),
					testutils.NewTestServiceV1("example", "test-namespace"),
				}
			})

			JustBeforeEach(func(ctx SpecContext) {
				driver = NewDriver(
					GinkgoLogr,
					scheme,
					testutils.DefaultControllerName,
					types.NamespacedName{Name: defaultManagerName},
					WithGatewayEnabled(false),
					WithSyncAllowConcurrent(true),
					WithDefaultDomainReclaimPolicy(defaultDomainReclaimPolicy),
				)
				c = fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()
				Expect(driver.Seed(ctx, c)).To(Succeed())
				Expect(driver.Sync(ctx, c)).To(Succeed())
			})

			When("policy is Delete", func() {
				BeforeEach(func() {
					defaultDomainReclaimPolicy = ingressv1alpha1.DomainReclaimPolicyDelete
				})

				When("no domains exist", func() {
					It("should create new domains with ReclaimPolicy set to Delete", func(ctx SpecContext) {
						domains := &ingressv1alpha1.DomainList{}
						Expect(c.List(ctx, domains)).To(Succeed())
						Expect(domains.Items).To(HaveLen(1))
						Expect(domains.Items[0].Spec.ReclaimPolicy).To(Equal(ingressv1alpha1.DomainReclaimPolicyDelete))
					})
				})

				When("a domain already exists", func() {
					BeforeEach(func() {
						d := testutils.NewDomainV1("example.com", "test-namespace")
						d.ObjectMeta.SetCreationTimestamp(metav1.Now())
						d.Spec.ReclaimPolicy = ingressv1alpha1.DomainReclaimPolicyRetain

						objs = append(objs, d)
					})

					It("should not modify the reclaim policy of existing domains", func(ctx SpecContext) {
						domains := &ingressv1alpha1.DomainList{}
						Expect(c.List(ctx, domains)).To(Succeed())
						Expect(domains.Items).To(HaveLen(1))
						Expect(domains.Items[0].Spec.ReclaimPolicy).To(Equal(ingressv1alpha1.DomainReclaimPolicyRetain))
					})
				})
			})

			When("policy is Retain", func() {
				BeforeEach(func() {
					defaultDomainReclaimPolicy = ingressv1alpha1.DomainReclaimPolicyRetain
				})

				When("no domains exist", func() {
					It("should create new domains with ReclaimPolicy set to Retain", func(ctx SpecContext) {
						domains := &ingressv1alpha1.DomainList{}
						Expect(c.List(ctx, domains)).To(Succeed())
						Expect(domains.Items).To(HaveLen(1))
						Expect(domains.Items[0].Spec.ReclaimPolicy).To(Equal(ingressv1alpha1.DomainReclaimPolicyRetain))
					})
				})

				When("a domain already exists", func() {
					BeforeEach(func() {
						d := testutils.NewDomainV1("example.com", "test-namespace")
						d.ObjectMeta.SetCreationTimestamp(metav1.Now())
						d.Spec.ReclaimPolicy = ingressv1alpha1.DomainReclaimPolicyDelete

						objs = append(objs, d)
					})

					It("should not modify the reclaim policy of existing domains", func(ctx SpecContext) {
						domains := &ingressv1alpha1.DomainList{}
						Expect(c.List(ctx, domains)).To(Succeed())
						Expect(domains.Items).To(HaveLen(1))
						Expect(domains.Items[0].Spec.ReclaimPolicy).To(Equal(ingressv1alpha1.DomainReclaimPolicyDelete))
					})
				})
			})
		})
	})

	Describe("calculateIngressLoadBalancerIPStatus", func() {
		var domains []ingressv1alpha1.Domain
		var ingress *netv1.Ingress
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
			domainsByDomain, err := getDomainsByDomain(GinkgoT().Context(), c)
			Expect(err).ToNot(HaveOccurred())
			status = calculateIngressLoadBalancerIPStatus(ingress, domainsByDomain)
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
				ingress = testutils.NewTestIngressV1("test-ingress", "test-namespace")
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
				ingress = testutils.NewTestIngressV1("test-ingress", "test-namespace")
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
				ingress = testutils.NewTestIngressV1("test-ingress", "test-namespace")
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
				ingress = testutils.NewTestIngressV1("test-ingress", "test-namespace")
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
				ingress = testutils.NewTestIngressV1("test-ingress", "test-namespace")
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
				ingress = testutils.NewTestIngressV1("test-ingress", "test-namespace")
				addIngressHostname(ingress, "test-domain1.com")
				addIngressHostname(ingress, "test-domain2.com")
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
				ingress = testutils.NewTestIngressV1("test-ingress", "test-namespace")
				addIngressHostname(ingress, "test-domain1.com")
				addIngressHostname(ingress, "test-domain1.com")
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
			ing := testutils.NewTestIngressV1("test-ingress", "test")
			Expect(driver.store.Add(ing)).To(BeNil())

			ms, err := getNgrokModuleSetForObject(ing, driver.store)
			Expect(err).To(BeNil())
			Expect(ms.Modules.Compression).To(BeNil())
			Expect(ms.Modules.Headers).To(BeNil())
			Expect(ms.Modules.IPRestriction).To(BeNil())
			Expect(ms.Modules.TLSTermination).To(BeNil())
			Expect(ms.Modules.WebhookVerification).To(BeNil())
		})

		It("Should return the matching module set if the ingress has a modules annotaion", func() {
			ing := testutils.NewTestIngressV1("test-ingress", "test")
			ing.SetAnnotations(map[string]string{"k8s.ngrok.com/modules": "ms1"})
			Expect(driver.store.Add(ing)).To(BeNil())

			ms, err := getNgrokModuleSetForObject(ing, driver.store)
			Expect(err).To(BeNil())
			Expect(ms.Modules).To(Equal(ms1.Modules))
		})

		It("merges modules with the last one winning if multiple module sets are specified", func() {
			ing := testutils.NewTestIngressV1("test-ingress", "test")
			ing.SetAnnotations(map[string]string{"k8s.ngrok.com/modules": "ms1,ms2,ms3"})
			Expect(driver.store.Add(ing)).To(BeNil())

			ms, err := getNgrokModuleSetForObject(ing, driver.store)
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

			err := secondWait(GinkgoT().Context())
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

			secondErr := secondWait(GinkgoT().Context())
			Expect(secondErr).To(BeNil())

			driver.syncDone()

			err := thirdWait(GinkgoT().Context())
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

			thirdErr := thirdWait(GinkgoT().Context())
			Expect(thirdErr).To(BeNil())

			driver.syncDone()

			err := secondWait(GinkgoT().Context())
			Expect(err).To(Equal(errSyncDone))
		})
	})

	Describe("When ingresses are opted in to use edges", func() {
		It("Should create edges and not endpoints", func() {
			ic1 := testutils.NewTestIngressClass("test-ingress-class", true, true)

			i1 := testutils.NewTestIngressV1WithClass("ingress-1", "test-namespace", ic1.Name)
			if i1.Annotations == nil {
				i1.Annotations = map[string]string{}
			}
			i1.Annotations["k8s.ngrok.com/mapping-strategy"] = "edges"
			i1.Spec.Rules = []netv1.IngressRule{
				{
					Host: "a.customdomain.com",
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
					Host: "b.customdomain.com",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path: "/b/",
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: "b-service",
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
			i2 := testutils.NewTestIngressV1WithClass("ingress-2", "other-namespace", ic1.Name)
			if i2.Annotations == nil {
				i2.Annotations = map[string]string{}
			}
			i2.Annotations["k8s.ngrok.com/mapping-strategy"] = "edges"
			i2.Spec.Rules = []netv1.IngressRule{
				{
					Host: "c.customdomain.com",
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
				{
					Host: "d.customdomain.com",
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

			obs := []runtime.Object{ic1, i1, i2}
			c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(obs...).Build()
			Expect(driver.Seed(GinkgoT().Context(), c)).To(BeNil())

			domainSet := driver.calculateDomainSet()
			Expect(domainSet.edgeIngressDomains).To(HaveLen(4))
			Expect(domainSet.edgeIngressDomains).To(HaveKey("a.customdomain.com"))
			Expect(domainSet.edgeIngressDomains).To(HaveKey("b.customdomain.com"))
			Expect(domainSet.edgeIngressDomains).To(HaveKey("c.customdomain.com"))
			Expect(domainSet.edgeIngressDomains).To(HaveKey("d.customdomain.com"))
			Expect(domainSet.endpointIngressDomains).To(HaveLen(0))
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
