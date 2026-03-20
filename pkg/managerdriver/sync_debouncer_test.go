package managerdriver

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/testutils"
)

// TestSyncDebouncer verifies the sync debouncer's contract with reconcilers.
//
// The debouncer batches concurrent Sync/SyncEndpoints calls so that only one
// sync body runs at a time. The contract, from a reconciler's perspective:
//
//   - No contention: reconcile succeeds, no requeue.
//   - Concurrent calls while a sync is running: exactly ONE reconciler gets
//     ctrl.Result{Requeue: true} (so it retries with the full store), all
//     others succeed silently with ctrl.Result{}.
//   - After requeue: the next reconcile succeeds normally.
//
// Startup example (100 ingress objects reconcile at once):
//  1. Reconciler 1 proceeds and syncs on a partial store view.
//  2. Reconcilers 2-99 are dismissed (success, no requeue).
//  3. Reconciler 100 gets Requeue: true. On retry, the store is fully
//     populated, so it syncs the complete state.
//
// Note: tests use the unexported syncStart/syncDone methods to hold the
// debouncer lock during test setup. This is necessary because the sync body
// completes too fast for concurrent goroutines to reliably arrive during it.
// Assertions are made entirely through the public reconciler interface
// (HandleSyncResult + ctrl.Result).
func TestSyncDebouncer(t *testing.T) {
	t.Parallel()

	testScheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(testScheme))
	utilruntime.Must(ingressv1alpha1.AddToScheme(testScheme))
	utilruntime.Must(ngrokv1alpha1.AddToScheme(testScheme))

	newDriver := func() *Driver {
		return NewDriver(
			logr.Discard(),
			testScheme,
			testutils.DefaultControllerName,
			types.NamespacedName{Name: "test-manager"},
			WithSyncAllowConcurrent(false),
		)
	}

	newClient := func() client.Client {
		return fake.NewClientBuilder().WithScheme(testScheme).Build()
	}

	reconcileSync := func(d *Driver, ctx context.Context) (ctrl.Result, error) {
		return HandleSyncResult(d.Sync(ctx, newClient()))
	}

	reconcileSyncEndpoints := func(d *Driver, ctx context.Context) (ctrl.Result, error) {
		return HandleSyncResult(d.SyncEndpoints(ctx, newClient()))
	}

	type reconcileResult struct {
		result ctrl.Result
		err    error
	}

	// collectResults gathers n reconcile results with a timeout and counts
	// how many requested a requeue vs succeeded without one.
	collectResults := func(t *testing.T, ch <-chan reconcileResult, n int) (requeues, successes int) {
		t.Helper()
		for i := range n {
			select {
			case res := <-ch:
				require.NoError(t, res.err, "reconcile %d returned unexpected error", i)
				if res.result.RequeueAfter > 0 {
					requeues++
				} else {
					successes++
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("timed out waiting for result %d/%d", i+1, n)
			}
		}
		return
	}

	t.Run("no contention - reconcile succeeds without requeue", func(t *testing.T) {
		t.Parallel()
		d := newDriver()

		for i := range 5 {
			result, err := reconcileSync(d, context.Background())
			require.NoError(t, err)
			assert.Zero(t, result.RequeueAfter, "iteration %d", i)
		}
	})

	t.Run("concurrent syncs - exactly one requeue, rest dismissed", func(t *testing.T) {
		t.Parallel()
		d := newDriver()
		ctx := context.Background()

		// Hold the debouncer lock to simulate a running sync.
		proceed, _ := d.syncStart(false)
		require.True(t, proceed)

		const N = 20
		results := make(chan reconcileResult, N)
		var ready sync.WaitGroup
		ready.Add(N)

		for range N {
			go func() {
				ready.Done()
				r, e := reconcileSync(d, ctx)
				results <- reconcileResult{r, e}
			}()
		}

		ready.Wait()
		time.Sleep(100 * time.Millisecond)
		d.syncDone()

		requeues, successes := collectResults(t, results, N)
		assert.Equal(t, 1, requeues, "exactly one reconciler should be requeued")
		assert.Equal(t, N-1, successes, "all others should succeed without requeue")
	})

	t.Run("requeued reconciler completes on next attempt", func(t *testing.T) {
		t.Parallel()
		d := newDriver()
		ctx := context.Background()

		// First sync is running.
		proceed, _ := d.syncStart(false)
		require.True(t, proceed)

		// A reconciler arrives and waits.
		resCh := make(chan reconcileResult, 1)
		go func() {
			r, e := reconcileSync(d, ctx)
			resCh <- reconcileResult{r, e}
		}()
		time.Sleep(50 * time.Millisecond)
		d.syncDone()

		// The waiter should get a requeue.
		res := <-resCh
		require.NoError(t, res.err)
		require.Greater(t, res.result.RequeueAfter, time.Duration(0))

		// On retry, the debouncer is idle — sync runs to completion.
		result, err := reconcileSync(d, ctx)
		require.NoError(t, err)
		assert.Zero(t, result.RequeueAfter)
	})

	t.Run("SyncEndpoints shares the debouncer with Sync", func(t *testing.T) {
		t.Parallel()
		d := newDriver()
		ctx := context.Background()

		// A Sync is running.
		proceed, _ := d.syncStart(false)
		require.True(t, proceed)

		// SyncEndpoints arrives — it should be debounced by the same lock.
		resCh := make(chan reconcileResult, 1)
		go func() {
			r, e := reconcileSyncEndpoints(d, ctx)
			resCh <- reconcileResult{r, e}
		}()
		time.Sleep(50 * time.Millisecond)
		d.syncDone()

		res := <-resCh
		require.NoError(t, res.err)
		assert.Greater(t, res.result.RequeueAfter, time.Duration(0), "SyncEndpoints waiter should be requeued")
	})

	t.Run("context cancellation releases waiting reconciler", func(t *testing.T) {
		t.Parallel()
		d := newDriver()

		proceed, _ := d.syncStart(false)
		require.True(t, proceed)

		ctx, cancel := context.WithCancel(context.Background())
		resCh := make(chan reconcileResult, 1)
		go func() {
			r, e := reconcileSync(d, ctx)
			resCh <- reconcileResult{r, e}
		}()

		time.Sleep(50 * time.Millisecond)
		cancel()

		select {
		case res := <-resCh:
			assert.ErrorIs(t, res.err, context.Canceled)
		case <-time.After(5 * time.Second):
			t.Fatal("timed out")
		}

		d.syncDone()
	})

	t.Run("concurrent mode bypasses debouncer", func(t *testing.T) {
		t.Parallel()
		d := NewDriver(
			logr.Discard(),
			testScheme,
			testutils.DefaultControllerName,
			types.NamespacedName{Name: "test-manager"},
			WithSyncAllowConcurrent(true),
		)
		c := newClient()
		ctx := context.Background()

		const N = 5
		results := make(chan reconcileResult, N)
		var ready sync.WaitGroup
		ready.Add(N)

		for range N {
			go func() {
				ready.Done()
				r, e := HandleSyncResult(d.Sync(ctx, c))
				results <- reconcileResult{r, e}
			}()
		}

		ready.Wait()

		requeues, successes := collectResults(t, results, N)
		assert.Equal(t, 0, requeues, "no requeues in concurrent mode")
		assert.Equal(t, N, successes, "all syncs succeed independently")
	})
}
