package editor

import (
	"fmt"
	"strings"
	"time"

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
	navigateYAML(doc.Content[0], v.parentSegs, "", &errs, v.check)
	return errs
}

func (v *pathMutuallyExclusiveValidator) check(node *yaml.Node, path string, errs *[]string) {
	if node.Kind != yaml.MappingNode {
		return
	}
	where := path
	if where == "" {
		where = strings.Join(v.parentSegs, ".")
	}
	checkMutualExclusion(node, v.keys, where, errs)
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
	navigateYAML(doc.Content[0], v.navSegs, "", &errs, func(n *yaml.Node, p string, e *[]string) {
		v.walk(n, "", p, e)
	})
	return errs
}

// walk visits node recursively. parentKey is the mapping key whose value is
// node; path is the dot-separated YAML path to node (for error messages).
func (v *mutuallyExclusiveNestedValidator) walk(node *yaml.Node, parentKey, path string, errs *[]string) {
	switch node.Kind {
	case yaml.MappingNode:
		if parentKey == v.parentKey {
			where := path
			if where == "" {
				where = v.parentKey
			}
			checkMutualExclusion(node, v.keys, where, errs)
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

// navigateYAML traverses node following segs, expanding sequences and
// dict-of-structs automatically. onArrival is called when segs is exhausted.
func navigateYAML(node *yaml.Node, segs []string, path string, errs *[]string,
	onArrival func(*yaml.Node, string, *[]string)) {
	if node.Kind == yaml.SequenceNode {
		for i, item := range node.Content {
			navigateYAML(item, segs, fmt.Sprintf("%s[%d]", path, i), errs, onArrival)
		}
		return
	}
	if len(segs) == 0 {
		onArrival(node, path, errs)
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
			navigateYAML(node.Content[i+1], rest, childPath, errs, onArrival)
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
		navigateYAML(node.Content[i+1], segs, childPath, errs, onArrival)
	}
}

// checkMutualExclusion appends to errs when more than one of keys appears as a
// direct child key of node (which must be a MappingNode). where is the
// dot-separated path used in the error message.
func checkMutualExclusion(node *yaml.Node, keys []string, where string, errs *[]string) {
	var present []string
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i].Value
		for _, want := range keys {
			if k == want {
				present = append(present, k)
				break
			}
		}
	}
	if len(present) > 1 {
		*errs = append(*errs, fmt.Sprintf(
			"%s: mutually exclusive — use only one of: %s",
			where, joinQuoted(present),
		))
	}
}

// AtLeastOneOf reports a violation when none of the listed keys is present.
func AtLeastOneOf(keys ...string) Validator {
	return &atLeastOneOfValidator{keys: keys}
}

type atLeastOneOfValidator struct{ keys []string }

func (v *atLeastOneOfValidator) Validate(_ []byte, blocks []document.Block) []string {
	present := keysPresent(blocks)
	for _, k := range v.keys {
		if present[k] {
			return nil
		}
	}
	return []string{fmt.Sprintf("at least one of %s is required", joinQuoted(v.keys))}
}

// ExactlyOneOf reports a violation when none or more than one of the listed keys is present.
func ExactlyOneOf(keys ...string) Validator {
	return &exactlyOneOfValidator{keys: keys}
}

type exactlyOneOfValidator struct{ keys []string }

func (v *exactlyOneOfValidator) Validate(_ []byte, blocks []document.Block) []string {
	present := keysPresent(blocks)
	var found []string
	for _, k := range v.keys {
		if present[k] {
			found = append(found, k)
		}
	}
	switch len(found) {
	case 1:
		return nil
	case 0:
		return []string{fmt.Sprintf("exactly one of %s is required", joinQuoted(v.keys))}
	default:
		return []string{fmt.Sprintf(
			"exactly one of %s must be set — found: %s",
			joinQuoted(v.keys), joinQuoted(found),
		)}
	}
}

// RequiredIf reports a violation when key is absent but condPath equals condValue.
func RequiredIf(key, condPath, condValue string) Validator {
	return &requiredIfValidator{key: key, condPath: condPath, condValue: condValue}
}

type requiredIfValidator struct{ key, condPath, condValue string }

func (v *requiredIfValidator) Validate(raw []byte, _ []document.Block) []string {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil || len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	if scalarAt(root, strings.Split(v.condPath, ".")) != v.condValue {
		return nil
	}
	if scalarAt(root, strings.Split(v.key, ".")) == "" {
		return []string{fmt.Sprintf("%q is required when %q is %q", v.key, v.condPath, v.condValue)}
	}
	return nil
}

// ValueOneOf reports a violation when the field at path exists but its value is not in allowed.
func ValueOneOf(path string, allowed ...string) Validator {
	return &valueOneOfValidator{path: path, allowed: allowed}
}

type valueOneOfValidator struct {
	path    string
	allowed []string
}

func (v *valueOneOfValidator) Validate(raw []byte, _ []document.Block) []string {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil || len(doc.Content) == 0 {
		return nil
	}
	val := scalarAt(doc.Content[0], strings.Split(v.path, "."))
	if val == "" {
		return nil
	}
	for _, a := range v.allowed {
		if val == a {
			return nil
		}
	}
	return []string{fmt.Sprintf(
		"%q: value %q is not allowed — use one of: %s",
		v.path, val, joinQuoted(v.allowed),
	)}
}

// CrossFieldOrdered reports a violation when both paths are present but the value
// at smallerPath is not strictly less than the value at largerPath.
// Values are compared as time.Duration strings (e.g. "24h") or size strings (e.g. "10MB").
func CrossFieldOrdered(smallerPath, largerPath string) Validator {
	return &crossFieldOrderedValidator{smallerPath: smallerPath, largerPath: largerPath}
}

type crossFieldOrderedValidator struct{ smallerPath, largerPath string }

func (v *crossFieldOrderedValidator) Validate(raw []byte, _ []document.Block) []string {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil || len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	aStr := scalarAt(root, strings.Split(v.smallerPath, "."))
	bStr := scalarAt(root, strings.Split(v.largerPath, "."))
	if aStr == "" || bStr == "" {
		return nil
	}
	a, b, ok := parseOrderedPair(aStr, bStr)
	if !ok {
		return nil
	}
	if a >= b {
		return []string{fmt.Sprintf(
			"%q (%s) must be less than %q (%s)",
			v.smallerPath, aStr, v.largerPath, bStr,
		)}
	}
	return nil
}

// NoDuplicates reports a violation when two or more items in the sequence at seqPath
// share the same value for field.
func NoDuplicates(seqPath, field string) Validator {
	return &noDuplicatesValidator{seqPath: seqPath, field: field}
}

type noDuplicatesValidator struct{ seqPath, field string }

func (v *noDuplicatesValidator) Validate(raw []byte, _ []document.Block) []string {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil || len(doc.Content) == 0 {
		return nil
	}
	seqNode := nodeAtStr(doc.Content[0], strings.Split(v.seqPath, "."))
	if seqNode == nil || seqNode.Kind != yaml.SequenceNode {
		return nil
	}
	seen := make(map[string]int)
	var errs []string
	for i, item := range seqNode.Content {
		val := scalarAt(item, []string{v.field})
		if val == "" {
			continue
		}
		if firstIdx, dup := seen[val]; dup {
			errs = append(errs, fmt.Sprintf(
				"%s[%d].%s: duplicate value %q (first seen at %s[%d])",
				v.seqPath, i, v.field, val, v.seqPath, firstIdx,
			))
		} else {
			seen[val] = i
		}
	}
	return errs
}

// scalarAt navigates node following segs and returns the scalar value at the terminal node.
// Returns "" when the path does not exist or the terminal node is not a scalar.
func scalarAt(node *yaml.Node, segs []string) string {
	if len(segs) == 0 {
		if node.Kind == yaml.ScalarNode {
			return node.Value
		}
		return ""
	}
	if node.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == segs[0] {
			return scalarAt(node.Content[i+1], segs[1:])
		}
	}
	return ""
}

