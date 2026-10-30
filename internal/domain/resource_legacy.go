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

// LEGACY-domain-kind: delete this file at cleanup.

package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1alpha1 "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
)

// FromLegacy returns the Resource view of a deprecated
// ingress.k8s.ngrok.com/v1alpha1 Domain. Its status is bridged into the v1
// status type through JSON (the two are wire-identical); CommitStatus writes
// it back onto d.
func FromLegacy(d *ingressv1alpha1.Domain) (Resource, error) {
	status := &ngrokv1.DomainStatus{}
	if err := convertJSON(&d.Status, status); err != nil {
		return nil, fmt.Errorf("bridging status of legacy Domain %s/%s: %w", d.Namespace, d.Name, err)
	}
	return legacyResource{d: d, status: status}, nil
}

// legacyResource adapts a legacy Domain to Resource. status is a v1-typed copy
// of d.Status, made once by FromLegacy; it is a pointer so every copy of this
// value shares it. Changes made through Status() reach d only when
// CommitStatus runs, which writeStatus does right before each status write.
type legacyResource struct {
	d      *ingressv1alpha1.Domain
	status *ngrokv1.DomainStatus
}

func (r legacyResource) Object() client.Object { return r.d }
func (r legacyResource) Hostname() string      { return r.d.Spec.Domain }
func (r legacyResource) Description() string   { return r.d.Spec.Description }

// MetadataAPIString passes the raw legacy value through exactly as the
// controller always has, so upgrading never rewrites a reservation's metadata.
func (r legacyResource) MetadataAPIString() string {
	return commonv1alpha1.MetadataAPIString(r.d.Spec.Metadata)
}

func (r legacyResource) ResolvesTo() []ngrokv1.DomainResolvesToEntry {
	return FromLegacyResolvesTo(r.d.Spec.GetResolvesTo())
}

func (r legacyResource) ReclaimPolicy() ngrokv1.DomainReclaimPolicy {
	return ngrokv1.DomainReclaimPolicy(r.d.Spec.ReclaimPolicy)
}

func (r legacyResource) Status() *ngrokv1.DomainStatus { return r.status }

func (r legacyResource) CommitStatus() error {
	status := ingressv1alpha1.DomainStatus{}
	if err := convertJSON(r.status, &status); err != nil {
		return fmt.Errorf("bridging status of legacy Domain %s/%s: %w", r.d.Namespace, r.d.Name, err)
	}
	r.d.Status = status
	return nil
}

func (r legacyResource) IsLegacy() bool { return true }

// convertJSON copies src into dst through their JSON encoding.
func convertJSON(src, dst any) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

// ErrMigrationBlocked is returned by EnsureV1 when a legacy Domain exists at
// the key but can't be copied to v1 without changing its reservation: its
// spec.metadata is a string that isn't a flat JSON object of strings, which
// the v1 map type can't hold.
var ErrMigrationBlocked = errors.New("legacy Domain cannot be migrated to ngrok.com/v1 automatically")

// copyOfLegacy returns the v1 copy of the legacy Domain at key, or nil if there
// is none. It reads through r, which should bypass the cache: missing a legacy
// Domain here means EnsureV1 creates a defaults-only v1 Domain that would then
// overwrite the legacy Domain's settings on the shared reservation.
func copyOfLegacy(ctx context.Context, r client.Reader, key client.ObjectKey) (*ngrokv1.Domain, error) {
	legacy := &ingressv1alpha1.Domain{}
	if err := r.Get(ctx, key, legacy); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	return V1FromLegacy(legacy)
}

// V1FromLegacy builds the v1 copy of a legacy Domain: same name, namespace and
// spec. Labels and annotations are not copied: they belong to whatever applied
// the legacy object, and a GitOps tool's tracking label on an object it didn't
// create makes it prune that object. Callers add the operator's own labels.
func V1FromLegacy(legacy *ingressv1alpha1.Domain) (*ngrokv1.Domain, error) {
	metadata := commonv1alpha1.MetadataToMap(legacy.Spec.Metadata)
	// An unset or empty legacy value maps to nil, and v1 then gets the CRD
	// default. Any other value must survive as a map, or the copy would change
	// the metadata stored on the reservation.
	if metadata == nil && commonv1alpha1.MetadataAPIString(legacy.Spec.Metadata) != "" {
		return nil, fmt.Errorf("%w: %s/%s has spec.metadata %s, which is not a JSON object of strings; re-create it as ngrok.com/v1 by hand",
			ErrMigrationBlocked, legacy.Namespace, legacy.Name, string(legacy.Spec.Metadata))
	}

	v1 := &ngrokv1.Domain{}
	v1.Name = legacy.Name
	v1.Namespace = legacy.Namespace
	v1.Spec = ngrokv1.DomainSpec{
		Description:   legacy.Spec.Description,
		Metadata:      metadata,
		Domain:        legacy.Spec.Domain,
		ResolvesTo:    FromLegacyResolvesTo(legacy.Spec.ResolvesTo),
		ReclaimPolicy: ngrokv1.DomainReclaimPolicy(legacy.Spec.ReclaimPolicy),
	}
	return v1, nil
}

// FromLegacyResolvesTo converts legacy resolvesTo entries to the v1 type.
func FromLegacyResolvesTo(in []ingressv1alpha1.DomainResolvesToEntry) []ngrokv1.DomainResolvesToEntry {
	if in == nil {
		return nil
	}
	out := make([]ngrokv1.DomainResolvesToEntry, len(in))
	for i, e := range in {
		out[i] = ngrokv1.DomainResolvesToEntry{Value: e.Value}
	}
	return out
}
