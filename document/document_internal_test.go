package document

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBlockSemanticEqual_roundtripComparison guards the round-trip verification
// in Insert/Replace. The check used to compare snippet against
// key+":\n"+recovered, but recovered (from BlockContent) already includes the
// key line - so the prefix produced a duplicate-key YAML that fails to parse,
// and blockSemanticEqual fail-opens to true. A real divergence was therefore
// never caught. The fix compares snippet against recovered directly.
func TestBlockSemanticEqual_roundtripComparison(t *testing.T) {
	is := assert.New(t)
	snippet := "image: ubuntu:22.04\n"
	recovered := "image: ubuntu:22.04\n" // BlockContent includes the key line

	is.True(blockSemanticEqual(snippet, recovered), "identical blocks must compare equal")

	diverged := "image: SOMETHING-ELSE\n"
	is.False(blockSemanticEqual(snippet, diverged), "divergent blocks must compare NOT equal")

	// When b is a malformed duplicate-key document (e.g. the old code produced
	// key+":\n"+recovered, creating two "image" keys), it fails to parse and
	// blockSemanticEqual must return false so the round-trip check triggers a
	// rollback rather than silently accepting corrupted content.
	is.False(blockSemanticEqual(snippet, "image:\n"+diverged), "malformed b must fail-closed (false) so corruption triggers rollback")

	// When a (the original snippet) fails to parse, the function must also
	// fail-closed - it must not silently accept an unverifiable round-trip.
	is.False(blockSemanticEqual("image:\n"+snippet, snippet), "malformed a must fail-closed (false) - symmetric with malformed b")
}

// TestSnapshot_clearsFuture documents the precondition for the rollback fix:
// snapshot() always sets future to nil before the round-trip check runs.
func TestSnapshot_clearsFuture(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	d, err := New([]byte("a: 1\n"), nil)
	must.NoError(err)
	d.future = [][]byte{[]byte("redo-state\n")}

	d = d.snapshot()
	is.Nil(d.future, "snapshot clears the redo stack")
}

// TestRollback_doesNotRestoreFuture shows that rollback() alone leaves the redo
// stack empty -- which is why Insert/Replace must explicitly restore savedFuture.
func TestRollback_doesNotRestoreFuture(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	d, err := New([]byte("a: 1\n"), nil)
	must.NoError(err)
	d.history = [][]byte{copyBytes(d.raw)}
	d.future = [][]byte{[]byte("redo-state\n")}

	d = d.snapshot()
	d = d.rollback()
	is.Empty(d.future, "rollback alone does not restore future (expectedpre-fix behavior)")
}

// TestRollback_savedFutureRestoresRedoStack verifies the fix used in Insert/Replace:
// capture savedFuture before snapshot, restore it in all rollback paths.
func TestRollback_savedFutureRestoresRedoStack(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	d, err := New([]byte("a: 1\n"), nil)
	must.NoError(err)
	d.history = [][]byte{copyBytes(d.raw)}
	d.future = [][]byte{[]byte("redo-state\n")}
	want := d.future

	savedFuture := d.future
	d = d.snapshot()
	d = d.rollback()
	d.future = savedFuture

	is.Equal(want, d.future, "savedFuture pattern restores the redo stack after rollback")
	is.Equal("a: 1\n", string(d.raw), "rollback restores raw")
}

// TestRollback_restoresConsistencyAfterRawMutation verifies that rollback()
// restores both raw and blocks when called after d.raw was set to invalid content.
func TestRollback_restoresConsistencyAfterRawMutation(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	d, err := New([]byte("a: 1\n"), nil)
	must.NoError(err)

	// Simulate what Insert does: snapshot, then set raw to content that would
	// make ParseBlocks fail, then rollback to restore consistency.
	d = d.snapshot()
	d.raw = []byte("invalid: [\n") // unclosed flow sequence - ParseBlocks would fail on this

	// Pre-fix: caller received d with d.raw=invalid, d.blocks=stale (inconsistent)
	// Post-fix: rollback is called, restoring both raw and blocks.
	d = d.rollback()
	is.Equal("a: 1\n", string(d.raw), "rollback restored raw to pre-mutation state")
	must.Len(d.blocks, 1, "rollback restored blocks via re-parse")
	is.Equal("a", d.blocks[0].Key, "restored block key matches original content")
}
