package nmockapi

import (
	context "context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"time"

	"github.com/ngrok/ngrok-api-go/v9"
	"github.com/segmentio/ksuid"
)

// baseClient is shared by the mock resource clients. P is the paging type
// used by that resource's real List method (ngrok.Paging or
// ngrok.FilteredPaging), which varies by resource in ngrok-api-go v9.
type baseClient[T any, P any] struct {
	idPrefix string
	items    map[string]T

	// Error injection fields for testing
	createError error
	getError    error
	updateError error
	listError   error

	// Call counters for testing
	updateCallCount int

	// lastFilter records the CEL filter from the most recent List call.
	lastFilter string
}

func newBase[T any, P any](idPrefix string) baseClient[T, P] {
	return baseClient[T, P]{
		items:    make(map[string]T),
		idPrefix: idPrefix,
	}
}

func (m *baseClient[T, P]) Get(_ context.Context, id string) (T, error) {
	if m.getError != nil {
		return *new(T), m.getError
	}
	item, ok := m.items[id]
	if !ok {
		return *new(T), m.notFoundErr()
	}
	return item, nil
}

func (m *baseClient[T, P]) List(paging *P) ngrok.Iter[T] {
	items := slices.Collect(maps.Values(m.items))
	if m.listError != nil {
		return NewIter(items, m.listError)
	}

	// Record what the caller asked for so tests can assert on the filter the
	// production code built, which is a separate question from whether filtering
	// selected the right items.
	filter := pagingFilter(paging)
	m.lastFilter = filter

	matched, err := applyFilter(items, filter)
	if err != nil {
		return NewIter[T](nil, err)
	}
	return NewIter(matched, nil)
}

// LastFilter returns the CEL filter expression passed to the most recent List
// call, or "" if that call passed none.
func (m *baseClient[T, P]) LastFilter() string {
	return m.lastFilter
}

func (m *baseClient[T, P]) Delete(ctx context.Context, id string) error {
	_, err := m.Get(ctx, id)
	if err != nil {
		return err
	}
	delete(m.items, id)
	return nil
}

// Reset clears the items in the client.
// This is useful for resetting the state of the client between tests, without allocating a new client.
func (m *baseClient[T, P]) Reset() {
	m.items = make(map[string]T)
	m.updateCallCount = 0
	m.lastFilter = ""
}

func (m *baseClient[T, P]) newID() string {
	return fmt.Sprintf("%s_%s", m.idPrefix, ksuid.New().String())
}

func (m *baseClient[T, P]) notFoundErr() error {
	return &ngrok.Error{
		StatusCode: http.StatusNotFound,
	}
}

func (m *baseClient[T, P]) any(predicate func(T) bool) bool {
	for _, item := range m.items {
		if predicate(item) {
			return true
		}
	}
	return false
}

func (m *baseClient[T, P]) createdAt() string {
	return time.Now().Format(time.RFC3339)
}

// SetCreateError configures the client to return an error on Create calls
func (m *baseClient[T, P]) SetCreateError(err error) {
	m.createError = err
}

// SetGetError configures the client to return an error on Get calls
func (m *baseClient[T, P]) SetGetError(err error) {
	m.getError = err
}

// SetUpdateError configures the client to return an error on Update calls
func (m *baseClient[T, P]) SetUpdateError(err error) {
	m.updateError = err
}

// SetListError configures the client to return an error on List calls
func (m *baseClient[T, P]) SetListError(err error) {
	m.listError = err
}

// ClearErrors clears all configured errors
func (m *baseClient[T, P]) ClearErrors() {
	m.createError = nil
	m.getError = nil
	m.updateError = nil
	m.listError = nil
}

// UpdateCallCount returns the number of times Update has been called
func (m *baseClient[T, P]) UpdateCallCount() int {
	return m.updateCallCount
}

// ResetUpdateCallCount resets the update call counter to zero
func (m *baseClient[T, P]) ResetUpdateCallCount() {
	m.updateCallCount = 0
}
