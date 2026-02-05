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
package controller

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v7"
	"github.com/ngrok/ngrok-operator/internal/drain"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
	"github.com/ngrok/ngrok-operator/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBaseController_Reconcile_ObjectNotFound(t *testing.T) {
	ctx := context.Background()
	s := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(s))

	c := fake.NewClientBuilder().WithScheme(s).Build()

	bc := &BaseController[*netv1.Ingress]{
		Kube:     c,
		Log:      logr.Discard(),
		Recorder: record.NewFakeRecorder(10),
		StatusID: func(_ *netv1.Ingress) string { return "" },
	}

	result, err := bc.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"},
	}, &netv1.Ingress{})

	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestBaseController_Reconcile_DrainState(t *testing.T) {
	ctx := context.Background()
	s := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(s))

	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "default",
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(ingress).Build()

	createCalled := false
	bc := &BaseController[*netv1.Ingress]{
		Kube:       c,
		Log:        logr.Discard(),
		Recorder:   record.NewFakeRecorder(10),
		DrainState: drain.AlwaysDraining{},
		StatusID:   func(_ *netv1.Ingress) string { return "" },
		Create: func(_ context.Context, _ *netv1.Ingress) error {
			createCalled = true
			return nil
		},
	}

	result, err := bc.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-ingress", Namespace: "default"},
	}, &netv1.Ingress{})

	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, createCalled, "Create should not be called when draining")
}

func TestBaseController_Reconcile_DrainState_AllowsDelete(t *testing.T) {
	ctx := context.Background()
	s := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(s))

	now := metav1.NewTime(time.Now())
	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-ingress",
			Namespace:         "default",
			DeletionTimestamp: &now,
			Finalizers:        []string{util.FinalizerName},
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(ingress).Build()

	deleteCalled := false
	bc := &BaseController[*netv1.Ingress]{
		Kube:       c,
		Log:        logr.Discard(),
		Recorder:   record.NewFakeRecorder(10),
		DrainState: drain.AlwaysDraining{},
		StatusID:   func(_ *netv1.Ingress) string { return "existing-id" },
		Delete: func(_ context.Context, _ *netv1.Ingress) error {
			deleteCalled = true
			return nil
		},
	}

	result, err := bc.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-ingress", Namespace: "default"},
	}, &netv1.Ingress{})

	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, deleteCalled, "Delete should be called even when draining")
}

func TestBaseController_Reconcile_Create(t *testing.T) {
	ctx := context.Background()
	s := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(s))

	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "default",
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(ingress).Build()

	createCalled := false
	bc := &BaseController[*netv1.Ingress]{
		Kube:     c,
		Log:      logr.Discard(),
		Recorder: record.NewFakeRecorder(10),
		StatusID: func(_ *netv1.Ingress) string { return "" },
		Create: func(_ context.Context, _ *netv1.Ingress) error {
			createCalled = true
			return nil
		},
		Update: func(_ context.Context, _ *netv1.Ingress) error {
			t.Error("Update should not be called")
			return nil
		},
	}

	result, err := bc.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-ingress", Namespace: "default"},
	}, &netv1.Ingress{})

	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, createCalled, "Create should be called when StatusID returns empty")
}

func TestBaseController_Reconcile_Update(t *testing.T) {
	ctx := context.Background()
	s := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(s))

	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "default",
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(ingress).Build()

	updateCalled := false
	bc := &BaseController[*netv1.Ingress]{
		Kube:     c,
		Log:      logr.Discard(),
		Recorder: record.NewFakeRecorder(10),
		StatusID: func(_ *netv1.Ingress) string { return "existing-id" },
		Create: func(_ context.Context, _ *netv1.Ingress) error {
			t.Error("Create should not be called")
			return nil
		},
		Update: func(_ context.Context, _ *netv1.Ingress) error {
			updateCalled = true
			return nil
		},
	}

	result, err := bc.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-ingress", Namespace: "default"},
	}, &netv1.Ingress{})

	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, updateCalled, "Update should be called when StatusID returns non-empty")
}

func TestBaseController_Reconcile_Delete(t *testing.T) {
	ctx := context.Background()
	s := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(s))

	now := metav1.NewTime(time.Now())
	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-ingress",
			Namespace:         "default",
			DeletionTimestamp: &now,
			Finalizers:        []string{util.FinalizerName},
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(ingress).Build()

	deleteCalled := false
	bc := &BaseController[*netv1.Ingress]{
		Kube:     c,
		Log:      logr.Discard(),
		Recorder: record.NewFakeRecorder(10),
		StatusID: func(_ *netv1.Ingress) string { return "existing-id" },
		Delete: func(_ context.Context, _ *netv1.Ingress) error {
			deleteCalled = true
			return nil
		},
	}

	result, err := bc.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-ingress", Namespace: "default"},
	}, &netv1.Ingress{})

	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, deleteCalled, "Delete should be called when object has finalizer and deletion timestamp")
}

