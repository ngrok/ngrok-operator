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
}

func newBase[T any](idPrefix string) baseClient[T] {
	return baseClient[T]{
		items:    make(map[string]T),
		idPrefix: idPrefix,
	}
}

func (m *baseClient[T]) Get(_ context.Context, id string) (T, error) {
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
