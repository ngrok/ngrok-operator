/*
MIT License

Copyright (c) 2026 ngrok, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package domain

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1alpha1 "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
)

// Resource is the kind-agnostic view of a Domain custom resource. Both the
// canonical ngrok.com/v1 Domain and the deprecated
// ingress.k8s.ngrok.com/v1alpha1 Domain are served during the migration
// window, and everything that reconciles or reads a Domain is written once
// against this view.
//
// The two kinds are wire-identical except spec.metadata, so rather than
// mirror every status field behind an accessor, Status always returns the v1
// status type. For the legacy kind it is a bridged copy; CommitStatus writes
// it back onto the object before a status update.
//
// LEGACY-domain-kind: this interface outlives the migration. At cleanup only
// the legacy adapter (resource_legacy.go) goes away.
type Resource interface {
	// Object returns the underlying custom resource, for client calls,
	// events, finalizers and logging.
	Object() client.Object

	// Hostname returns spec.domain.
	Hostname() string

	// Description returns spec.description.
	Description() string

	// MetadataAPIString returns spec.metadata encoded as the string the ngrok
	// API stores. This is the only form reconcilers send to, or compare with,
	// the API, so each kind keeps exactly the encoding the operator has always
	// produced for it.
	MetadataAPIString() string

	// ResolvesTo returns spec.resolvesTo.
	ResolvesTo() []ngrokv1.DomainResolvesToEntry

	// ReclaimPolicy returns spec.reclaimPolicy.
	ReclaimPolicy() ngrokv1.DomainReclaimPolicy

	// Status returns the status to read and mutate in place.
	Status() *ngrokv1.DomainStatus

	// CommitStatus copies the status returned by Status onto the underlying
	// object. Call it before writing status.
	CommitStatus() error

	// IsLegacy reports whether this is the deprecated
	// ingress.k8s.ngrok.com/v1alpha1 kind.
	//
	// LEGACY-domain-kind: delete at cleanup.
	IsLegacy() bool
}

// FromV1 returns the Resource view of a canonical Domain. Mutations through
// Status apply to d directly.
func FromV1(d *ngrokv1.Domain) Resource {
	return v1Resource{d: d}
}

type v1Resource struct {
	d *ngrokv1.Domain
}

func (r v1Resource) Object() client.Object { return r.d }
func (r v1Resource) Hostname() string      { return r.d.Spec.Domain }
func (r v1Resource) Description() string   { return r.d.Spec.Description }
func (r v1Resource) MetadataAPIString() string {
	return commonv1alpha1.MetadataAPIString(commonv1alpha1.MetadataFromMap(r.d.Spec.Metadata))
}
func (r v1Resource) ResolvesTo() []ngrokv1.DomainResolvesToEntry { return r.d.Spec.ResolvesTo }
func (r v1Resource) ReclaimPolicy() ngrokv1.DomainReclaimPolicy  { return r.d.Spec.ReclaimPolicy }
func (r v1Resource) Status() *ngrokv1.DomainStatus               { return &r.d.Status }
func (r v1Resource) CommitStatus() error                         { return nil }
func (r v1Resource) IsLegacy() bool                              { return false }

// IsBacked reports whether the Domain is backed by a reservation: its own
// (status.id) or the wildcard parent's it is covered by.
func IsBacked(r Resource) bool {
	s := r.Status()
	return s.ID != "" || s.CoveredByWildcardDomain != nil
}
