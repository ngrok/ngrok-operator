package controller

import (
	"testing"

	"github.com/ngrok/ngrok-api-go/v5"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controllers package Test Suite")
}

var _ = Describe("HTTPSEdgeController", func() {
	DescribeTable("isMigratingAuthProviders", func(current *ngrok.HTTPSEdgeRoute, desired *ingressv1alpha1.HTTPSEdgeRouteSpec, expected bool) {
		Expect(isMigratingAuthProviders(current, desired)).To(Equal(expected))
	},
		Entry("no auth", &ngrok.HTTPSEdgeRoute{}, &ingressv1alpha1.HTTPSEdgeRouteSpec{}, false),
		Entry("Same Auth: Oauth", &ngrok.HTTPSEdgeRoute{OAuth: &ngrok.EndpointOAuth{}}, &ingressv1alpha1.HTTPSEdgeRouteSpec{OAuth: &ingressv1alpha1.EndpointOAuth{}}, false),
		Entry("Same Auth: SAML", &ngrok.HTTPSEdgeRoute{SAML: &ngrok.EndpointSAML{}}, &ingressv1alpha1.HTTPSEdgeRouteSpec{SAML: &ingressv1alpha1.EndpointSAML{}}, false),
		Entry("Same Auth: OIDC", &ngrok.HTTPSEdgeRoute{OIDC: &ngrok.EndpointOIDC{}}, &ingressv1alpha1.HTTPSEdgeRouteSpec{OIDC: &ingressv1alpha1.EndpointOIDC{}}, false),
		Entry("Added Oauth", &ngrok.HTTPSEdgeRoute{}, &ingressv1alpha1.HTTPSEdgeRouteSpec{OAuth: &ingressv1alpha1.EndpointOAuth{}}, false),
		Entry("Added SAML", &ngrok.HTTPSEdgeRoute{}, &ingressv1alpha1.HTTPSEdgeRouteSpec{SAML: &ingressv1alpha1.EndpointSAML{}}, false),
		Entry("Added OIDC", &ngrok.HTTPSEdgeRoute{}, &ingressv1alpha1.HTTPSEdgeRouteSpec{OIDC: &ingressv1alpha1.EndpointOIDC{}}, false),
		Entry("Removed Oauth", &ngrok.HTTPSEdgeRoute{OAuth: &ngrok.EndpointOAuth{}}, &ingressv1alpha1.HTTPSEdgeRouteSpec{}, false),
		Entry("Removed SAML", &ngrok.HTTPSEdgeRoute{SAML: &ngrok.EndpointSAML{}}, &ingressv1alpha1.HTTPSEdgeRouteSpec{}, false),
		Entry("Removed OIDC", &ngrok.HTTPSEdgeRoute{OIDC: &ngrok.EndpointOIDC{}}, &ingressv1alpha1.HTTPSEdgeRouteSpec{}, false),
		Entry("Removed Oauth and Added Saml", &ngrok.HTTPSEdgeRoute{OAuth: &ngrok.EndpointOAuth{}}, &ingressv1alpha1.HTTPSEdgeRouteSpec{SAML: &ingressv1alpha1.EndpointSAML{}}, true),
		Entry("Removed Oauth and Added OIDC", &ngrok.HTTPSEdgeRoute{OAuth: &ngrok.EndpointOAuth{}}, &ingressv1alpha1.HTTPSEdgeRouteSpec{OIDC: &ingressv1alpha1.EndpointOIDC{}}, true),
		Entry("Removed SAML and Added Oauth", &ngrok.HTTPSEdgeRoute{SAML: &ngrok.EndpointSAML{}}, &ingressv1alpha1.HTTPSEdgeRouteSpec{OAuth: &ingressv1alpha1.EndpointOAuth{}}, true),
		Entry("Removed SAML and Added OIDC", &ngrok.HTTPSEdgeRoute{SAML: &ngrok.EndpointSAML{}}, &ingressv1alpha1.HTTPSEdgeRouteSpec{OIDC: &ingressv1alpha1.EndpointOIDC{}}, true),
		Entry("Removed OIDC and Added Oauth", &ngrok.HTTPSEdgeRoute{OIDC: &ngrok.EndpointOIDC{}}, &ingressv1alpha1.HTTPSEdgeRouteSpec{OAuth: &ingressv1alpha1.EndpointOAuth{}}, true),
		Entry("Removed OIDC and Added SAML", &ngrok.HTTPSEdgeRoute{OIDC: &ngrok.EndpointOIDC{}}, &ingressv1alpha1.HTTPSEdgeRouteSpec{SAML: &ingressv1alpha1.EndpointSAML{}}, true),
	)
})
