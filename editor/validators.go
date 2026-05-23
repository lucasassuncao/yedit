package editor

import (
	"fmt"
	"strings"

	"github.com/lucasassuncao/yedit/document"
)

// MutuallyExclusive reports a violation when more than one of the listed keys
// is present in the document. Pass the canonical yaml names of the keys that
// must not coexist.
func MutuallyExclusive(keys ...string) Validator {
	return ValidatorFunc(func(_ []byte, blocks []document.Block) []string {
		present := keysPresent(blocks)
		var found []string
		for _, k := range keys {
			if present[k] {
				found = append(found, k)
			}
		}
		if len(found) > 1 {
			return []string{fmt.Sprintf(
				"mutually exclusive — use only one of: %s",
				joinQuoted(found),
			)}
		}
		return nil
	})
}

// RequiredWith reports a violation when key is present but parent is not.
// Useful for "service requires dockerComposeFile" style rules.
func RequiredWith(key, parent string) Validator {
	return ValidatorFunc(func(_ []byte, blocks []document.Block) []string {
		present := keysPresent(blocks)
		if present[key] && !present[parent] {
			return []string{fmt.Sprintf(
				"%q requires %q to be set", key, parent,
			)}
		}
		return nil
	})
}

func keysPresent(blocks []document.Block) map[string]bool {
	out := make(map[string]bool, len(blocks))
	for _, b := range blocks {
		out[b.Key] = true
	}
	return out
}

func joinQuoted(ss []string) string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = `"` + s + `"`
	}
	return strings.Join(out, ", ")
}
