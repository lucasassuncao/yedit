package document

import "testing"

// TestBlockSemanticEqual_roundtripComparison guards the round-trip verification
// in Insert/Replace. The check used to compare snippet against
// key+":\n"+recovered, but recovered (from BlockContent) already includes the
// key line - so the prefix produced a duplicate-key YAML that fails to parse,
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
		t.Error("divergent blocks must compare NOT equal - the check must be able to fail")
	}

	// When b is a malformed duplicate-key document (e.g. the old code produced
	// key+":\n"+recovered, creating two "image" keys), it fails to parse and
	// blockSemanticEqual must return false so the round-trip check triggers a
	// rollback rather than silently accepting corrupted content.
	if blockSemanticEqual(snippet, "image:\n"+diverged) {
		t.Error("malformed b must fail-closed (false) so corruption triggers rollback")
	}

	// When a (the original snippet) fails to parse, the function must also
	// fail-closed - it must not silently accept an unverifiable round-trip.
	if blockSemanticEqual("image:\n"+snippet, snippet) {
		t.Error("malformed a must fail-closed (false) - symmetric with malformed b")
	}
}
