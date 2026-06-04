package document

import "testing"

// TestBlockSemanticEqual_roundtripComparison guards the round-trip verification
// in Insert/Replace. The check used to compare snippet against
// key+":\n"+recovered, but recovered (from BlockContent) already includes the
// key line — so the prefix produced a duplicate-key YAML that fails to parse,
// and blockSemanticEqual fail-opens to true. A real divergence was therefore
// never caught. The fix compares snippet against recovered directly.
func TestBlockSemanticEqual_roundtripComparison(t *testing.T) {
	snippet := "image: ubuntu:22.04\n"
	recovered := "image: ubuntu:22.04\n" // BlockContent includes the key line

	if !blockSemanticEqual(snippet, recovered) {
		t.Error("identical blocks must compare equal")
	}

	diverged := "image: SOMETHING-ELSE\n"
	if blockSemanticEqual(snippet, diverged) {
		t.Error("divergent blocks must compare NOT equal — the check must be able to fail")
	}

	// Documents why the key prefix was removed: the old form builds a duplicate
	// key, fails to parse, and fail-opens to true — masking the divergence above.
	if !blockSemanticEqual(snippet, "image:\n"+diverged) {
		t.Error("regression: the old key-prefixed form should fail-open to true")
	}
}
