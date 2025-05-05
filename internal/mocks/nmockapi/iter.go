package nmockapi

import context "context"

// Iter is a mock iterator that implements the ngrok.Iter[T] interface.
type Iter[T any] struct {
	items []T
	err   error
	n     int
}

func (m *Iter[T]) Next(_ context.Context) bool {
	// If there is an error, stop iteration
	if m.err != nil {
		return false
	}

	// Increment the index
	m.n++

	return m.n < len(m.items) && m.n >= 0
}

func (m *Iter[T]) Item() T {
	if m.n >= 0 && m.n < len(m.items) {
		return m.items[m.n]
	}
	return *new(T)
}

func (m *Iter[T]) Err() error {
	return m.err
}

func NewIter[T any](items []T, err error) *Iter[T] {
	return &Iter[T]{
		items: items,
		err:   err,
		n:     -1,
	}
}
