package fieldtree

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/yamledit"
)

// A free-form map (map[string]string, map[string]map[string]any) declares no
// child fields: its keys are user data, not schema. New already gets this right
// - the tree is empty and the YAML editor takes focus. The regression is that
// the first resync repopulates it, flagging every user key as UNKNOWN.
func TestFreeFormMapStaysEmptyAfterResync(t *testing.T) {
	cases := map[string]string{
		"containerEnv": "containerEnv:\n  LOG_LEVEL: debug\n  NODE_ENV: development\n",
		"features":     "features:\n  ghcr.io/devcontainers/features/github-cli:1: {}\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			tm := New(schema.KindDictionary, nil, content, 20)
			require.True(t, tm.IsEmpty(), "New should yield an empty tree for a free-form map")

			valueNode := yamledit.BlockValueNode(content)
			require.NotNil(t, valueNode)

			got := SyncCheckedFromNode(tm, valueNode)

			var unknown []string
			for _, n := range got.Nodes {
				if n.Kind == KindUnknown {
					unknown = append(unknown, n.Label)
				}
			}
			assert.Empty(t, unknown, "user-chosen map keys must not be flagged as unknown schema fields")
			assert.True(t, got.IsEmpty(), "resync must leave a free-form map tree empty")
		})
	}
}

// The same resync on a real object block must keep flagging genuinely unknown
// keys - the fix must not blunt that.
func TestObjectBlockStillFlagsUnknownKeys(t *testing.T) {
	defs := []schema.FieldDef{
		{YAMLName: "dockerfile", Kind: schema.KindPrimitive, Scalar: "string"},
		{YAMLName: "context", Kind: schema.KindPrimitive, Scalar: "string"},
	}
	content := "build:\n  dockerfile: Dockerfile\n  bogusKey: 1\n"

	tm := New(schema.KindObject, defs, content, 20)
	got := SyncCheckedFromNode(tm, yamledit.BlockValueNode(content))

	var unknown []string
	for _, n := range got.Nodes {
		if n.Kind == KindUnknown {
			unknown = append(unknown, n.Label)
		}
	}
	assert.Equal(t, []string{"bogusKey"}, unknown)
}
