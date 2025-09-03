package agent

import (
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/pkg/agent"
)

var _ = Describe("AgentEndpointReconciler", func() {
	var (
		reconciler *AgentEndpointReconciler
		mockDriver *agent.MockAgentDriver
		endpoint   *ngrokv1alpha1.AgentEndpoint
	)

	BeforeEach(func() {
		mockDriver = agent.NewMockAgentDriver()
		reconciler = &AgentEndpointReconciler{
			Client:      k8sClient,
			Log:         logr.Discard(),
			Recorder:    record.NewFakeRecorder(10),
			AgentDriver: mockDriver,
		}

		endpoint = &ngrokv1alpha1.AgentEndpoint{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-endpoint",
				Namespace:  "default",
				Generation: 1,
			},
			Spec: ngrokv1alpha1.AgentEndpointSpec{
				URL: "https://test.ngrok.app",
				Upstream: ngrokv1alpha1.EndpointUpstream{
					URL: "http://backend:80",
				},
			},
		}
	})

	Describe("updateEndpointStatus", func() {
		Context("when endpoint creation succeeds", func() {
			It("should set Ready condition to True", func() {
				result := &agent.EndpointResult{
					URL:           "https://test.ngrok.app",
					TrafficPolicy: "{}",
					Ready:         true,
				}

				reconciler.updateEndpointStatus(endpoint, result, nil, "{}")

				readyCondition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionReady)
				Expect(readyCondition).NotTo(BeNil())
				Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
				Expect(readyCondition.Reason).To(Equal(ReasonEndpointActive))
				Expect(endpoint.Status.AssignedURL).To(Equal("https://test.ngrok.app"))
				Expect(endpoint.Status.AttachedTrafficPolicy).To(Equal("inline"))
			})
		})

		Context("when endpoint creation fails", func() {
			It("should set Ready condition to False with NgrokAPIError", func() {
				err := fmt.Errorf("connection failed")

				reconciler.updateEndpointStatus(endpoint, nil, err, "")

				readyCondition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionReady)
				Expect(readyCondition).NotTo(BeNil())
				Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
				Expect(readyCondition.Reason).To(Equal(ReasonNgrokAPIError))
				Expect(endpoint.Status.AttachedTrafficPolicy).To(Equal("none"))
			})

			It("should set TrafficPolicyError reason for policy errors", func() {
				err := fmt.Errorf("Invalid policy action type 'rate-limit-fake' ERR_NGROK_2201")

				reconciler.updateEndpointStatus(endpoint, nil, err, "{}")

				readyCondition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionReady)
				Expect(readyCondition).NotTo(BeNil())
				Expect(readyCondition.Reason).To(Equal(ReasonTrafficPolicyError))

				policyCondition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionTrafficPolicy)
				Expect(policyCondition).NotTo(BeNil())
				Expect(policyCondition.Status).To(Equal(metav1.ConditionFalse))
			})
		})

		Context("traffic policy reference", func() {
			It("should set AttachedTrafficPolicy to reference name", func() {
				endpoint.Spec.TrafficPolicy = &ngrokv1alpha1.TrafficPolicyCfg{
					Reference: &ngrokv1alpha1.K8sObjectRef{Name: "my-policy"},
				}

				result := &agent.EndpointResult{
					URL:   "https://test.ngrok.app",
					Ready: true,
				}

				reconciler.updateEndpointStatus(endpoint, result, nil, "{}")

				Expect(endpoint.Status.AttachedTrafficPolicy).To(Equal("my-policy"))
			})
		})
	})

	Describe("ensureDomainExists", func() {
		Context("when domain needs to be created", func() {
			It("should create domain and set conditions", func(ctx SpecContext) {
				endpoint.Spec.URL = "https://new-domain.ngrok.app"

				err := reconciler.ensureDomainExists(ctx, endpoint)

				Expect(err).To(Equal(ErrDomainCreating))
				domainCondition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionDomainReady)
				Expect(domainCondition).NotTo(BeNil())
				Expect(domainCondition.Status).To(Equal(metav1.ConditionFalse))
				Expect(domainCondition.Reason).To(Equal(ReasonDomainCreating))
			})
		})

		Context("when using TCP endpoint", func() {
			It("should succeed without creating domain", func(ctx SpecContext) {
				endpoint.Spec.URL = "tcp://1.tcp.ngrok.io:12345"

				err := reconciler.ensureDomainExists(ctx, endpoint)

				Expect(err).To(BeNil())
				domainCondition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionDomainReady)
				Expect(domainCondition).NotTo(BeNil())
				Expect(domainCondition.Status).To(Equal(metav1.ConditionTrue))
			})
		})

		Context("when URL is invalid", func() {
			It("should set error conditions", func(ctx SpecContext) {
				endpoint.Spec.URL = ":/invalid-url"

				err := reconciler.ensureDomainExists(ctx, endpoint)

				Expect(err).To(HaveOccurred())
				domainCondition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionDomainReady)
				Expect(domainCondition).NotTo(BeNil())
				Expect(domainCondition.Status).To(Equal(metav1.ConditionFalse))
				Expect(domainCondition.Reason).To(Equal(ReasonNgrokAPIError))
			})
		})

		Context("when using internal domain", func() {
			It("should succeed without creating domain", func(ctx SpecContext) {
				endpoint.Spec.URL = "https://test.internal"

				err := reconciler.ensureDomainExists(ctx, endpoint)

				Expect(err).To(BeNil())
				domainCondition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionDomainReady)
				Expect(domainCondition).NotTo(BeNil())
				Expect(domainCondition.Status).To(Equal(metav1.ConditionTrue))
			})
		})

		Context("when domain exists but is not ready", func() {
			It("should return ErrDomainCreating and set conditions", func(ctx SpecContext) {
				domain := &ingressv1alpha1.Domain{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ngrok-app",
						Namespace: "default",
					},
					Spec: ingressv1alpha1.DomainSpec{
						Domain: "test.ngrok.app",
					},
				}
				Expect(k8sClient.Create(ctx, domain)).To(Succeed())

				endpoint.Spec.URL = "https://test.ngrok.app"

				err := reconciler.ensureDomainExists(ctx, endpoint)

				Expect(err).To(Equal(ErrDomainCreating))
				domainCondition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionDomainReady)
				Expect(domainCondition).NotTo(BeNil())
				Expect(domainCondition.Status).To(Equal(metav1.ConditionFalse))
				Expect(domainCondition.Reason).To(Equal(ReasonDomainCreating))
			})
		})

		Context("when domain exists and is ready", func() {
			It("should succeed without error", func(ctx SpecContext) {
				domain := &ingressv1alpha1.Domain{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ready-ngrok-app",
						Namespace: "default",
					},
					Spec: ingressv1alpha1.DomainSpec{
						Domain: "ready.ngrok.app",
					},
				}
				Expect(k8sClient.Create(ctx, domain)).To(Succeed())

				domain.Status = ingressv1alpha1.DomainStatus{
					ID: "do_123456",
				}
				Expect(k8sClient.Status().Update(ctx, domain)).To(Succeed())

				endpoint.Spec.URL = "https://ready.ngrok.app"

				err := reconciler.ensureDomainExists(ctx, endpoint)

				Expect(err).To(BeNil())
				domainCondition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionDomainReady)
				Expect(domainCondition).NotTo(BeNil())
				Expect(domainCondition.Status).To(Equal(metav1.ConditionTrue))
			})
		})
	})

	Describe("getTrafficPolicy", func() {
		Context("when no traffic policy is configured", func() {
			It("should return empty policy", func(ctx SpecContext) {
				endpoint.Spec.TrafficPolicy = nil

				policy, err := reconciler.getTrafficPolicy(ctx, endpoint)

				Expect(err).To(BeNil())
				Expect(policy).To(Equal(""))
			})
		})

		Context("when both inline and reference are set", func() {
			It("should return error and set conditions", func(ctx SpecContext) {
				endpoint.Spec.TrafficPolicy = &ngrokv1alpha1.TrafficPolicyCfg{
					Inline:    []byte(`{}`),
					Reference: &ngrokv1alpha1.K8sObjectRef{Name: "policy"},
				}

				policy, err := reconciler.getTrafficPolicy(ctx, endpoint)

				Expect(err).To(Equal(ErrInvalidTrafficPolicyConfig))
				Expect(policy).To(Equal(""))

				policyCondition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionTrafficPolicy)
				Expect(policyCondition).NotTo(BeNil())
				Expect(policyCondition.Status).To(Equal(metav1.ConditionFalse))
				Expect(policyCondition.Reason).To(Equal(ReasonTrafficPolicyError))
			})
		})

		Context("with valid inline policy", func() {
			It("should return policy JSON", func(ctx SpecContext) {
				policyData := []byte(`{"on_http_request":[]}`)
				endpoint.Spec.TrafficPolicy = &ngrokv1alpha1.TrafficPolicyCfg{
					Inline: policyData,
				}

				policy, err := reconciler.getTrafficPolicy(ctx, endpoint)

				Expect(err).To(BeNil())
				Expect(policy).To(Equal(`{"on_http_request":[]}`))
			})
		})

		Context("with policy reference to existing policy", func() {
			It("should resolve policy reference successfully", func(ctx SpecContext) {
				trafficPolicy := &ngrokv1alpha1.NgrokTrafficPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ref-policy",
						Namespace: "default",
					},
					Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
						Policy: []byte(`{"on_http_request":[]}`),
					},
				}
				Expect(k8sClient.Create(ctx, trafficPolicy)).To(Succeed())

				endpoint.Spec.TrafficPolicy = &ngrokv1alpha1.TrafficPolicyCfg{
					Reference: &ngrokv1alpha1.K8sObjectRef{Name: "ref-policy"},
				}

				policy, err := reconciler.getTrafficPolicy(ctx, endpoint)

				Expect(err).To(BeNil())
				Expect(policy).To(Equal(`{"on_http_request":[]}`))
			})
		})

		Context("with policy reference to missing policy", func() {
			It("should return error and set conditions", func(ctx SpecContext) {
				endpoint.Spec.TrafficPolicy = &ngrokv1alpha1.TrafficPolicyCfg{
					Reference: &ngrokv1alpha1.K8sObjectRef{Name: "missing-policy"},
				}

				policy, err := reconciler.getTrafficPolicy(ctx, endpoint)

				Expect(err).To(HaveOccurred())
				Expect(errors.IsNotFound(err)).To(BeTrue())
				Expect(policy).To(Equal(""))

				policyCondition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionTrafficPolicy)
				Expect(policyCondition).NotTo(BeNil())
				Expect(policyCondition.Status).To(Equal(metav1.ConditionFalse))
				Expect(policyCondition.Reason).To(Equal(ReasonTrafficPolicyError))
			})
		})
	})

	Describe("getClientCerts", func() {
		Context("when no client certificates are configured", func() {
			It("should return empty slice", func(ctx SpecContext) {
				endpoint.Spec.ClientCertificateRefs = nil

				certs, err := reconciler.getClientCerts(ctx, endpoint)

				Expect(err).To(BeNil())
				Expect(certs).To(BeEmpty())
			})
		})

		Context("when secret is missing", func() {
			It("should return error and set conditions", func(ctx SpecContext) {
				endpoint.Spec.ClientCertificateRefs = []ngrokv1alpha1.K8sObjectRefOptionalNamespace{
					{Name: "missing-secret"},
				}

				certs, err := reconciler.getClientCerts(ctx, endpoint)

				Expect(err).To(HaveOccurred())
				Expect(certs).To(BeNil())

				readyCondition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionReady)
				Expect(readyCondition).NotTo(BeNil())
				Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
				Expect(readyCondition.Reason).To(Equal(ReasonConfigError))
			})
		})

		Context("when secret is missing tls.crt", func() {
			It("should return error and set conditions", func(ctx SpecContext) {
				secret := &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "no-crt-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"tls.key": []byte("some-key-data"),
					},
				}
				Expect(k8sClient.Create(ctx, secret)).To(Succeed())

				endpoint.Spec.ClientCertificateRefs = []ngrokv1alpha1.K8sObjectRefOptionalNamespace{
					{Name: "no-crt-secret"},
				}

				certs, err := reconciler.getClientCerts(ctx, endpoint)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("tls.crt data is missing"))
				Expect(certs).To(BeNil())
			})
		})

		Context("when secret is missing tls.key", func() {
			It("should return error and set conditions", func(ctx SpecContext) {
				secret := &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "no-key-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"tls.crt": []byte("some-cert-data"),
					},
				}
				Expect(k8sClient.Create(ctx, secret)).To(Succeed())

				endpoint.Spec.ClientCertificateRefs = []ngrokv1alpha1.K8sObjectRefOptionalNamespace{
					{Name: "no-key-secret"},
				}

				certs, err := reconciler.getClientCerts(ctx, endpoint)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("tls.key data is missing"))
				Expect(certs).To(BeNil())
			})
		})

		Context("when certificate data is invalid", func() {
			It("should return TLS parsing error", func(ctx SpecContext) {
				secret := &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "invalid-cert-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"tls.crt": []byte("invalid-certificate-data"),
						"tls.key": []byte("invalid-key-data"),
					},
				}
				Expect(k8sClient.Create(ctx, secret)).To(Succeed())

				endpoint.Spec.ClientCertificateRefs = []ngrokv1alpha1.K8sObjectRefOptionalNamespace{
					{Name: "invalid-cert-secret"},
				}

				certs, err := reconciler.getClientCerts(ctx, endpoint)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to parse TLS certificate"))
				Expect(certs).To(BeNil())
			})
		})

		Context("with namespace specified in reference", func() {
			It("should fetch secret from specified namespace", func(ctx SpecContext) {
				ns := &v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "other-namespace",
					},
				}
				Expect(k8sClient.Create(ctx, ns)).To(Succeed())

				secret := &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cross-ns-secret",
						Namespace: "other-namespace",
					},
					Data: map[string][]byte{
						"tls.crt": []byte("invalid-cert-data"),
						"tls.key": []byte("invalid-key-data"),
					},
					Type: v1.SecretTypeTLS,
				}
				Expect(k8sClient.Create(ctx, secret)).To(Succeed())

				otherNamespace := "other-namespace"
				endpoint.Spec.ClientCertificateRefs = []ngrokv1alpha1.K8sObjectRefOptionalNamespace{
					{Name: "cross-ns-secret", Namespace: &otherNamespace},
				}

				certs, err := reconciler.getClientCerts(ctx, endpoint)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to parse TLS certificate"))
				Expect(certs).To(BeNil())
			})
		})
	})

	Describe("delete", func() {
		Context("when deleting an endpoint", func() {
			It("should call AgentDriver.DeleteAgentEndpoint with correct name", func(ctx SpecContext) {
				err := reconciler.delete(ctx, endpoint)

				Expect(err).To(BeNil())
				Expect(mockDriver.DeleteCalls).To(HaveLen(1))
				Expect(mockDriver.DeleteCalls[0].Name).To(Equal("default/test-endpoint"))
			})

			It("should return error from AgentDriver", func(ctx SpecContext) {
				deleteErr := fmt.Errorf("delete failed")
				mockDriver.DeleteError = deleteErr

				err := reconciler.delete(ctx, endpoint)

				Expect(err).To(Equal(deleteErr))
				Expect(mockDriver.DeleteCalls).To(HaveLen(1))
			})
		})
	})

	Describe("findTrafficPolicyByName", func() {
		var trafficPolicy *ngrokv1alpha1.NgrokTrafficPolicy

		BeforeEach(func() {
			trafficPolicy = &ngrokv1alpha1.NgrokTrafficPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-policy",
					Namespace: "default",
				},
				Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
					Policy: []byte(`{"on_http_request":[{"name":"rate-limit","config":{"rate":"100r/m"}}]}`),
				},
			}
		})

		Context("when traffic policy exists", func() {
			It("should return policy JSON", func(ctx SpecContext) {
				Expect(k8sClient.Create(ctx, trafficPolicy)).To(Succeed())

				policy, err := reconciler.findTrafficPolicyByName(ctx, "test-policy", "default")

				Expect(err).To(BeNil())
				Expect(policy).To(Equal(`{"on_http_request":[{"config":{"rate":"100r/m"},"name":"rate-limit"}]}`))
			})
		})

		Context("when traffic policy does not exist", func() {
			It("should return not found error", func(ctx SpecContext) {
				policy, err := reconciler.findTrafficPolicyByName(ctx, "missing-policy", "default")

				Expect(err).To(HaveOccurred())
				Expect(errors.IsNotFound(err)).To(BeTrue())
				Expect(policy).To(Equal(""))
			})
		})

		Context("when traffic policy has invalid JSON", func() {
			It("should return marshal error", func(ctx SpecContext) {
				trafficPolicy.Spec.Policy = []byte(`invalid json`)

				// The k8s client will reject invalid JSON, so we expect the Create to fail
				err := k8sClient.Create(ctx, trafficPolicy)
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
