package ingress

import (
	"context"
	"fmt"
	"time"

	commonv1alpha1 "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/ippolicy"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("IPPolicyReconciler", func() {
	const (
		timeout  = 10 * time.Second
		interval = 250 * time.Millisecond
	)

	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = GinkgoT().Context()
	})

	AfterEach(func() {
		ipPolicyClient.Reset()
	})

	It("creates IPPolicy and configures rules", func() {
		ip := &ingressv1alpha1.IPPolicy{
			Name: "test-ip-policy", Namespace: "default",
			Spec: ingressv1alpha1.IPPolicySpec{
				Metadata: commonv1alpha1.MetadataFromLegacyString("test"),
			},
		}
		ip.Spec.Rules = []ingressv1alpha1.IPPolicyRule{
			{CIDR: "10.0.0.0/8", Action: "allow", Description: "desc1"},
			{CIDR: "192.168.1.0/24", Action: "deny", Description: "desc2"},
		}

		Expect(k8sClient.Create(ctx, ip)).To(Succeed())

		Eventually(func() []string {
			items := ipPolicyRuleClient.Items()
			out := []string{}
			for _, it := range items {
				out = append(out, it.CIDR)
			}
			return out
		}, timeout, interval).Should(ContainElements("10.0.0.0/8", "192.168.1.0/24"))
	})

	It("updates existing rule descriptions", func() {
		ip := &ingressv1alpha1.IPPolicy{
			Name: "test-ip-policy-update", Namespace: "default",
			Spec: ingressv1alpha1.IPPolicySpec{
				Metadata: commonv1alpha1.MetadataFromLegacyString("test"),
			},
		}
		ip.Spec.Rules = []ingressv1alpha1.IPPolicyRule{
			{CIDR: "10.0.0.0/8", Action: "allow", Description: "orig"},
		}

		Expect(k8sClient.Create(ctx, ip)).To(Succeed())

		Eventually(func() []string {
			items := ipPolicyRuleClient.Items()
			out := []string{}
			for _, it := range items {
				out = append(out, it.Description)
			}
			return out
		}, timeout, interval).Should(ContainElement("orig"))

		patch := client.MergeFrom(ip.DeepCopy())
		ip.Spec.Rules[0].Description = "updated"
		Expect(k8sClient.Patch(ctx, ip, patch)).To(Succeed())

		Eventually(func() []string {
			items := ipPolicyRuleClient.Items()
			out := []string{}
			for _, it := range items {
				out = append(out, it.Description)
			}
			return out
		}, timeout, interval).Should(ContainElement("updated"))
	})

	It("deletes obsolete rules", func() {
		ip := &ingressv1alpha1.IPPolicy{
			Name: "test-ip-policy-del", Namespace: "default",
			Spec: ingressv1alpha1.IPPolicySpec{
				Metadata: commonv1alpha1.MetadataFromLegacyString("test"),
			},
		}
		ip.Spec.Rules = []ingressv1alpha1.IPPolicyRule{
			{CIDR: "10.0.0.0/8", Action: "allow", Description: "orig"},
		}

		Expect(k8sClient.Create(ctx, ip)).To(Succeed())

		Eventually(func() []string {
			items := ipPolicyRuleClient.Items()
			out := []string{}
			for _, it := range items {
				out = append(out, it.CIDR)
			}
			return out
		}, timeout, interval).Should(ContainElement("10.0.0.0/8"))

		patch := client.MergeFrom(ip.DeepCopy())
		ip.Spec.Rules = []ingressv1alpha1.IPPolicyRule{}
		Expect(k8sClient.Patch(ctx, ip, patch)).To(Succeed())

		Eventually(func() int {
			count := 0
			for _, it := range ipPolicyRuleClient.Items() {
				if it.IPPolicy.ID == ip.Status.ID {
					count++
				}
			}
			return count
		}, timeout, interval).Should(Equal(0))
	})

	It("sets Created condition for existing IP Policy", func() {
		By("creating an IP Policy")
		ip := &ingressv1alpha1.IPPolicy{
			Name: "test-existing-policy", Namespace: "default",
			Spec: ingressv1alpha1.IPPolicySpec{
				Metadata: commonv1alpha1.MetadataFromLegacyString("test"),
			},
		}
		ip.Spec.Description = "test-existing"
		Expect(k8sClient.Create(ctx, ip)).To(Succeed())

		By("waiting for the IP Policy to be created")
		kginkgo.EventuallyIPPolicyHasCondition(ctx, ip, ippolicy.ConditionIPPolicyCreated, metav1.ConditionTrue)
		kginkgo.EventuallyIPPolicyHasCondition(ctx, ip, ippolicy.ConditionIPPolicyReady, metav1.ConditionTrue)

		By("clearning the status conditions to simulate existing ip policy")
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(ip), ip)
			g.Expect(err).NotTo(HaveOccurred())

			ip.Status.Conditions = []metav1.Condition{}
			err = k8sClient.Status().Update(ctx, ip)
			g.Expect(err).NotTo(HaveOccurred())
		}, timeout, interval).Should(Succeed())

		By("touching an annotation to trigger a reconcile, since status-only changes are now filtered")
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(ip), ip)
			g.Expect(err).NotTo(HaveOccurred())

			if ip.Annotations == nil {
				ip.Annotations = map[string]string{}
			}
			ip.Annotations["test.k8s.ngrok.com/force-reconcile"] = "1"
			err = k8sClient.Update(ctx, ip)
			g.Expect(err).NotTo(HaveOccurred())
		}, timeout, interval).Should(Succeed())

		By("verifying the created and ready conditions are set again")
		kginkgo.EventuallyIPPolicyHasCondition(ctx, ip, ippolicy.ConditionIPPolicyCreated, metav1.ConditionTrue)
		kginkgo.EventuallyIPPolicyHasCondition(ctx, ip, ippolicy.ConditionIPPolicyReady, metav1.ConditionTrue)
	})

	It("sets Created condition for existing IP Policy with rules", func() {
		By("creating an IPPolicy resource with rules")
		ip := &ingressv1alpha1.IPPolicy{
			Name: "test-existing-policy-rules", Namespace: "default",
			Spec: ingressv1alpha1.IPPolicySpec{
				Metadata: commonv1alpha1.MetadataFromLegacyString("test"),
			},
		}
		ip.Spec.Description = "test-with-rules"
		ip.Spec.Rules = []ingressv1alpha1.IPPolicyRule{{CIDR: "10.0.0.0/8", Action: "allow"}}
		Expect(k8sClient.Create(ctx, ip)).To(Succeed())

		By("waiting for the IP Policy to be created")
		kginkgo.EventuallyIPPolicyHasCondition(ctx, ip, ippolicy.ConditionIPPolicyCreated, metav1.ConditionTrue)
		kginkgo.EventuallyIPPolicyHasCondition(ctx, ip, ippolicy.ConditionIPPolicyReady, metav1.ConditionTrue)
		kginkgo.EventuallyIPPolicyHasCondition(ctx, ip, ippolicy.ConditionIPPolicyRulesConfigured, metav1.ConditionTrue)

		By("clearing the status conditions to simulate an existing ip policy")
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(ip), ip)
			g.Expect(err).NotTo(HaveOccurred())

			ip.Status.Conditions = []metav1.Condition{}
			err = k8sClient.Status().Update(ctx, ip)
			g.Expect(err).NotTo(HaveOccurred())
		}, timeout, interval).Should(Succeed())

		By("touching an annotation to trigger a reconcile, since status-only changes are now filtered")
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(ip), ip)
			g.Expect(err).NotTo(HaveOccurred())

			if ip.Annotations == nil {
				ip.Annotations = map[string]string{}
			}
			ip.Annotations["test.k8s.ngrok.com/force-reconcile"] = "1"
			err = k8sClient.Update(ctx, ip)
			g.Expect(err).NotTo(HaveOccurred())
		}, timeout, interval).Should(Succeed())

		By("verifying the created, rules configured, and ready conditions are set again")
		kginkgo.EventuallyIPPolicyHasCondition(ctx, ip, ippolicy.ConditionIPPolicyCreated, metav1.ConditionTrue)
		kginkgo.EventuallyIPPolicyHasCondition(ctx, ip, ippolicy.ConditionIPPolicyReady, metav1.ConditionTrue)
		kginkgo.EventuallyIPPolicyHasCondition(ctx, ip, ippolicy.ConditionIPPolicyRulesConfigured, metav1.ConditionTrue)
	})
})

