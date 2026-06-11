package editor

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucasassuncao/yedit/schema"
)

// FuzzStructInvariants feeds random action sequences into a struct block editor
// and asserts the SOT invariant (tree ≡ node) after every step.
// Run with: go test ./editor/... -fuzz=FuzzStructInvariants
func FuzzStructInvariants(f *testing.F) {
	f.Add([]byte{2, 0, 2, 1, 3, 2})
	f.Add([]byte{0, 0, 0, 2, 3, 1, 1, 2})
	f.Add([]byte{2, 2, 2, 3, 3, 3, 0, 2})
	f.Fuzz(func(t *testing.T, actions []byte) {
		be := newBlockEdit(Config{NoDeleteConfirm: true}, blockSpec{
			key: "cfg", defs: cfgStructDefs(), kind: schema.KindObject, content: "cfg:\n",
		}, 120, 40)
		be = expandAll(be)
		for _, a := range actions {
			be = applyFuzzAction(be, a)
			assertTreeMatchesNode(t, be)
		}
	})
}

// FuzzCollectionInvariants feeds random action sequences into a sequence
// collection editor and asserts the collection SOT invariant after every step.
// Run with: go test ./editor/... -fuzz=FuzzCollectionInvariants
func FuzzCollectionInvariants(f *testing.F) {
	f.Add([]byte{2, 0, 2, 3, 0, 2})
	f.Add([]byte{3, 3, 2, 0, 2, 1, 2})
	f.Fuzz(func(t *testing.T, actions []byte) {
		be := newBlockEdit(Config{NoDeleteConfirm: true}, blockSpec{
			key:  "categories",
			defs: catDefs(),
			kind: schema.KindList,
			content: `categories:
  - name: a
  - name: b
`,
		}, 120, 40)
		be = expandAll(be)
		for _, a := range actions {
			be = applyFuzzAction(be, a)
			assertCollTreeMatchesNode(t, be)
		}
	})
}

// applyFuzzAction maps a byte to one of 5 safe editor actions.
func applyFuzzAction(be blockEditState, a byte) blockEditState {
	vis := be.tree.visibleNodes()
	switch a % 5 {
	case 0:
		be, _ = be.updateTreePanel(tea.KeyMsg{Type: tea.KeyDown})
	case 1:
		be, _ = be.updateTreePanel(tea.KeyMsg{Type: tea.KeyUp})
	case 2:
		be, _ = be.updateTreePanel(tea.KeyMsg{Type: tea.KeyEnter})
		be = expandAll(be)
	case 3:
		if be.tree.cursor >= 0 && be.tree.cursor < len(vis) {
			ni := vis[be.tree.cursor]
			if ni < len(be.tree.nodes) {
				n := be.tree.nodes[ni]
				switch {
				case n.kind == treeNodeField && n.checked:
					be = be.dispatch(ToggleField{NodeIdx: ni, Checked: false})
					be = expandAll(be)
				case n.kind == treeNodeSeqItem:
					be = be.performEntryDelete(n.seqIdx)
				}
			}
		}
	case 4:
		be, _ = be.updateTreePanel(tea.KeyMsg{Type: tea.KeyRight})
	}
	return be
}
