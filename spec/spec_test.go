package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.yaml.in/yaml/v3"
)

func TestSeverityString(t *testing.T) {
	assert.Equal(t, "error", SeverityError.String())
	assert.Equal(t, "warning", SeverityWarning.String())
	assert.Equal(t, "error", Violation{}.Severity.String(), "the zero severity is an error")
}

func TestViolation_PathAndString(t *testing.T) {
	if got := (Violation{Message: "msg"}).String(); got != "msg" {
		t.Errorf("String without Path = %q, want %q", got, "msg")
	}
	if got := (Violation{Path: "a.b", Message: "msg"}).String(); got != "a.b: msg" {
		t.Errorf("String with Path = %q, want %q", got, "a.b: msg")
	}
}

func TestNewValidationInput(t *testing.T) {
	in := NewValidationInput([]byte("a: 1\n"), nil)
	require.NotNil(t, in.Root)
	assert.Equal(t, yaml.MappingNode, in.Root.Kind)
	assert.Equal(t, []byte("a: 1\n"), in.Raw)

	empty := NewValidationInput(nil, nil)
	require.NotNil(t, empty.Root, "an empty document still runs unconditional checks")
	assert.Empty(t, empty.Root.Content)

	assert.Nil(t, NewValidationInput([]byte("a: [unclosed\n  b: }{"), nil).Root)
}

func TestValidatorFunc(t *testing.T) {
	want := []Violation{{Path: "x", Message: "bad"}}
	var v Validator = ValidatorFunc(func(ValidationInput) []Violation { return want })
	assert.Equal(t, want, v.Validate(ValidationInput{}))
}

func TestMetadataFunc(t *testing.T) {
	var src MetadataSource = MetadataFunc(func(block, field string) FieldMeta {
		return FieldMeta{Required: block == "server" && field == "host"}
	})
	assert.True(t, src.FieldMeta("server", "host").Required)
	assert.False(t, src.FieldMeta("server", "port").Required)
}
