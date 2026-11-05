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

// Package cloudendpoint holds the kind-agnostic reconcile logic shared by the
// canonical ngrok.com/v1 CloudEndpoint and the deprecated
// ngrok.k8s.ngrok.com/v1alpha1 CloudEndpoint, following the same
// LEGACY-KIND-MIGRATION pattern as internal/ippolicy (tagged
// LEGACY-cloudendpoint-kind; see docs/developer-guide/passivity-shims.md).
package cloudendpoint

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
)

// Resource is the kind-agnostic view of a CloudEndpoint custom resource. Both
// kinds satisfy it, so the reconcile logic is written once against it.
//
// LEGACY-cloudendpoint-kind: this interface outlives the migration. At
// cleanup only the legacy assertion below and the accessor methods on that
// type go away.
type Resource interface {
	client.Object
	ngrokv1.EndpointWithDomain
	ngrokv1.EndpointWithTrafficPolicy

	// GetID returns the ngrok API ID assigned to this endpoint, if any.
	GetID() string
	// SetID records the ngrok API ID assigned to this endpoint.
	SetID(string)
	// SetAssignedURL records the URL the ngrok API assigned to this endpoint.
	SetAssignedURL(string)
	// GetDescription returns the human-readable description from the spec.
	GetDescription() string
	// GetMetadataAPIString returns the spec metadata encoded as the string
	// the ngrok API stores. Each kind keeps the encoding the operator has
	// always produced for it; compare with MetadataEqual, not ==.
	GetMetadataAPIString() string
	// GetPoolingEnabled returns the spec pooling setting; nil means unset.
	GetPoolingEnabled() *bool
	// ResetStatus clears the whole status.
	ResetStatus()
}

// ResourcePtr constrains PT to *T where *T is a Resource, so generic code can
// allocate a fresh T with new(T) and use it as a Resource.
type ResourcePtr[T any] interface {
	*T
	Resource
}

var (
	_ Resource = (*ngrokv1.CloudEndpoint)(nil)
	// LEGACY-cloudendpoint-kind: drop this assertion at cleanup.
	_ Resource = (*ngrokv1alpha1.CloudEndpoint)(nil)
)
