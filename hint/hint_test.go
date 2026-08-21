package hint_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/lucasassuncao/yedit/hint"
	"github.com/lucasassuncao/yedit/spec"
	"github.com/lucasassuncao/yedit/theme"
)

// Type shows only when set, Required only when true.
func TestRender_typeAndRequiredBehavior(t *testing.T) {
	th := theme.Resolve(theme.Theme{})

	t.Run("type shown when set", func(t *testing.T) {
		is := assert.New(t)
		out := hint.Render(th, spec.FieldMeta{Type: "string"}, "")
		is.Contains(out, "Type:")
		is.Contains(out, "string")
	})

	t.Run("type omitted when empty", func(t *testing.T) {
		is := assert.New(t)
		out := hint.Render(th, spec.FieldMeta{Description: "desc"}, "")
		is.NotContains(out, "Type:", "expected no Type line when Type is empty")
	})

	t.Run("required shown only when true", func(t *testing.T) {
		is := assert.New(t)
		out := hint.Render(th, spec.FieldMeta{Required: true}, "")
		is.Contains(out, "Required:")
		is.Contains(out, "yes")
	})

	t.Run("required omitted when false", func(t *testing.T) {
		is := assert.New(t)
		out := hint.Render(th, spec.FieldMeta{Description: "desc"}, "")
		is.NotContains(out, "Required:", "expected no Required line when false")
	})
}

// Constraint fields render when set and stay absent on a zero FieldMeta.
func TestRender_constraints(t *testing.T) {
	is := assert.New(t)
	th := theme.Resolve(theme.Theme{})
	out := hint.Render(th, spec.FieldMeta{
		Min: "1s", Max: "168h",
		Pattern:    `^\d+$`,
		MinCount:   1,
		Unique:     true,
		Deprecated: "use limits instead",
	}, "")
	for _, want := range []string{
		"Range:", "1s – 168h",
		"Pattern:", `^\d+$`,
		"Entries:", "1 – ∞",
		"Unique:", "yes",
		"Deprecated:", "use limits instead",
	} {
		is.Contains(out, want, "rendered hint should contain %q", want)
	}
	empty := hint.Render(th, spec.FieldMeta{}, "")
	is.NotContains(empty, "Range:", "zero FieldMeta must render no constraint lines")
	is.NotContains(empty, "Pattern:", "zero FieldMeta must render no constraint lines")
	is.NotContains(empty, "Entries:", "zero FieldMeta must render no constraint lines")
	is.NotContains(empty, "Unique:", "zero FieldMeta must render no constraint lines")
	is.NotContains(empty, "Deprecated:", "zero FieldMeta must render no constraint lines")
}
