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
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type State interface {
	IsDraining(ctx context.Context) bool
}

type StateChecker struct {
	client              client.Client
	controllerNamespace string
	controllerName      string

	mu       sync.RWMutex
	draining bool
}

func NewStateChecker(c client.Client, controllerNamespace, controllerName string) *StateChecker {
	return &StateChecker{
		client:              c,
		controllerNamespace: controllerNamespace,
		controllerName:      controllerName,
	}
}

func (s *StateChecker) IsDraining(ctx context.Context) bool {
	s.mu.RLock()
	if s.draining {
		s.mu.RUnlock()
		return true
	}
	s.mu.RUnlock()

	list := &ngrokv1alpha1.KubernetesOperatorList{}
	if err := s.client.List(ctx, list, client.InNamespace(s.controllerNamespace)); err != nil {
		return false
	}

	for _, op := range list.Items {
		if op.Spec.Deployment == nil {
			continue
		}
		if op.Spec.Deployment.Name != s.controllerName || op.Spec.Deployment.Namespace != s.controllerNamespace {
			continue
		}

		isDraining := !op.DeletionTimestamp.IsZero() ||
			op.Spec.DrainMode ||
			op.Status.DrainStatus == ngrokv1alpha1.DrainStatusDraining

		if isDraining {
			s.mu.Lock()
			s.draining = true
			s.mu.Unlock()
		}
		return isDraining
	}

	return false
}

func (s *StateChecker) SetDraining(draining bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.draining = draining
}

type NeverDraining struct{}

var _ State = NeverDraining{}

func (NeverDraining) IsDraining(context.Context) bool { return false }

type AlwaysDraining struct{}

var _ State = AlwaysDraining{}

func (AlwaysDraining) IsDraining(context.Context) bool { return true }
