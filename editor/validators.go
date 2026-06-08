package editor

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/document"
)

// RunAll executes all validators against raw/blocks and collects violations.
func RunAll(validators []Validator, raw []byte, blocks []document.Block) []string {
	if len(validators) == 0 {
		return nil
	}
	var errs []string
	for _, v := range validators {
		errs = append(errs, v.Validate(raw, blocks)...)
	}
	return errs
}

// MutuallyExclusive reports a violation when more than one of the listed keys
// is present at the same time.
//
// Two forms are supported:
//
// Top-level keys (no dots) — checks the document's root-level blocks:
//
//	editor.MutuallyExclusive("image", "build", "dockerComposeFile")
//
// Dotted paths — all paths must share the same parent prefix. The validator
// navigates to that parent in the YAML tree, automatically expanding sequences
// (all items are checked) and dict-style mappings (all values are checked).
// Use this for constraints that live at a specific location in the document:
//
//	editor.MutuallyExclusive(
//	    "categories.installers.source.filter.any",
//	    "categories.installers.source.filter.all",
//	)
//
// For constraints that must hold at every occurrence of a key regardless of
// depth (e.g. recursive schemas), use MutuallyExclusiveNested instead.
func MutuallyExclusive(keys ...string) Validator {
	for _, k := range keys {
		if strings.Contains(k, ".") {
			return newPathMutuallyExclusiveValidator(keys)
		}
	}
	return &mutuallyExclusiveValidator{keys: keys}
}

type mutuallyExclusiveValidator struct{ keys []string }

func (v *mutuallyExclusiveValidator) Validate(_ []byte, blocks []document.Block) []string {
	present := keysPresent(blocks)
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

// newPathMutuallyExclusiveValidator builds the path-aware variant. All keys
// must share the same parent path (everything before the last dot). The leaf
// segments (last component of each path) become the mutually exclusive keys.
func newPathMutuallyExclusiveValidator(fullPaths []string) Validator {
	segs := make([][]string, len(fullPaths))
	for i, p := range fullPaths {
		segs[i] = strings.Split(p, ".")
	}
	// Validate that all paths share the same parent.
	parent := segs[0][:len(segs[0])-1]
	for _, s := range segs[1:] {
		if len(s) != len(segs[0]) {
			// Mismatched depth — fall back to treating them as plain keys.
			return &mutuallyExclusiveValidator{keys: fullPaths}
		}
		for j, seg := range s[:len(s)-1] {
			if seg != parent[j] {
				return &mutuallyExclusiveValidator{keys: fullPaths}
			}
		}
	}
	leaves := make([]string, len(segs))
	for i, s := range segs {
		leaves[i] = s[len(s)-1]
	}
	return &pathMutuallyExclusiveValidator{parentSegs: parent, keys: leaves}
}

type pathMutuallyExclusiveValidator struct {
	parentSegs []string // path segments to the parent mapping
	keys       []string // mutually exclusive leaf keys within that mapping
}

func (v *pathMutuallyExclusiveValidator) Validate(raw []byte, _ []document.Block) []string {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil || len(doc.Content) == 0 {
		return nil
	}
	var errs []string
	v.navigate(doc.Content[0], v.parentSegs, "", &errs)
	return errs
}

// navigate walks node following segs. On arrival (len(segs)==0) it checks the
// mutual-exclusivity constraint. Sequences are expanded (all items checked);
// dict-style mappings whose direct key does not match the next segment are
// also expanded (all values checked), covering map[string]Struct fields.
func (v *pathMutuallyExclusiveValidator) navigate(node *yaml.Node, segs []string, path string, errs *[]string) {
	if node.Kind == yaml.SequenceNode {
		for i, item := range node.Content {
			v.navigate(item, segs, fmt.Sprintf("%s[%d]", path, i), errs)
		}
		return
	}

	if len(segs) == 0 {
		v.check(node, path, errs)
		return
	}

	if node.Kind != yaml.MappingNode {
		return
	}

	next, rest := segs[0], segs[1:]
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == next {
			childPath := next
			if path != "" {
				childPath = path + "." + next
			}
			v.navigate(node.Content[i+1], rest, childPath, errs)
			return
		}
	}
	// Key not found at this level — treat as a dict-of-structs: check all values.
	for i := 0; i+1 < len(node.Content); i += 2 {
		dictKey := node.Content[i].Value
		childPath := dictKey
		if path != "" {
			childPath = path + "." + dictKey
		}
		v.navigate(node.Content[i+1], segs, childPath, errs)
	}
}

