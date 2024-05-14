package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
)

type oauthProvider interface {
	Provided() bool
}

func TestOAuthCommonProvided(t *testing.T) {
	oauth := EndpointOAuth{}

	providers := []oauthProvider{
		oauth.Amazon,
		oauth.Facebook,
		oauth.Github,
		oauth.Gitlab,
		oauth.Google,
		oauth.Linkedin,
		oauth.Microsoft,
		oauth.Twitch,
	}

	// When no provider config is present, all should return false for Provided()
	for _, p := range providers {
		assert.False(t, p.Provided())
	}

	microsoft := &EndpointOAuthMicrosoft{}
	microsoft.ClientID = ptr.To("a")

	oauth = EndpointOAuth{
		Microsoft: microsoft,
	}
	// When a provider is present, it should return true for Provided() and ones
	// that are not provided should return false.
	assert.True(t, oauth.Microsoft.Provided())
	assert.False(t, oauth.Google.Provided())
	assert.False(t, oauth.Twitch.Provided())
	assert.False(t, oauth.Github.Provided())
}
