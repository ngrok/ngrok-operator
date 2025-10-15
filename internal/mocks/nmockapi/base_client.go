package nmockapi

import (
	context "context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"time"

	"github.com/ngrok/ngrok-api-go/v7"
	"github.com/segmentio/ksuid"
)

type baseClient[T any] struct {
	idPrefix string
	items    map[string]T

	// Error injection fields for testing
	createError error
	getError    error
	updateError error
	listError   error
}

func newBase[T any](idPrefix string) baseClient[T] {
	return baseClient[T]{
		items:    make(map[string]T),
		idPrefix: idPrefix,
	}
}

func (m *baseClient[T]) Get(_ context.Context, id string) (T, error) {
	if m.getError != nil {
		return *new(T), m.getError
	}
	item, ok := m.items[id]
	if !ok {
		return *new(T), m.notFoundErr()
	}
	return item, nil
}

func (m *baseClient[T]) List(_ *ngrok.Paging) ngrok.Iter[T] {
	items := slices.Collect(maps.Values(m.items))
	return NewIter(items, nil)
}

func (m *baseClient[T]) Delete(ctx context.Context, id string) error {
	_, err := m.Get(ctx, id)
	if err != nil {
		return err
	}
	delete(m.items, id)
	return nil
}

// Reset clears the items in the client.
// This is useful for resetting the state of the client between tests, without allocating a new client.
func (m *baseClient[T]) Reset() {
	m.items = make(map[string]T)
}

func (m *baseClient[T]) newID() string {
	return fmt.Sprintf("%s_%s", m.idPrefix, ksuid.New().String())
}

func (m *baseClient[T]) notFoundErr() error {
	return &ngrok.Error{
		StatusCode: http.StatusNotFound,
	}
}

func (m *baseClient[T]) any(predicate func(T) bool) bool {
	for _, item := range m.items {
		if predicate(item) {
			return true
		}
	}
	return false
}

func (m *baseClient[T]) createdAt() string {
	return time.Now().Format(time.RFC3339)
}

// SetCreateError configures the client to return an error on Create calls
func (m *baseClient[T]) SetCreateError(err error) {
	m.createError = err
}

// SetGetError configures the client to return an error on Get calls
func (m *baseClient[T]) SetGetError(err error) {
	m.getError = err
}

// SetUpdateError configures the client to return an error on Update calls
func (m *baseClient[T]) SetUpdateError(err error) {
	m.updateError = err
}

// SetListError configures the client to return an error on List calls
func (m *baseClient[T]) SetListError(err error) {
	m.listError = err
}

// ClearErrors clears all configured errors
func (m *baseClient[T]) ClearErrors() {
	m.createError = nil
	m.getError = nil
	m.updateError = nil
	m.listError = nil
}
