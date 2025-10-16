package ingress

import (
	"context"
	"time"

	ngrok "github.com/ngrok/ngrok-api-go/v7"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
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
		// reset mocks
		ipPolicyClient.Reset()
		ipPolicyRuleClient.SetItems(nil)
	})

	It("creates IPPolicy and configures rules", func() {
		ip := &ingressv1alpha1.IPPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "test-ip-policy", Namespace: "default"},
			Spec:       ingressv1alpha1.IPPolicySpec{},
		}
		ip.Spec.Metadata = "test"
		ip.Spec.Rules = []ingressv1alpha1.IPPolicyRule{{CIDR: "10.0.0.0/8", Action: "allow"}, {CIDR: "192.168.1.0/24", Action: "deny"}}

		// set descriptions after literal construction
		ip.Spec.Rules[0].Description = "desc1"
		ip.Spec.Rules[1].Description = "desc2"

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
			ObjectMeta: metav1.ObjectMeta{Name: "test-ip-policy-update", Namespace: "default"},
			Spec:       ingressv1alpha1.IPPolicySpec{},
		}
		ip.Spec.Metadata = "test"
		ip.Spec.Rules = []ingressv1alpha1.IPPolicyRule{{CIDR: "10.0.0.0/8", Action: "allow"}}
		ip.Spec.Rules[0].Description = "orig"

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
			ObjectMeta: metav1.ObjectMeta{Name: "test-ip-policy-del", Namespace: "default"},
			Spec:       ingressv1alpha1.IPPolicySpec{},
		}
		ip.Spec.Metadata = "test"
		ip.Spec.Rules = []ingressv1alpha1.IPPolicyRule{{CIDR: "10.0.0.0/8", Action: "allow"}}
		ip.Spec.Rules[0].Description = "orig"

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
})

var _ = Describe("IPPolicyDiff", func() {
	It("computes creates, deletes and updates correctly", func() {
		remoteRules := []*ngrok.IPPolicyRule{
			{ID: "1", CIDR: "192.168.1.0/25", Action: IPPolicyRuleActionAllow, Description: "a"},
			{ID: "2", CIDR: "192.168.128.0/25", Action: IPPolicyRuleActionDeny, Description: "aa"},
			{ID: "3", CIDR: "172.16.0.0/16", Action: IPPolicyRuleActionAllow, Description: "aaa"},
			{ID: "4", CIDR: "172.17.0.0/16", Action: IPPolicyRuleActionDeny, Description: "aaaa"},
			{ID: "5", CIDR: "172.19.0.0/16", Action: IPPolicyRuleActionAllow, Description: "aaaaa"},
		}

		changedDescriptionRule := ingressv1alpha1.IPPolicyRule{CIDR: "172.19.0.0/16", Action: IPPolicyRuleActionAllow}
		changedDescriptionRule.Description = "b"

		specRules := []ingressv1alpha1.IPPolicyRule{
			{CIDR: "192.168.1.0/25", Action: IPPolicyRuleActionDeny},
			{CIDR: "192.168.128.0/25", Action: IPPolicyRuleActionAllow},
			{CIDR: "10.0.0.0/8", Action: IPPolicyRuleActionDeny},
			{CIDR: "172.18.0.0/16", Action: IPPolicyRuleActionAllow},
			changedDescriptionRule,
		}

		diff := newIPPolicyDiff("test", remoteRules, specRules)

		Expect(diff.Next()).To(BeTrue())
		Expect(diff.NeedsDelete()).To(BeEmpty())
		Expect(diff.NeedsUpdate()).To(BeEmpty())
		Expect(diff.NeedsCreate()).To(Equal([]*ngrok.IPPolicyRuleCreate{{IPPolicyID: "test", CIDR: specRules[2].CIDR, Action: ptr.To(IPPolicyRuleActionDeny)}}))

		Expect(diff.Next()).To(BeTrue())
		Expect(diff.NeedsUpdate()).To(BeEmpty())
		Expect(diff.NeedsDelete()).To(Equal([]*ngrok.IPPolicyRule{remoteRules[0]}))
		Expect(diff.NeedsCreate()).To(Equal([]*ngrok.IPPolicyRuleCreate{{IPPolicyID: "test", CIDR: specRules[0].CIDR, Action: ptr.To(IPPolicyRuleActionDeny)}}))

		Expect(diff.Next()).To(BeTrue())
		Expect(diff.NeedsUpdate()).To(BeEmpty())
		Expect(diff.NeedsDelete()).To(Equal([]*ngrok.IPPolicyRule{remoteRules[1]}))
		Expect(diff.NeedsCreate()).To(Equal([]*ngrok.IPPolicyRuleCreate{{IPPolicyID: "test", CIDR: specRules[1].CIDR, Action: ptr.To(IPPolicyRuleActionAllow)}}))

		Expect(diff.Next()).To(BeTrue())
		Expect(diff.NeedsUpdate()).To(BeEmpty())
		Expect(diff.NeedsDelete()).To(BeEmpty())
		Expect(diff.NeedsCreate()).To(Equal([]*ngrok.IPPolicyRuleCreate{{IPPolicyID: "test", CIDR: specRules[3].CIDR, Action: ptr.To(IPPolicyRuleActionAllow)}}))

		Expect(diff.Next()).To(BeTrue())
		Expect(diff.NeedsUpdate()).To(BeEmpty())
		Expect(diff.NeedsDelete()).To(Equal([]*ngrok.IPPolicyRule{remoteRules[2], remoteRules[3]}))
		Expect(diff.NeedsCreate()).To(BeEmpty())

		Expect(diff.Next()).To(BeTrue())
		Expect(diff.NeedsDelete()).To(BeEmpty())
		Expect(diff.NeedsCreate()).To(BeEmpty())
		Expect(diff.NeedsUpdate()).To(Equal([]*ngrok.IPPolicyRuleUpdate{{ID: "5", CIDR: ptr.To("172.19.0.0/16"), Description: ptr.To("b"), Metadata: ptr.To("")}}))

		Expect(diff.Next()).To(BeFalse())
	})
})
