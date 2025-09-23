package util

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
)

// ErrDomainCreating is returned when a domain is being created and the caller should requeue
var ErrDomainCreating = errors.New("domain is being created, requeue after delay")

// IsDomainReady checks if a domain is ready by examining its conditions
func IsDomainReady(domain *ingressv1alpha1.Domain) bool {
	readyCondition := meta.FindStatusCondition(domain.Status.Conditions, "Ready")
	return readyCondition != nil && readyCondition.Status == metav1.ConditionTrue
}

// EnsureDomainExists checks if the Domain CRD exists for the given URL, and if not, creates it.
// Returns the domain object and any error. Callers should set their own conditions based on results.
func EnsureDomainExists(ctx context.Context, kube client.Client, recorder record.EventRecorder, 
	owner client.Object, urlStr string, defaultReclaimPolicy *ingressv1alpha1.DomainReclaimPolicy) (*ingressv1alpha1.Domain, error) {
	
	parsedURL, err := ParseAndSanitizeEndpointURL(urlStr, true)
	if err != nil {
		recorder.Event(owner, "Warning", "InvalidURL", fmt.Sprintf("Failed to parse URL: %s", urlStr))
		return nil, fmt.Errorf("failed to parse URL %q from %s \"%s.%s\"", urlStr, owner.GetObjectKind().GroupVersionKind().Kind, owner.GetName(), owner.GetNamespace())
	}

	// Skip creating the Domain CR for ngrok TCP URLs
	if parsedURL.Scheme == "tcp" && strings.HasSuffix(parsedURL.Hostname(), "tcp.ngrok.io") {
		return nil, nil
	}

	domain := parsedURL.Hostname()
	hyphenatedDomain := ingressv1alpha1.HyphenatedDomainNameFromURL(domain)
	
	// Skip creating the Domain CRD for reserved TLDs
	if domainEndsInReservedTLD(domain) {
		return nil, nil
	}

	log := ctrl.LoggerFrom(ctx).WithValues("domain", domain)

	// Check if the Domain CRD already exists
	domainObj := &ingressv1alpha1.Domain{}
	err = kube.Get(ctx, client.ObjectKey{Name: hyphenatedDomain, Namespace: owner.GetNamespace()}, domainObj)
	if err == nil {
		// Domain already exists - check if it's ready using conditions
		if IsDomainReady(domainObj) {
			return domainObj, nil
		} else {
			// Domain is not ready yet
			return domainObj, ErrDomainCreating
		}
	}
	if client.IgnoreNotFound(err) != nil {
		// Some other error occurred
		log.Error(err, "failed to check Domain CRD existence")
		return nil, err
	}

	// Create the Domain CRD since it doesn't exist
	newDomain := &ingressv1alpha1.Domain{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      hyphenatedDomain,
			Namespace: owner.GetNamespace(),
		},
		Spec: ingressv1alpha1.DomainSpec{
			Domain: domain,
		},
	}

	if defaultReclaimPolicy != nil {
		newDomain.Spec.ReclaimPolicy = *defaultReclaimPolicy
	}

	if err := kube.Create(ctx, newDomain); err != nil {
		recorder.Event(owner, "Warning", "DomainCreationFailed", fmt.Sprintf("Failed to create Domain CRD %s", hyphenatedDomain))
		return newDomain, err
	}

	recorder.Event(owner, "Normal", "DomainCreated", fmt.Sprintf("Domain CRD %s created successfully", hyphenatedDomain))
	return newDomain, ErrDomainCreating
}

// domainEndsInReservedTLD checks if the domain ends in a reserved TLD (e.g., ".internal") in
// order to filter it out of lists of domains to create automatically.
func domainEndsInReservedTLD(domain string) bool {
	// Check if the domain ends in the "internal" tld
	return strings.HasSuffix(domain, ".internal")
}
