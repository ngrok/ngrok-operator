package nmockapi_test

import (
	"context"

	"github.com/ngrok/ngrok-api-go/v7"
	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("IPPolicyRuleClient", func() {
	var (
		client *nmockapi.IPPolicyRuleClient
		ctx    context.Context
	)

	BeforeEach(func() {
		client = nmockapi.NewIPPolicyRuleClient(nmockapi.NewIPPolicyClient())
		ctx = context.Background()
	})

	Describe("Create", func() {
		It("creates a rule successfully", func() {
			action := "allow"
			rule, err := client.Create(ctx, &ngrok.IPPolicyRuleCreate{
				Action: &action,
				CIDR:   "192.168.1.0/24",
			})
			Expect(err).To(BeNil())
			Expect(rule).NotTo(BeNil())
			Expect(rule.Action).To(Equal("allow"))
			Expect(rule.CIDR).To(Equal("192.168.1.0/24"))
		})

		It("fails when action is missing", func() {
			_, err := client.Create(ctx, &ngrok.IPPolicyRuleCreate{
				CIDR: "10.0.0.0/8",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.(*ngrok.Error).Msg).To(Equal("Missing action"))
		})

		It("fails when action is invalid", func() {
			action := "block"
			_, err := client.Create(ctx, &ngrok.IPPolicyRuleCreate{
				Action: &action,
				CIDR:   "10.0.0.0/8",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.(*ngrok.Error).Msg).To(ContainSubstring("Invalid action"))
		})

		It("fails when CIDR is invalid", func() {
			action := "deny"
			_, err := client.Create(ctx, &ngrok.IPPolicyRuleCreate{
				Action: &action,
				CIDR:   "10.0.0.0-8",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.(*ngrok.Error).Msg).To(ContainSubstring("Invalid CIDR"))
		})
	})

	Describe("Update", func() {
		var rule *ngrok.IPPolicyRule

		BeforeEach(func() {
			action := "allow"
			var err error
			rule, err = client.Create(ctx, &ngrok.IPPolicyRuleCreate{
				Action: &action,
				CIDR:   "192.168.1.0/24",
			})
			Expect(err).To(BeNil())
		})

		It("updates CIDR and description successfully", func() {
			newCIDR := "10.0.0.0/8"
			newDesc := "updated"
			updated, err := client.Update(ctx, &ngrok.IPPolicyRuleUpdate{
				ID:          rule.ID,
				CIDR:        &newCIDR,
				Description: &newDesc,
			})
			Expect(err).To(BeNil())
			Expect(updated.CIDR).To(Equal(newCIDR))
			Expect(updated.Description).To(Equal(newDesc))
		})

		It("fails to update with invalid CIDR", func() {
			badCIDR := "notacidr"
			_, err := client.Update(ctx, &ngrok.IPPolicyRuleUpdate{
				ID:   rule.ID,
				CIDR: &badCIDR,
			})
			Expect(err).To(HaveOccurred())
			Expect(err.(*ngrok.Error).Msg).To(ContainSubstring("Invalid CIDR"))
		})

		It("fails to update non-existent rule", func() {
			cidr := "10.0.0.0/8"
			_, err := client.Update(ctx, &ngrok.IPPolicyRuleUpdate{
				ID:   "does-not-exist",
				CIDR: &cidr,
			})
			Expect(err).To(HaveOccurred())
		})

		It("updates only description", func() {
			newDesc := "desc only"
			updated, err := client.Update(ctx, &ngrok.IPPolicyRuleUpdate{
				ID:          rule.ID,
				Description: &newDesc,
			})
			Expect(err).To(BeNil())
			Expect(updated.Description).To(Equal(newDesc))
			Expect(updated.CIDR).To(Equal(rule.CIDR))
		})
	})
})
