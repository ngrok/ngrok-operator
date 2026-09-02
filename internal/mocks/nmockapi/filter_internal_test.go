package nmockapi

import (
	"net/http"
	"testing"

	"github.com/ngrok/ngrok-api-go/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPagingFilter(t *testing.T) {
	t.Run("FilteredPaging with a filter", func(t *testing.T) {
		assert.Equal(t, `obj.domain == "a.com"`,
			pagingFilter(&ngrok.FilteredPaging{Filter: new(`obj.domain == "a.com"`)}))
	})

	t.Run("FilteredPaging with no filter", func(t *testing.T) {
		assert.Empty(t, pagingFilter(&ngrok.FilteredPaging{}))
		assert.Empty(t, pagingFilter(&ngrok.FilteredPaging{Limit: new("1")}))
	})

	t.Run("typed nil FilteredPaging", func(t *testing.T) {
		assert.Empty(t, pagingFilter[ngrok.FilteredPaging](nil))
	})

	// ngrok.Paging has no Filter field at all; these resources cannot filter.
	t.Run("Paging never filters", func(t *testing.T) {
		assert.Empty(t, pagingFilter(&ngrok.Paging{Limit: new("1")}))
		assert.Empty(t, pagingFilter[ngrok.Paging](nil))
	})
}

// TestCELObject_EmitsEveryDeclaredKey is the regression guard for the omitzero
// trap: json.Marshal would drop every zero-valued field, and CEL raises an
// error rather than false when an expression selects a missing key.
func TestCELObject_EmitsEveryDeclaredKey(t *testing.T) {
	obj, ok := celObject(&ngrok.ReservedDomain{Domain: "a.example.com"}).(map[string]any)
	require.True(t, ok, "celObject should render a struct as a map")

	for _, key := range []string{
		"id", "uri", "created_at", "description", "metadata", "domain",
		"region", "cname_target", "certificate", "certificate_management_policy",
		"certificate_management_status", "acme_challenge_cname_target", "resolves_to",
	} {
		assert.Contains(t, obj, key, "key %q must be present even when zero", key)
	}

	assert.Equal(t, "a.example.com", obj["domain"])
	// Absent pointers become CEL null, not "".
	assert.Nil(t, obj["cname_target"])
	assert.Nil(t, obj["certificate"])
	assert.Nil(t, obj["resolves_to"])
	assert.Equal(t, "", obj["description"])
}

func TestCELObject_NestedAndSlices(t *testing.T) {
	obj := celObject(&ngrok.ReservedDomain{
		Domain:      "a.example.com",
		CNAMETarget: new("target.ngrok-cname.com"),
		Certificate: &ngrok.Ref{ID: "cert_123", URI: "https://example/cert_123"},
		ResolvesTo:  []ngrok.ReservedDomainResolvesToEntry{{Value: "us"}},
	}).(map[string]any)

	assert.Equal(t, "target.ngrok-cname.com", obj["cname_target"])

	cert, ok := obj["certificate"].(map[string]any)
	require.True(t, ok, "nested struct pointer should render as a map")
	assert.Equal(t, "cert_123", cert["id"])

	resolvesTo, ok := obj["resolves_to"].([]any)
	require.True(t, ok, "slice should render as []any")
	require.Len(t, resolvesTo, 1)
	assert.Equal(t, "us", resolvesTo[0].(map[string]any)["value"])
}

func TestApplyFilter(t *testing.T) {
	wildcard := &ngrok.ReservedDomain{ID: "rd_w", Domain: "*.example.com"}
	child := &ngrok.ReservedDomain{ID: "rd_c", Domain: "a.example.com"}
	other := &ngrok.ReservedDomain{ID: "rd_o", Domain: "unrelated.test"}
	items := []*ngrok.ReservedDomain{wildcard, child, other}

	t.Run("empty filter returns everything", func(t *testing.T) {
		got, err := applyFilter(items, "")
		require.NoError(t, err)
		assert.Equal(t, items, got)
	})

	t.Run("exact match", func(t *testing.T) {
		got, err := applyFilter(items, `obj.domain == "a.example.com"`)
		require.NoError(t, err)
		assert.Equal(t, []*ngrok.ReservedDomain{child}, got)
	})

	t.Run("exact match with no hit", func(t *testing.T) {
		got, err := applyFilter(items, `obj.domain == "nope.example.com"`)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("in list returns both candidates", func(t *testing.T) {
		got, err := applyFilter(items, `obj.domain in ["a.example.com","*.example.com"]`)
		require.NoError(t, err)
		assert.ElementsMatch(t, []*ngrok.ReservedDomain{child, wildcard}, got)
	})

	// Server-side proof that *.example.com does not cover a deeper label: the
	// operator asks for the direct parent only, so nothing comes back.
	t.Run("in list does not match a non-direct wildcard parent", func(t *testing.T) {
		got, err := applyFilter(items, `obj.domain in ["y.x.example.com","*.x.example.com"]`)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	// A legitimately-empty field must compare cleanly rather than erroring.
	t.Run("unset pointer field compares as null", func(t *testing.T) {
		got, err := applyFilter(items, `obj.cname_target == null`)
		require.NoError(t, err)
		assert.Len(t, got, 3)
	})

	t.Run("uncompilable filter is a 400", func(t *testing.T) {
		_, err := applyFilter(items, `obj.domain ==`)
		require.Error(t, err)

		var ngrokErr *ngrok.Error
		require.ErrorAs(t, err, &ngrokErr)
		assert.EqualValues(t, http.StatusBadRequest, ngrokErr.StatusCode)
	})

	// The point of real CEL evaluation: a typo'd field name must not silently
	// look like "no matches".
	t.Run("unknown field name is an error, not an empty result", func(t *testing.T) {
		_, err := applyFilter(items, `obj.doamin == "a.example.com"`)
		require.Error(t, err)
	})

	t.Run("non-bool expression is a 400", func(t *testing.T) {
		_, err := applyFilter(items, `obj.domain`)
		require.Error(t, err)
	})
}
