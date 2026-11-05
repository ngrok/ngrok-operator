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

package cloudendpoint

import (
	"context"
	"fmt"

	"github.com/ngrok/ngrok-api-go/v9"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/events"

	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
	trafficpolicypkg "github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	"github.com/ngrok/ngrok-operator/internal/util"
)

// Syncer pushes one CloudEndpoint's spec to the ngrok API. It works on either
// kind through Resource and only changes the object in memory (status ID,
// assigned URL, domain ref, conditions). Writing the status back is the
// caller's job.
type Syncer struct {
	Endpoints       ngrokapi.EndpointsClient
	Domains         *domainpkg.Manager
	TrafficPolicies *trafficpolicypkg.Manager
	Recorder        events.EventRecorder
}

// Sync ensures r's domain exists, resolves its traffic policy, and creates
// the remote endpoint if r has no ID yet (or the remote one has disappeared),
// otherwise updates it when it has drifted. The returned error is the one to
// hand to the status write: the sync failure if any, else
// domainpkg.ErrDomainNotReady while the domain is not ready.
func (s *Syncer) Sync(ctx context.Context, r Resource) error {
	domainResult, err := s.Domains.EnsureDomainExists(ctx, r)
	if err != nil {
		return finish(r, nil, domainResult, err)
	}

	tpResult, err := s.TrafficPolicies.Resolve(ctx, r)
	if err != nil {
		return finish(r, nil, domainResult, err)
	}
	policy := tpResult.Policy

	if r.GetID() == "" {
		return s.create(ctx, r, domainResult, policy)
	}

	// Fetch current endpoint state from the ngrok API so we can compare
	// before issuing an update. This avoids redundant API writes on every
	// requeue cycle (e.g. while waiting for a domain to become ready).
	current, err := s.Endpoints.Get(ctx, r.GetID())
	if ngrok.IsNotFound(err) {
		s.Recorder.Eventf(r, nil, v1.EventTypeWarning, "EndpointNotFound", "Reconcile", fmt.Sprintf("Failed to find endpoint %s by ID. Creating a new one", r.GetID()))
		r.SetID("")
		r.SetAssignedURL("")
		return s.create(ctx, r, domainResult, policy)
	}
	if err != nil {
		return finish(r, nil, domainResult, err)
	}

	if !NeedsUpdate(current, r, policy) {
		return s.recordWriteSuccess(r, current, domainResult, "CloudEndpoint updated successfully")
	}

	metadata := r.GetMetadataAPIString()
	updated, err := s.Endpoints.Update(ctx, &ngrok.EndpointUpdate{
		ID:             r.GetID(),
		Url:            new(r.GetURL()),
		Description:    new(r.GetDescription()),
		Metadata:       &metadata,
		TrafficPolicy:  &policy,
		Bindings:       util.NilIfEmpty(r.GetBindings()),
		PoolingEnabled: r.GetPoolingEnabled(),
	})
	if err != nil {
		return s.recordWriteError(r, domainResult, policy, err, fmt.Sprintf("Failed to update cloud endpoint: %v", err))
	}

	return s.recordWriteSuccess(r, updated, domainResult, "CloudEndpoint updated successfully")
}

// Delete deletes r's remote endpoint. A remote endpoint that is already gone
// counts as deleted. On success, r's ID is cleared.
func (s *Syncer) Delete(ctx context.Context, r Resource) error {
	err := s.Endpoints.Delete(ctx, r.GetID())
	if err == nil || ngrok.IsNotFound(err) {
		r.SetID("")
	}
	return err
}

func (s *Syncer) create(ctx context.Context, r Resource, domainResult *domainpkg.DomainResult, policy string) error {
	metadata := r.GetMetadataAPIString()
	created, err := s.Endpoints.Create(ctx, &ngrok.EndpointCreate{
		Type:           "cloud",
		URL:            r.GetURL(),
		Description:    new(r.GetDescription()),
		Metadata:       &metadata,
		TrafficPolicy:  policy,
		Bindings:       util.NilIfEmpty(r.GetBindings()),
		PoolingEnabled: r.GetPoolingEnabled(),
	})
	if err != nil {
		return s.recordWriteError(r, domainResult, policy, err, fmt.Sprintf("Failed to create cloud endpoint: %v", err))
	}

	return s.recordWriteSuccess(r, created, domainResult, "CloudEndpoint created successfully")
}

// recordWriteSuccess marks the resolved traffic policy as applied and the
// endpoint as created. Called after a create/update (or a skipped no-op
// update) succeeds.
func (s *Syncer) recordWriteSuccess(r Resource, remote *ngrok.Endpoint, domainResult *domainpkg.DomainResult, message string) error {
	s.TrafficPolicies.MarkApplied(r)
	SetCreatedCondition(r, true, ReasonCloudEndpointCreated, message)
	return finish(r, remote, domainResult, nil)
}

// recordWriteError marks the endpoint creation as failed and, when the ngrok
// API rejected the request because of the policy itself, surfaces that on the
// TrafficPolicyApplied condition.
func (s *Syncer) recordWriteError(r Resource, domainResult *domainpkg.DomainResult, policy string, err error, message string) error {
	SetCreatedCondition(r, false, ReasonCloudEndpointCreationFailed, message)
	if policy != "" && ngrokapi.IsTrafficPolicyError(err.Error()) {
		s.TrafficPolicies.SetError(r, ngrokapi.SanitizeErrorMessage(err.Error()))
	}
	return finish(r, nil, domainResult, err)
}

// finish records the remote endpoint (if any) on r, recalculates Ready, and
// returns the error the caller should write status with and return.
func finish(r Resource, remote *ngrok.Endpoint, domainResult *domainpkg.DomainResult, err error) error {
	if remote != nil {
		r.SetID(remote.ID)
		r.SetAssignedURL(remote.URL)
	}

	CalculateReadyCondition(r, domainResult)

	if err != nil {
		return err
	}
	// Requeue if domain is not ready (fallback to watch for convergence)
	if domainResult != nil {
		return domainResult.RequeueError()
	}
	return nil
}
