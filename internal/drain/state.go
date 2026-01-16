/*
MIT License

Copyright (c) 2025 ngrok, Inc.

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
package drain

import (
	"context"
	"sync"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/drainstate"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Re-export drainstate types for convenience
type State = drainstate.State

var (
	IsDraining    = drainstate.IsDraining
	NeverDraining = drainstate.NeverDraining{}
)

// Compile-time check that StateChecker implements drainstate.State
var _ drainstate.State = (*StateChecker)(nil)

// StateChecker checks if the operator is in drain mode by looking up the KubernetesOperator CR.
// It caches the draining state - once draining is detected, it stays true (never resets).
type StateChecker struct {
	client             client.Client
	operatorNamespace  string
	operatorConfigName string // The KubernetesOperator CR name (release name)

	mu       sync.RWMutex
	draining bool
}

// NewStateChecker creates a StateChecker that looks up the KubernetesOperator CR by name.
// operatorNamespace is the namespace where the operator is deployed.
// operatorConfigName is the name of the KubernetesOperator CR (typically the Helm release name).
func NewStateChecker(c client.Client, operatorNamespace, operatorConfigName string) *StateChecker {
	return &StateChecker{
		client:             c,
		operatorNamespace:  operatorNamespace,
		operatorConfigName: operatorConfigName,
	}
}

func (s *StateChecker) IsDraining(ctx context.Context) bool {
	// Fast path: already draining (cached)
	s.mu.RLock()
	if s.draining {
		s.mu.RUnlock()
		return true
	}
	s.mu.RUnlock()

	// Query the specific KubernetesOperator CR by name
	ko := &ngrokv1alpha1.KubernetesOperator{}
	if err := s.client.Get(ctx, types.NamespacedName{
		Namespace: s.operatorNamespace,
		Name:      s.operatorConfigName,
	}, ko); err != nil {
		// If CR doesn't exist or error, assume not draining
		return false
	}

	isDraining := !ko.DeletionTimestamp.IsZero() ||
		(ko.Spec.Drain != nil && ko.Spec.Drain.Enabled) ||
		ko.Status.DrainStatus == ngrokv1alpha1.DrainStatusDraining

	if isDraining {
		s.mu.Lock()
		s.draining = true
		s.mu.Unlock()
	}
	return isDraining
}

func (s *StateChecker) SetDraining(draining bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.draining = draining
}
