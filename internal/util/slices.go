package util

// NilIfEmpty returns nil when s has no elements, and s otherwise.
//
// This distinction matters for ngrok API request structs whose JSON tags use
// omitzero: a nil slice is omitted from the request body, while a non-nil
// empty slice is serialized as `[]`, which explicitly clears the field
// server-side instead of leaving it untouched.
func NilIfEmpty[T any](s []T) []T {
	if len(s) == 0 {
		return nil
	}
	return s
}
