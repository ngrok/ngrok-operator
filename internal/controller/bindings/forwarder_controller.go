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
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/mux"
	"github.com/ngrok/ngrok-operator/pkg/bindingsdriver"
	"golang.org/x/sync/errgroup"
	v1 "k8s.io/api/core/v1"
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

	controller *controller.BaseController[*bindingsv1alpha1.BoundEndpoint]
	Log        logr.Logger
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder

	BindingsDriver         *bindingsdriver.BindingsDriver
	KubernetesOperatorName string
}

func (r *ForwarderReconciler) SetupWithManager(mgr ctrl.Manager) (err error) {
	if r.BindingsDriver == nil {
		return fmt.Errorf("BindingsDriver is required")
	}

	if r.KubernetesOperatorName == "" {
		return fmt.Errorf("KubernetesOperatorName is required")
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

	// Get the KubernetesOperator

	op := ngrokv1alpha1.KubernetesOperator{}
	objectKey := client.ObjectKey{Name: r.KubernetesOperatorName, Namespace: epb.Namespace}
	if err := r.Client.Get(ctx, objectKey, &op); err != nil {
		return err
	}

	// Bindings should be enabled on the operator, if they aren't we can't do anything
	if op.Spec.Binding == nil {
		return fmt.Errorf("operator does not have binding configuration")
	}

	if op.Spec.Binding.IngressEndpoint == nil {
		return fmt.Errorf("operator binding configuration does not have an ingress endpoint")
	}

	// Get the secret
	secret := v1.Secret{}
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: op.Namespace, Name: op.Spec.Binding.TlsSecretName}, &secret); err != nil {
		return err
	}

	keyData, hasKey := secret.Data["tls.key"]
	certData, hasCert := secret.Data["tls.crt"]

	if !hasKey || !hasCert {
		return fmt.Errorf("missing tls.key or tls.crt")
	}

	cert, err := tls.X509KeyPair(certData, keyData)
	if err != nil {
		return err
	}

	tlsDialer := tls.Dialer{
		NetDialer: &net.Dialer{
			Timeout: 3 * time.Minute,
		},
		Config: &tls.Config{
			Certificates: []tls.Certificate{cert},
		},
	}

	endpointURI, err := url.Parse(epb.Spec.EndpointURI)
	if err != nil {
		return err
	}

	host := endpointURI.Hostname()
	port, err := strconv.Atoi(endpointURI.Port())
	if err != nil {
		return err
	}

	cnxnHandler := func(conn net.Conn) error {
		connReader := connReaderForEndpoint(epb, conn)
		defer connReader.Close()

		log := log.WithValues(
			"remoteAddr", connReader.RemoteAddr(),
			"ingress", map[string]string{
				"endpoint": *op.Spec.Binding.IngressEndpoint,
			},
			"binding", map[string]string{
				"host": host,
				"port": endpointURI.Port(),
			},
		)

		log.Info("Handling connnection")

		ngrokConn, err := tlsDialer.Dial("tcp", *op.Spec.Binding.IngressEndpoint)
		if err != nil {
			log.Error(err, "failed to dial ingress endpoint")
			return err
		}

		// Upgrade the connection to a binding connection
		resp, err := mux.UpgradeToBindingConnection(log, ngrokConn, host, port)
		log = log.WithValues("endpoint.id", resp.EndpointID, "proto", resp.Proto)
		if err != nil {
			log.Error(err, "failed to upgrade connection")
			return err
		}

		if resp.ErrorCode != "" || resp.ErrorMessage != "" {
			err := fmt.Errorf("%s: %s", resp.ErrorCode, resp.ErrorMessage)
			log.Error(err, "failed to upgrade connection", "errorCode", resp.ErrorCode, "errorMessage", resp.ErrorMessage)
			return err
		}

		log.Info("Bound connection")
		return joinConnections(log, connReader, ngrokConn)
	}

	log.Info("Listening on port")

	return r.BindingsDriver.Listen(int32(epb.Spec.Port), cnxnHandler)
}

// connReaderForEndpoint returns an io.Reader that will read from the net.Conn
// this is used in joinConnections()
func connReaderForEndpoint(ep *bindingsv1alpha1.BoundEndpoint, conn net.Conn) net.Conn {
	endpointURI, err := url.Parse(ep.Spec.EndpointURI)
	if err != nil {
		return conn
	}

	host := endpointURI.Hostname()
	scheme := endpointURI.Scheme

	switch {
	// http protocols are wrapped in Host header rewrites
	case strings.HasPrefix(scheme, "http"):
		// TODO(hkatz) Support HTTP/2 and HTTP/3 psuedo header `:authority`
		wrapped := newConnInterceptor(
			conn,
			// matches
			func(line string) bool {
				return strings.HasPrefix(line, "Host:")
			},
			// desired
			func(line string) string {
				return fmt.Sprintf("Host: %s", host)
			},
		)
		return wrapped
	default:
		return conn
	}

	// unreachable
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

func joinConnections(log logr.Logger, conn1, conn2 net.Conn) error {
	var g errgroup.Group
	g.Go(func() error {
		defer func() {
			if err := conn1.Close(); err != nil {
				log.Error(err, "failed closing connection to destination: %v", err)
			}
		}()

		_, err := io.Copy(conn1, conn2)
		return err
	})
	g.Go(func() error {
		defer func() {
			if err := conn2.Close(); err != nil {
				log.Error(err, "failed closing connection to client: %v", err)
			}
		}()

		_, err := io.Copy(conn2, conn1)
		return err
	})
	return g.Wait()
}

// connInterceptor is a net.Conn that intercepts a net.Conn.Read() and replaces the expected string with the desired string
type connInterceptor struct {
	// original conn
	net.Conn

	// piped connections
	reader *io.PipeReader
	writer *io.PipeWriter

	// if line matches, then replace
	fnMatches func(string) bool

	// replacement function
	fnReplace func(string) string
}

func newConnInterceptor(conn net.Conn, fnMatches func(string) bool, fnReplace func(string) string) *connInterceptor {
	pipeReader, pipeWriter := io.Pipe()

	i := &connInterceptor{
		Conn:      conn,
		fnMatches: fnMatches,
		fnReplace: fnReplace,
		reader:    pipeReader,
		writer:    pipeWriter,
	}

	go i.intercept()

	return i
}

func (i *connInterceptor) Read(b []byte) (n int, err error) {
	return i.reader.Read(b)
}

// runs in goroutine
func (i *connInterceptor) intercept() {
	defer i.writer.Close()

	buffer := bufio.NewReaderSize(i.Conn, 1024)

	for {
		line, err := buffer.ReadString('\n')
		if err != nil { // including io.EOF
			break
		}

		if i.fnMatches(line) {
			want := i.fnReplace(line)
			_, _ = i.writer.Write([]byte(want + "\r\n"))
			break
		}

		_, _ = i.writer.Write([]byte(line))
	}

	// finish the connection by copying the remaining data
	_, _ = io.Copy(i.writer, buffer)
}
