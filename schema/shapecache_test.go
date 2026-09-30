package schema

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

type shapedProvider struct{}

func (shapedProvider) Metadata() map[string]any {
	return map[string]any{"size": map[string]any{"kind": "primitive", "scalar": "int"}}
}

// A caller that edits the fields it got back must not change what the next
// caller of the same type sees.
func TestProviderChildrenCacheHandsOutCopies(t *testing.T) {
	typ := reflect.TypeFor[shapedProvider]()
	first := providerChildren(typ)
	if assert.Len(t, first, 1) {
		first[0].YAMLName = "changed"
	}
	assert.Equal(t, "size", providerChildren(typ)[0].YAMLName)
}
