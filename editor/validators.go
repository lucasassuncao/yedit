package editor

import (
	"fmt"
	"strings"

	"github.com/lucasassuncao/yedit/document"
)

// presenceValidator is an unexported extension of Validator that receives a
// pre-computed keysPresent map so RunAll can share it across all validators.
type presenceValidator interface {
	Validator
	validateWithPresent(raw []byte, blocks []document.Block, present map[string]bool) []string
}

// RunAll executes all validators against raw/blocks and collects violations.
// keysPresent is computed once and shared with validators that implement
// presenceValidator, avoiding redundant allocations per call.
func RunAll(validators []Validator, raw []byte, blocks []document.Block) []string {
	if len(validators) == 0 {
		return nil
	}
	present := keysPresent(blocks)
	var errs []string
	for _, v := range validators {
		if pv, ok := v.(presenceValidator); ok {
			errs = append(errs, pv.validateWithPresent(raw, blocks, present)...)
		} else {
			errs = append(errs, v.Validate(raw, blocks)...)
		}
	}
	return errs
}

// MutuallyExclusive reports a violation when more than one of the listed keys
// is present in the document.
func MutuallyExclusive(keys ...string) Validator {
	return &mutuallyExclusiveValidator{keys: keys}
}

type mutuallyExclusiveValidator struct{ keys []string }

func (v *mutuallyExclusiveValidator) Validate(raw []byte, blocks []document.Block) []string {
	return v.validateWithPresent(raw, blocks, keysPresent(blocks))
}

func (v *mutuallyExclusiveValidator) validateWithPresent(_ []byte, _ []document.Block, present map[string]bool) []string {
	var found []string
	for _, k := range v.keys {
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
}

// RequiredWith reports a violation when key is present but parent is not.
func RequiredWith(key, parent string) Validator {
	return &requiredWithValidator{key: key, parent: parent}
}

type requiredWithValidator struct{ key, parent string }

func (v *requiredWithValidator) Validate(raw []byte, blocks []document.Block) []string {
	return v.validateWithPresent(raw, blocks, keysPresent(blocks))
}

func (v *requiredWithValidator) validateWithPresent(_ []byte, _ []document.Block, present map[string]bool) []string {
	if present[v.key] && !present[v.parent] {
		return []string{fmt.Sprintf(
			"%q requires %q to be set", v.key, v.parent,
		)}
	}
	return nil
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