// nodeAtStr navigates node following string segs and returns the terminal yaml.Node.
// Returns nil when the path does not exist.
func nodeAtStr(node *yaml.Node, segs []string) *yaml.Node {
	if len(segs) == 0 {
		return node
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == segs[0] {
			return nodeAtStr(node.Content[i+1], segs[1:])
		}
	}
	return nil
}

// parseOrderedPair tries to parse a and b as comparable int64 values.
// It tries time.Duration first, then size strings (B/KB/MB/GB/TB).
func parseOrderedPair(a, b string) (int64, int64, bool) {
	da, errA := time.ParseDuration(a)
	db, errB := time.ParseDuration(b)
	if errA == nil && errB == nil {
		return int64(da), int64(db), true
	}
	sa, okA := parseSize(a)
	sb, okB := parseSize(b)
	if okA && okB {
		return sa, sb, true
	}
	return 0, 0, false
}

// sizeUnits maps suffix → byte multiplier, ordered longest-suffix-first to
// avoid "B" matching before "MB" or "GB".
var sizeUnits = []struct {
	suffix string
	mult   int64
}{
	{"TB", 1024 * 1024 * 1024 * 1024},
	{"GB", 1024 * 1024 * 1024},
	{"MB", 1024 * 1024},
	{"KB", 1024},
	{"B", 1},
}

// parseSize parses strings like "10MB", "500KB", "1.5GB".
func parseSize(s string) (int64, bool) {
	upper := strings.TrimSpace(strings.ToUpper(s))
	for _, u := range sizeUnits {
		if strings.HasSuffix(upper, u.suffix) {
			numStr := strings.TrimSpace(strings.TrimSuffix(upper, u.suffix))
			var n float64
			if _, err := fmt.Sscanf(numStr, "%f", &n); err == nil && n >= 0 {
				return int64(n * float64(u.mult)), true
			}
		}
	}
	return 0, false
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
