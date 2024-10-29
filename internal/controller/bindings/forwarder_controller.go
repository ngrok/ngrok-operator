/*
MIT License

Copyright (c) 2024 ngrok, Inc.

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

package bindings

import (
	"bufio"
	"context"
	"fmt"
	"net"

	"github.com/go-logr/logr"
	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/pkg/bindingsdriver"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controllerruntime "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type ForwarderReconciler struct {
	client.Client

	Log            logr.Logger
	Scheme         *runtime.Scheme
	Recorder       record.EventRecorder
	BindingsDriver *bindingsdriver.BindingsDriver

	controller *controller.BaseController[*bindingsv1alpha1.BoundEndpoint]
}

func (r *ForwarderReconciler) SetupWithManager(mgr ctrl.Manager) (err error) {
	if r.BindingsDriver == nil {
		return fmt.Errorf("BindingsDriver is required")
	}

	r.controller = &controller.BaseController[*bindingsv1alpha1.BoundEndpoint]{
		Kube:     r.Client,
		Log:      r.Log,
		Recorder: r.Recorder,

		Update:   r.update,
		Delete:   r.delete,
		StatusID: r.statusID,
	}

	cont, err := controllerruntime.NewUnmanaged("bindings-forwarder-controller", mgr, controllerruntime.Options{
		Reconciler: r,
		LogConstructor: func(_ *reconcile.Request) logr.Logger {
			return r.Log
		},
		NeedLeaderElection: ptr.To(false),
	})
	if err != nil {
		return
	}

	err = cont.Watch(
		source.Kind(mgr.GetCache(), &bindingsv1alpha1.BoundEndpoint{}),
		&handler.EnqueueRequestForObject{},
		predicate.Or(
			predicate.AnnotationChangedPredicate{},
			predicate.GenerationChangedPredicate{},
		),
	)
	if err != nil {
		return
	}

	err = mgr.Add(cont)
	return
}

func (r *ForwarderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.controller.Reconcile(ctx, req, new(bindingsv1alpha1.BoundEndpoint))
}

func (r *ForwarderReconciler) update(ctx context.Context, epb *bindingsv1alpha1.BoundEndpoint) error {
	log := ctrl.LoggerFrom(ctx).WithValues(
		"endpoint-binding", map[string]string{
			"namespace": epb.Namespace,
			"name":      epb.Name,
		},
		"port", epb.Spec.Port,
	)

	log.Info("Listening on port")
	port := int32(epb.Spec.Port)

	// TODO: wire this up to ngrok. For now, we'll just use the example connection handler
	return r.BindingsDriver.Listen(port, exampleConnectionHandler(epb))
}

func (r *ForwarderReconciler) delete(ctx context.Context, epb *bindingsv1alpha1.BoundEndpoint) error {
	port := int32(epb.Spec.Port)
	r.BindingsDriver.Close(port)
	return nil
}

// Always returns the endpoint binding's "namespace/name". This is different than most of our other
// controllers which return a .Status.ID field. We do this to always trigger the update handler of
// the base controller.
func (r *ForwarderReconciler) statusID(epb *bindingsv1alpha1.BoundEndpoint) string {
	return fmt.Sprintf("%s/%s", epb.Namespace, epb.Name)
}

// exampleConnectionHandler is a simple example of a connection handler that echos back each line
// it reads from the client. It also sends a welcome message to the client.
func exampleConnectionHandler(epb *bindingsv1alpha1.BoundEndpoint) bindingsdriver.ConnectionHandler {
	return func(conn net.Conn) error {
		defer conn.Close()
		_, err := conn.Write([]byte(
			fmt.Sprintf(
				`
Hello from the ngrok-operator bindings-forwarder

You are connected to my port: %d
The endpoint binding is: %s/%s

Type anything and hit enter to see it echoed back
Type an empty line to close the connection

>`, epb.Spec.Port, epb.Namespace, epb.Name,
			),
		))
		if err != nil {
			return err
		}
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" { // empty line means the client is done
				break
			}

			_, err := conn.Write([]byte(line + "\n> "))
			if err != nil {
				return err
			}
		}
		return nil
	}
}