func TestBaseController_Reconcile_Delete_NotFound(t *testing.T) {
	ctx := context.Background()
	s := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(s))

	now := metav1.NewTime(time.Now())
	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-ingress",
			Namespace:         "default",
			DeletionTimestamp: &now,
			Finalizers:        []string{util.FinalizerName},
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(ingress).Build()

	bc := &BaseController[*netv1.Ingress]{
		Kube:     c,
		Log:      logr.Discard(),
		Recorder: record.NewFakeRecorder(10),
		StatusID: func(_ *netv1.Ingress) string { return "existing-id" },
		Delete: func(_ context.Context, _ *netv1.Ingress) error {
			return &ngrok.Error{
				Msg:        "not found",
				StatusCode: http.StatusNotFound,
			}
		},
	}

	result, err := bc.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-ingress", Namespace: "default"},
	}, &netv1.Ingress{})

	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestCtrlResultForErr(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantResult ctrl.Result
		wantErr    bool
	}{
		{
			name:       "ngrok 500 error returns error",
			err:        &ngrok.Error{Msg: "internal error", StatusCode: 500},
			wantResult: ctrl.Result{},
			wantErr:    true,
		},
		{
			name:       "ngrok 502 error returns error",
			err:        &ngrok.Error{Msg: "bad gateway", StatusCode: 502},
			wantResult: ctrl.Result{},
			wantErr:    true,
		},
		{
			name:       "ngrok 429 TooManyRequests requeues after 1 minute",
			err:        &ngrok.Error{Msg: "rate limited", StatusCode: http.StatusTooManyRequests},
			wantResult: ctrl.Result{RequeueAfter: time.Minute},
			wantErr:    false,
		},
		{
			name:       "ngrok 404 NotFound returns error",
			err:        &ngrok.Error{Msg: "not found", StatusCode: http.StatusNotFound},
			wantResult: ctrl.Result{},
			wantErr:    true,
		},
		{
			name:       "ngrok 400 BadRequest does not retry",
			err:        &ngrok.Error{Msg: "bad request", StatusCode: http.StatusBadRequest},
			wantResult: ctrl.Result{},
			wantErr:    false,
		},
		{
			name:       "ngrok FailedToCreateCSR with non-5xx status requeues after 30 seconds",
			err:        &ngrok.Error{Msg: "failed to create CSR", ErrorCode: "ERR_NGROK_20006", StatusCode: http.StatusBadRequest},
			wantResult: ctrl.Result{RequeueAfter: 30 * time.Second},
			wantErr:    true,
		},
		{
			name:       "ngrok FailedToCreateUpstreamService with non-5xx status requeues after 1 minute",
			err:        &ngrok.Error{Msg: "failed to create upstream service", ErrorCode: "ERR_NGROK_20002", StatusCode: http.StatusBadRequest},
			wantResult: ctrl.Result{RequeueAfter: 1 * time.Minute},
			wantErr:    true,
		},
		{
			name:       "ngrok FailedToCreateTargetService with non-5xx status requeues after 1 minute",
			err:        &ngrok.Error{Msg: "failed to create target service", ErrorCode: "ERR_NGROK_20003", StatusCode: http.StatusBadRequest},
			wantResult: ctrl.Result{RequeueAfter: 1 * time.Minute},
			wantErr:    true,
		},
		{
			name:       "ngrok EndpointDenied does not retry",
			err:        &ngrok.Error{Msg: "endpoint denied", ErrorCode: "ERR_NGROK_20005", StatusCode: http.StatusForbidden},
			wantResult: ctrl.Result{},
			wantErr:    false,
		},
		{
			name:       "StatusError requeues after 10 seconds",
			err:        StatusError{err: errors.New("original error"), cause: errors.New("status update failed")},
			wantResult: ctrl.Result{RequeueAfter: 10 * time.Second},
			wantErr:    true,
		},
		{
			name:       "regular error returns error",
			err:        errors.New("some error"),
			wantResult: ctrl.Result{},
			wantErr:    true,
		},
		{
			name:       "nil error returns empty result",
			err:        nil,
			wantResult: ctrl.Result{},
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CtrlResultForErr(tt.err)
			assert.Equal(t, tt.wantResult, result)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestStatusError_Error(t *testing.T) {
	origErr := errors.New("original error")
	causeErr := errors.New("status update failed")
	se := StatusError{err: origErr, cause: causeErr}

	expected := "status update failed: original error"
	assert.Equal(t, expected, se.Error())
}

func TestStatusError_Unwrap(t *testing.T) {
	origErr := errors.New("original error")
	causeErr := errors.New("status update failed")
	se := StatusError{err: origErr, cause: causeErr}

	unwrapped := se.Unwrap()
	assert.Equal(t, causeErr, unwrapped)
}

func TestStatusError_ErrorsAs(t *testing.T) {
	origErr := errors.New("original error")
	causeErr := errors.New("status update failed")
	se := StatusError{err: origErr, cause: causeErr}

	var target StatusError
	assert.True(t, errors.As(se, &target))
	assert.Equal(t, se, target)
}

func TestBaseController_Reconcile_CreateError(t *testing.T) {
	ctx := context.Background()
	s := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(s))

	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "default",
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(ingress).Build()

	createErr := &ngrok.Error{Msg: "rate limited", StatusCode: http.StatusTooManyRequests}
	bc := &BaseController[*netv1.Ingress]{
		Kube:     c,
		Log:      logr.Discard(),
		Recorder: record.NewFakeRecorder(10),
		StatusID: func(_ *netv1.Ingress) string { return "" },
		Create: func(_ context.Context, _ *netv1.Ingress) error {
			return createErr
		},
	}

	result, err := bc.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-ingress", Namespace: "default"},
	}, &netv1.Ingress{})

	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: time.Minute}, result)
}