// check verifies mutual exclusivity of v.keys within a MappingNode.
func (v *pathMutuallyExclusiveValidator) check(node *yaml.Node, path string, errs *[]string) {
	if node.Kind != yaml.MappingNode {
		return
	}
	var present []string
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i].Value
		for _, want := range v.keys {
			if k == want {
				present = append(present, k)
			}
		}
	}
	if len(present) > 1 {
		where := path
		if where == "" {
			where = strings.Join(v.parentSegs, ".")
		}
		*errs = append(*errs, fmt.Sprintf(
			"%s: mutually exclusive — use only one of: %s",
			where, joinQuoted(present),
		))
	}
}

// RequiredWith reports a violation when key is present but parent is not.
func RequiredWith(key, parent string) Validator {
	return &requiredWithValidator{key: key, parent: parent}
}

type requiredWithValidator struct{ key, parent string }

func (v *requiredWithValidator) Validate(_ []byte, blocks []document.Block) []string {
	present := keysPresent(blocks)
	if present[v.key] && !present[v.parent] {
		return []string{fmt.Sprintf(
			"%q requires %q to be set", v.key, v.parent,
		)}
	}
	return nil
}

// MutuallyExclusiveNested walks the YAML tree and fires at every mapping whose
// direct parent key is the last segment of scopedPath, checking that at most
// one of keys is present.
//
// Two forms:
//
// Single key — searches the entire document (backward-compatible):
//
//	editor.MutuallyExclusiveNested("filter", "any", "all")
//
// Dotted path — navigates to the scoped root first, then recurses only within
// that subtree. The last segment is the key name used for recursive matching.
// Sequences and dict-style mappings along the path are expanded automatically:
//
//	editor.MutuallyExclusiveNested("categories.installers.source.filter", "any", "all")
//
// The scoped form is preferred when the constraint applies to a specific filter
// type and not to every mapping named "filter" in the document.
func MutuallyExclusiveNested(scopedPath string, keys ...string) Validator {
	segs := strings.Split(scopedPath, ".")
	return &mutuallyExclusiveNestedValidator{
		navSegs:   segs[:len(segs)-1],
		parentKey: segs[len(segs)-1],
		keys:      keys,
	}
}

type mutuallyExclusiveNestedValidator struct {
	navSegs   []string // path segments to navigate before starting recursive walk (may be empty)
	parentKey string   // recurse on mappings whose direct parent key equals this
	keys      []string
}

func (v *mutuallyExclusiveNestedValidator) Validate(raw []byte, _ []document.Block) []string {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil || len(doc.Content) == 0 {
		return nil
	}
	var errs []string
	v.navigateThenWalk(doc.Content[0], v.navSegs, "", &errs)
	return errs
}

// navigateThenWalk follows navSegs to reach the scoped subtree root, expanding
// sequences and dict-style mappings along the way, then hands off to walk.
// When navSegs is empty it calls walk immediately (whole-document search).
func (v *mutuallyExclusiveNestedValidator) navigateThenWalk(node *yaml.Node, segs []string, path string, errs *[]string) {
	if node.Kind == yaml.SequenceNode {
		for i, item := range node.Content {
			v.navigateThenWalk(item, segs, fmt.Sprintf("%s[%d]", path, i), errs)
		}
		return
	}
	if len(segs) == 0 {
		v.walk(node, "", path, errs)
		return
	}
	if node.Kind != yaml.MappingNode {
		return
	}
	next, rest := segs[0], segs[1:]
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == next {
			childPath := next
			if path != "" {
				childPath = path + "." + next
			}
			v.navigateThenWalk(node.Content[i+1], rest, childPath, errs)
			return
		}
	}
	// Key not found — treat as a dict-of-structs and expand all values.
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i].Value
		childPath := k
		if path != "" {
			childPath = path + "." + k
		}
		v.navigateThenWalk(node.Content[i+1], segs, childPath, errs)
	}
}

// walk visits node recursively. parentKey is the mapping key whose value is
// node; path is the dot-separated YAML path to node (for error messages).
func (v *mutuallyExclusiveNestedValidator) walk(node *yaml.Node, parentKey, path string, errs *[]string) {
	switch node.Kind {
	case yaml.MappingNode:
		if parentKey == v.parentKey {
			var present []string
			for i := 0; i+1 < len(node.Content); i += 2 {
				k := node.Content[i].Value
				for _, want := range v.keys {
					if k == want {
						present = append(present, k)
					}
				}
			}
			if len(present) > 1 {
				where := path
				if where == "" {
					where = v.parentKey
				}
				*errs = append(*errs, fmt.Sprintf(
					"%s: mutually exclusive — use only one of: %s",
					where, joinQuoted(present),
				))
			}
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			v.walk(node.Content[i+1], key, childPath, errs)
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			v.walk(child, parentKey, fmt.Sprintf("%s[%d]", path, i), errs)
		}
	}
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