var _ = Describe("IPPolicy schema validation", func() {
	newPolicy := func(name, cidr string) *ingressv1alpha1.IPPolicy {
		return &ingressv1alpha1.IPPolicy{
			Name: name, Namespace: "default",
			Spec: ingressv1alpha1.IPPolicySpec{
				Rules: []ingressv1alpha1.IPPolicyRule{{CIDR: cidr, Action: "allow"}},
			},
		}
	}

	It("accepts valid IPv4 and IPv6 CIDRs", func(ctx SpecContext) {
		for i, cidr := range []string{
			"10.0.0.0/8", "192.168.1.128/25", "0.0.0.0/0",
			"2001:db8::/32", "::/0", "fe80::1/128", "::1/128",
			"1:2:3:4:5:6:7:8/128", "2001:db8:0:0:0:0:2:1/64",
		} {
			ip := newPolicy(fmt.Sprintf("schema-valid-cidr-%d", i), cidr)
			Expect(k8sClient.Create(ctx, ip)).To(Succeed(), "expected CIDR %q to be accepted", cidr)
		}
	})

	It("rejects invalid CIDRs", func(ctx SpecContext) {
		for i, cidr := range []string{
			"10.0.0.1", "not-a-cidr", "10.0.0.0/33", "300.0.0.0/8", "2001:db8::/129", "",
			// malformed compressed/leading/trailing-colon IPv6 forms that a too-loose
			// pattern could otherwise admit
			":::/64", ":/64", "2001:db8:::/32", "1::2::3/64", "::g/64",
		} {
			ip := newPolicy(fmt.Sprintf("schema-invalid-cidr-%d", i), cidr)
			err := k8sClient.Create(ctx, ip)
			Expect(err).To(HaveOccurred(), "expected CIDR %q to be rejected", cidr)
			Expect(err.Error()).To(ContainSubstring("cidr"), "expected error for CIDR %q to mention the cidr field", cidr)
		}
	})
})