func TestBaseController_Reconcile_UpdateError(t *testing.T) {
	ctx := context.Background()
	s := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(s))

	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "default",
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(ingress).Build()

	updateErr := &ngrok.Error{Msg: "internal server error", StatusCode: http.StatusInternalServerError}
	bc := &BaseController[*netv1.Ingress]{
		Kube:     c,
		Log:      logr.Discard(),
		Recorder: record.NewFakeRecorder(10),
		StatusID: func(_ *netv1.Ingress) string { return "existing-id" },
		Update: func(_ context.Context, _ *netv1.Ingress) error {
			return updateErr
		},
	}

	result, err := bc.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-ingress", Namespace: "default"},
	}, &netv1.Ingress{})

	assert.Error(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestBaseController_Reconcile_CustomErrResult(t *testing.T) {
	ctx := context.Background()
	s := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(s))

	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "default",
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(ingress).Build()

	customResult := ctrl.Result{RequeueAfter: 5 * time.Minute}
	bc := &BaseController[*netv1.Ingress]{
		Kube:     c,
		Log:      logr.Discard(),
		Recorder: record.NewFakeRecorder(10),
		StatusID: func(_ *netv1.Ingress) string { return "" },
		Create: func(_ context.Context, _ *netv1.Ingress) error {
			return errors.New("some error")
		},
		ErrResult: func(_ BaseControllerOp, _ *netv1.Ingress, _ error) (ctrl.Result, error) {
			return customResult, nil
		},
	}

	result, err := bc.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-ingress", Namespace: "default"},
	}, &netv1.Ingress{})

	assert.NoError(t, err)
	assert.Equal(t, customResult, result)
}

func TestCtrlResultForErr_NgrokErrorCodes(t *testing.T) {
	tests := []struct {
		name       string
		errorCode  string
		statusCode int32
		wantResult ctrl.Result
		wantErr    bool
	}{
		{
			name:       "FailedToCreateCSR with non-5xx status",
			errorCode:  "ERR_NGROK_20006",
			statusCode: http.StatusBadRequest,
			wantResult: ctrl.Result{RequeueAfter: 30 * time.Second},
			wantErr:    true,
		},
		{
			name:       "FailedToCreateUpstreamService with non-5xx status",
			errorCode:  "ERR_NGROK_20002",
			statusCode: http.StatusBadRequest,
			wantResult: ctrl.Result{RequeueAfter: 1 * time.Minute},
			wantErr:    true,
		},
		{
			name:       "FailedToCreateTargetService with non-5xx status",
			errorCode:  "ERR_NGROK_20003",
			statusCode: http.StatusBadRequest,
			wantResult: ctrl.Result{RequeueAfter: 1 * time.Minute},
			wantErr:    true,
		},
		{
			name:       "EndpointDenied",
			errorCode:  "ERR_NGROK_20005",
			statusCode: http.StatusForbidden,
			wantResult: ctrl.Result{},
			wantErr:    false,
		},
		{
			name:       "5xx errors take precedence over error codes",
			errorCode:  "ERR_NGROK_20006",
			statusCode: http.StatusInternalServerError,
			wantResult: ctrl.Result{},
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &ngrok.Error{
				Msg:        tt.name,
				ErrorCode:  tt.errorCode,
				StatusCode: tt.statusCode,
			}
			result, resultErr := CtrlResultForErr(err)
			assert.Equal(t, tt.wantResult, result)
			if tt.wantErr {
				assert.Error(t, resultErr)
			} else {
				assert.NoError(t, resultErr)
			}
		})
	}
}

func TestCtrlResultForErr_WrappedStatusError(t *testing.T) {
	origErr := errors.New("original error")
	causeErr := errors.New("status update failed")
	se := StatusError{err: origErr, cause: causeErr}

	wrappedErr := errors.New("wrapped")
	_ = wrappedErr

	result, err := CtrlResultForErr(se)
	assert.Equal(t, ctrl.Result{RequeueAfter: 10 * time.Second}, result)
	assert.Error(t, err)

	var target StatusError
	assert.True(t, errors.As(err, &target))
}

func TestNewNgrokError(t *testing.T) {
	origErr := errors.New("original error")
	ee := ngrokapi.NgrokOpErrInternalServerError

	ngrokErr := ngrokapi.NewNgrokError(origErr, ee, "test message")

	assert.NotNil(t, ngrokErr)
	assert.Contains(t, ngrokErr.Msg, "test message")
	assert.Contains(t, ngrokErr.Msg, "original error")
}
