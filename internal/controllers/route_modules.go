package controllers

/*
TODO: this all should be replaced.  This is a quick and dirty implementation for two HTTPS Route Modules:
- "compresison"
- "oauth", but only google oauth
*/

import (
	"context"
	"fmt"
	"strings"

	"github.com/ngrok/ngrok-ingress-controller/pkg/ngrokapidriver"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func annotationsToCompression(annotations map[string]string) bool {
	if value, found := annotations["k8s.ngrok.com/https-compression"]; found && value == "true" {
		return true
	}
	return false
}

func (irec *IngressReconciler) annotationsToOauth(ctx context.Context, annotations map[string]string) (*ngrokapidriver.OAuthGoogle, error) {
	googleOauth := ngrokapidriver.OAuthGoogle{}

	// Must have a provider and it must be "google"
	if provider, found := annotations["k8s.ngrok.com/https-oauth.provider"]; !found {
		return nil, nil
	} else {
		if provider != "google" {
			return nil, fmt.Errorf("currently the ingress controller only support \"google\" but %q was provided", provider)
		}
	}

	// Must have a secret-name so we can configure ClientID and ClientSecret
	if secretName, found := annotations["k8s.ngrok.com/https-oauth.secret-name"]; !found {
		return nil, fmt.Errorf("no \"k8s.ngrok.com/https-oauth.secret-name\", this is required to configure ClientID and ClientSecret")
	} else {
		secret := v1.Secret{}
		err := irec.Get(ctx, client.ObjectKey{
			Namespace: irec.Namespace,
			Name:      secretName,
		}, &secret)
		if err != nil {
			return nil, err
		}

		if clientId, found := secret.Data["ClientID"]; found {
			googleOauth.ClientID = string(clientId)
		} else {
			return nil, fmt.Errorf("failed to retrived \"ClientID\" from secret %q", secretName)
		}

		if clientSecret, found := secret.Data["ClientSecret"]; found {
			googleOauth.ClientSecret = string(clientSecret)
		} else {
			return nil, fmt.Errorf("failed to retrived \"ClientSecret\" from secret %q", secretName)
		}
	}

	if scopesStr, found := annotations["k8s.ngrok.com/https-oauth.scopes"]; found {
		googleOauth.Scopes = strings.Split(scopesStr, ",")
	}

	if allowDomains, found := annotations["k8s.ngrok.com/https-oauth.allow-domains"]; found {
		googleOauth.EmailDomains = strings.Split(allowDomains, ",")
	}

	return &googleOauth, nil
}
