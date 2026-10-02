package nmockapi

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/ngrok/ngrok-api-go/v9"
)

// This file gives the mock clients real CEL filtering, matching what the ngrok
// API does server-side (https://ngrok.com/docs/api/api-filtering). cel-go is
// used purely as test infrastructure: nothing under cmd/ imports this package.
//
// Without it, List would ignore FilteredPaging.Filter entirely and every
// filtered lookup would appear to work no matter what expression it sent.

// pagingFilter extracts the CEL filter expression from an arbitrary paging
// argument. ngrok-api-go v9 has exactly two paging types: ngrok.Paging (no
// filtering) and ngrok.FilteredPaging (Filter *string). They share no method,
// so a type switch on the boxed pointer is the only non-reflective way to reach
// Filter from baseClient[T, P]. A third paging type would silently lose its
// filter here, which is why TestPagingFilter enumerates the known types.
//
// Returns "" when there is no filter, meaning "return everything".
func pagingFilter[P any](paging *P) string {
	switch p := any(paging).(type) {
	case *ngrok.FilteredPaging:
		if p == nil || p.Filter == nil {
			return ""
		}
		return *p.Filter
	default:
		return ""
	}
}

// celEnv is built once: cel.NewEnv constructs the whole standard declaration
// set and type registry, which dwarfs the cost of compiling an expression.
var celEnv = sync.OnceValues(func() (*cel.Env, error) {
	return cel.NewEnv(cel.Variable("obj", cel.DynType))
})

// celPrograms caches compiled filters. The operator only ever emits a small
// fixed set of expressions, and List runs on every reconcile.
var celPrograms sync.Map // filter string -> cel.Program

func badFilterErr(filter string, cause error) error {
	return &ngrok.Error{
		StatusCode: http.StatusBadRequest,
		Msg:        fmt.Sprintf("invalid filter %q: %s", filter, cause),
		ErrorCode:  "ERR_NGROK_9001",
	}
}

// compileFilter compiles a CEL filter expression, caching the result.
func compileFilter(filter string) (cel.Program, error) {
	if cached, ok := celPrograms.Load(filter); ok {
		return cached.(cel.Program), nil
	}

	env, err := celEnv()
	if err != nil {
		return nil, fmt.Errorf("building CEL environment: %w", err)
	}

	ast, issues := env.Compile(filter)
	if issues != nil && issues.Err() != nil {
		return nil, badFilterErr(filter, issues.Err())
	}

	prg, err := env.Program(ast)
	if err != nil {
		return nil, badFilterErr(filter, err)
	}

	celPrograms.Store(filter, prg)
	return prg, nil
}

// applyFilter evaluates filter against items and returns those that match,
// mirroring what the real API does server-side.
//
// An expression that does not compile, does not evaluate, or does not yield a
// bool is a client error: the real API answers HTTP 400, so we do too rather
// than passing the items through. In particular a typo'd field name
// (obj.doamin) reaches CEL as a missing key and must surface as an error --
// swallowing it would return "no matches" and hide exactly the class of bug
// real CEL evaluation is here to catch.
func applyFilter[T any](items []T, filter string) ([]T, error) {
	if filter == "" {
		return items, nil
	}

	prg, err := compileFilter(filter)
	if err != nil {
		return nil, err
	}

	matched := make([]T, 0, len(items))
	for _, item := range items {
		out, _, err := prg.Eval(map[string]any{"obj": celObject(item)})
		if err != nil {
			return nil, badFilterErr(filter, err)
		}

		match, ok := out.Value().(bool)
		if !ok {
			return nil, badFilterErr(filter, fmt.Errorf("expression yielded %T, want bool", out.Value()))
		}
		if match {
			matched = append(matched, item)
		}
	}

	return matched, nil
}

// celObject converts v into the map shape the ngrok API's CEL filters expect:
// keys are JSON/API field names, and every declared field is present even when
// it holds its zero value.
//
// json.Marshal is deliberately not used for structs. ngrok-api-go v9 tags its
// fields `omitzero`, so marshalling drops zero-valued fields, and cel-go raises
// "no such key" -- an error, not false -- when an expression selects an absent
// key. Emitting every declared key, with nil for absent pointers, gives CEL
// `null`, which compares as false against any other type rather than erroring.
// That keeps a legitimately-empty field from looking like a broken filter,
// while a genuinely unknown field name still errors.
func celObject(v any) any {
	return celValue(reflect.ValueOf(v))
}

func celValue(rv reflect.Value) any {
	if !rv.IsValid() {
		return nil
	}

	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface:
		if rv.IsNil() {
			return nil
		}
		return celValue(rv.Elem())

	case reflect.Struct:
		obj := map[string]any{}
		celStructFields(rv, obj)
		return obj

	case reflect.Slice, reflect.Array:
		// json.RawMessage and other byte slices are strings on the wire.
		if rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() == reflect.Uint8 {
			if rv.IsNil() {
				return nil
			}
			return string(rv.Bytes())
		}
		if rv.Kind() == reflect.Slice && rv.IsNil() {
			return nil
		}
		out := make([]any, rv.Len())
		for i := range out {
			out[i] = celValue(rv.Index(i))
		}
		return out

	case reflect.Map:
		if rv.IsNil() {
			return nil
		}
		out := map[string]any{}
		for _, key := range rv.MapKeys() {
			out[fmt.Sprint(key.Interface())] = celValue(rv.MapIndex(key))
		}
		return out

	case reflect.String:
		return rv.String()
	case reflect.Bool:
		return rv.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(rv.Uint())
	case reflect.Float32, reflect.Float64:
		return rv.Float()
	default:
		return rv.Interface()
	}
}

// celStructFields writes rv's exported fields into obj, keyed by JSON name.
func celStructFields(rv reflect.Value, obj map[string]any) {
	rt := rv.Type()
	for i := range rt.NumField() {
		field := rt.Field(i)
		if !field.IsExported() {
			continue
		}

		tag := field.Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "-" {
			continue
		}

		// Embedded structs without a JSON name splice their fields in.
		if field.Anonymous && name == "" {
			inner := rv.Field(i)
			for inner.Kind() == reflect.Pointer {
				if inner.IsNil() {
					inner = reflect.Value{}
					break
				}
				inner = inner.Elem()
			}
			if inner.IsValid() && inner.Kind() == reflect.Struct {
				celStructFields(inner, obj)
				continue
			}
		}

		if name == "" {
			name = field.Name
		}
		obj[name] = celValue(rv.Field(i))
	}
}
