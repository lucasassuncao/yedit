package editor

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/document"
	"github.com/lucasassuncao/yedit/schema"
)

// RunAll executes all validators against raw/blocks and collects violations.
func RunAll(validators []Validator, raw []byte, blocks []document.Block) []Violation {
	if len(validators) == 0 {
		return nil
	}
	var errs []Violation
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

func (v *mutuallyExclusiveValidator) Validate(_ []byte, blocks []document.Block) []Violation {
	present := keysPresent(blocks)
	var found []string
	for _, k := range v.keys {
		if present[k] {
			found = append(found, k)
		}
	}
	if len(found) > 1 {
		return []Violation{{Message: fmt.Sprintf(
			"mutually exclusive — use only one of: %s",
			joinQuoted(found),
		)}}
	}
	return nil
}

// newPathMutuallyExclusiveValidator builds the path-aware variant. All keys
// must share the same parent path (everything before the last dot). The leaf
// segments (last component of each path) become the mutually exclusive keys.
func newPathMutuallyExclusiveValidator(fullPaths []string) Validator {
	parent, leaves, ok := splitSharedParent(fullPaths)
	if !ok {
		// Mismatched depth or parent — fall back to treating them as plain keys.
		return &mutuallyExclusiveValidator{keys: fullPaths}
	}
	return &pathMutuallyExclusiveValidator{parentSegs: parent, keys: leaves}
}

// splitSharedParent splits dotted paths that all share the same parent prefix
// into (parentSegs, leaves). ok is false when the paths have different depths
// or diverging parents.
func splitSharedParent(fullPaths []string) (parent, leaves []string, ok bool) {
	segs := make([][]string, len(fullPaths))
	for i, p := range fullPaths {
		segs[i] = strings.Split(p, ".")
	}
	parent = segs[0][:len(segs[0])-1]
	for _, s := range segs[1:] {
		if len(s) != len(segs[0]) {
			return nil, nil, false
		}
		for j, seg := range s[:len(s)-1] {
			if seg != parent[j] {
				return nil, nil, false
			}
		}
	}
	leaves = make([]string, len(segs))
	for i, s := range segs {
		leaves[i] = s[len(s)-1]
	}
	return parent, leaves, true
}

type pathMutuallyExclusiveValidator struct {
	parentSegs []string // path segments to the parent mapping
	keys       []string // mutually exclusive leaf keys within that mapping
}

func (v *pathMutuallyExclusiveValidator) Validate(raw []byte, _ []document.Block) []Violation {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil || len(doc.Content) == 0 {
		return nil
	}
	var errs []Violation
	navigateYAML(doc.Content[0], v.parentSegs, "", &errs, v.check)
	return errs
}

func (v *pathMutuallyExclusiveValidator) check(node *yaml.Node, path string, errs *[]Violation) {
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

func (v *requiredWithValidator) Validate(_ []byte, blocks []document.Block) []Violation {
	present := keysPresent(blocks)
	if present[v.key] && !present[v.parent] {
		return []Violation{{Message: fmt.Sprintf(
			"%q requires %q to be set", v.key, v.parent,
		)}}
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

func (v *mutuallyExclusiveNestedValidator) Validate(raw []byte, _ []document.Block) []Violation {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil || len(doc.Content) == 0 {
		return nil
	}
	var errs []Violation
	navigateYAML(doc.Content[0], v.navSegs, "", &errs, func(n *yaml.Node, p string, e *[]Violation) {
		v.walk(n, "", p, e)
	})
	return errs
}

// walk visits node recursively. parentKey is the mapping key whose value is
// node; path is the dot-separated YAML path to node (for error messages).
func (v *mutuallyExclusiveNestedValidator) walk(node *yaml.Node, parentKey, path string, errs *[]Violation) {
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
func navigateYAML(node *yaml.Node, segs []string, path string, errs *[]Violation,
	onArrival func(*yaml.Node, string, *[]Violation)) {
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
// dot-separated path reported in the violation.
func checkMutualExclusion(node *yaml.Node, keys []string, where string, errs *[]Violation) {
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
		*errs = append(*errs, Violation{
			Path: where,
			Message: fmt.Sprintf(
				"mutually exclusive — use only one of: %s",
				joinQuoted(present),
			),
		})
	}
}

// AtLeastOneOf reports a violation when none of the listed keys is present.
func AtLeastOneOf(keys ...string) Validator {
	return &atLeastOneOfValidator{keys: keys}
}

type atLeastOneOfValidator struct{ keys []string }

func (v *atLeastOneOfValidator) Validate(_ []byte, blocks []document.Block) []Violation {
	present := keysPresent(blocks)
	for _, k := range v.keys {
		if present[k] {
			return nil
		}
	}
	return []Violation{{Message: fmt.Sprintf("at least one of %s is required", joinQuoted(v.keys))}}
}

// ExactlyOneOf reports a violation when none or more than one of the listed keys is present.
func ExactlyOneOf(keys ...string) Validator {
	return &exactlyOneOfValidator{keys: keys}
}

type exactlyOneOfValidator struct{ keys []string }

func (v *exactlyOneOfValidator) Validate(_ []byte, blocks []document.Block) []Violation {
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
		return []Violation{{Message: fmt.Sprintf("exactly one of %s is required", joinQuoted(v.keys))}}
	default:
		return []Violation{{Message: fmt.Sprintf(
			"exactly one of %s must be set — found: %s",
			joinQuoted(v.keys), joinQuoted(found),
		)}}
	}
}

// RequiredIf reports a violation when key is absent but condPath equals condValue.
//
// When key and condPath share the same parent prefix, the rule is evaluated
// inside every mapping reached by that parent — sequences and dict-style
// mappings are expanded automatically, so each entry is checked against its
// own condition value:
//
//	// every servers[n] with protocol https needs its own tls-cert
//	editor.RequiredIf("servers.tls-cert", "servers.protocol", "https")
//
// Paths with unrelated parents are both resolved from the document root.
func RequiredIf(key, condPath, condValue string) Validator {
	return &requiredIfValidator{key: key, condPath: condPath, condValue: condValue}
}

type requiredIfValidator struct{ key, condPath, condValue string }

func (v *requiredIfValidator) Validate(raw []byte, _ []document.Block) []Violation {
	root, ok := rootMapping(raw)
	if !ok {
		return nil
	}
	var errs []Violation
	if parent, leaves, shared := splitSharedParent([]string{v.key, v.condPath}); shared {
		keyLeaf, condLeaf := leaves[0], leaves[1]
		navigateYAML(root, parent, "", &errs, func(n *yaml.Node, p string, e *[]Violation) {
			if n.Kind != yaml.MappingNode || scalarChild(n, condLeaf) != v.condValue {
				return
			}
			// A non-scalar value (mapping/sequence) counts as present; only a
			// missing key or an empty scalar is a violation.
			if !presentNonEmpty(childOf(n, keyLeaf)) {
				*e = append(*e, Violation{
					Path:    joinPath(p, keyLeaf),
					Message: fmt.Sprintf("required when %q is %q", v.condPath, v.condValue),
				})
			}
		})
		return errs
	}
	// Unrelated parents: both paths are resolved from the root.
	if scalarAt(root, strings.Split(v.condPath, ".")) != v.condValue {
		return nil
	}
	if !presentNonEmpty(nodeAtStr(root, strings.Split(v.key, "."))) {
		errs = append(errs, Violation{
			Path:    v.key,
			Message: fmt.Sprintf("required when %q is %q", v.condPath, v.condValue),
		})
	}
	return errs
}

// ValueOneOf reports a violation when the field at path exists but its value
// is not in allowed. Sequences and dict-style mappings along the path are
// expanded automatically, so every entry in a list or every value in a map is
// checked.
func ValueOneOf(path string, allowed ...string) Validator {
	return &valueOneOfValidator{path: path, allowed: allowed}
}

type valueOneOfValidator struct {
	path    string
	allowed []string
}

func (v *valueOneOfValidator) Validate(raw []byte, _ []document.Block) []Violation {
	root, ok := rootMapping(raw)
	if !ok {
		return nil
	}
	var errs []Violation
	forEachLeaf(root, v.path, &errs, func(node *yaml.Node, where string, e *[]Violation) {
		// A mapping or sequence can never match a scalar from the allowed set —
		// flag it instead of silently treating it as absent.
		if node.Kind != yaml.ScalarNode {
			*e = append(*e, Violation{
				Path:    where,
				Message: fmt.Sprintf("expected a scalar value — use one of: %s", joinQuoted(v.allowed)),
			})
			return
		}
		if node.Value == "" {
			return
		}
		for _, a := range v.allowed {
			if node.Value == a {
				return
			}
		}
		*e = append(*e, Violation{
			Path:    where,
			Message: fmt.Sprintf("value %q is not allowed — use one of: %s", node.Value, joinQuoted(v.allowed)),
		})
	})
	return errs
}

// CrossFieldOrdered reports a violation when both paths are present but the value
// at smallerPath is not strictly less than the value at largerPath.
// Values are compared as plain numbers ("1", "0.5"), time.Duration strings
// (e.g. "24h"), or size strings; both sides must be of the same kind.
// Size suffixes follow their standard meaning: KB/MB/GB/TB are decimal
// (powers of 1000) and KiB/MiB/GiB/TiB are binary (powers of 1024).
//
// When the two paths share the same parent prefix, the pair is compared inside
// every mapping reached by that parent — sequences and dict-style mappings are
// expanded automatically, so each entry's own min/max pair is checked. Paths
// with unrelated parents are both resolved from the document root.
func CrossFieldOrdered(smallerPath, largerPath string) Validator {
	return &crossFieldOrderedValidator{smallerPath: smallerPath, largerPath: largerPath}
}

type crossFieldOrderedValidator struct{ smallerPath, largerPath string }

func (v *crossFieldOrderedValidator) Validate(raw []byte, _ []document.Block) []Violation {
	root, ok := rootMapping(raw)
	if !ok {
		return nil
	}
	var errs []Violation
	if parent, leaves, shared := splitSharedParent([]string{v.smallerPath, v.largerPath}); shared {
		smallLeaf, largeLeaf := leaves[0], leaves[1]
		navigateYAML(root, parent, "", &errs, func(n *yaml.Node, p string, e *[]Violation) {
			if n.Kind != yaml.MappingNode {
				return
			}
			checkOrdered(scalarChild(n, smallLeaf), scalarChild(n, largeLeaf), smallLeaf, largeLeaf, p, e)
		})
		return errs
	}
	// Unrelated parents: both paths are resolved from the root.
	aStr := scalarAt(root, strings.Split(v.smallerPath, "."))
	bStr := scalarAt(root, strings.Split(v.largerPath, "."))
	checkOrdered(aStr, bStr, v.smallerPath, v.largerPath, "", &errs)
	return errs
}

// checkOrdered appends a violation when both values are present, comparable,
// and aStr is not strictly less than bStr. aName/bName label the two fields in
// the message; where is the violation path (may be empty).
func checkOrdered(aStr, bStr, aName, bName, where string, errs *[]Violation) {
	if aStr == "" || bStr == "" {
		return
	}
	a, b, ok := parseOrderedPair(aStr, bStr)
	if !ok {
		return
	}
	if a >= b {
		*errs = append(*errs, Violation{
			Path:    where,
			Message: fmt.Sprintf("%q (%s) must be less than %q (%s)", aName, aStr, bName, bStr),
		})
	}
}

// NoDuplicates reports a violation when two or more items in the sequence at seqPath
// share the same value for field.
func NoDuplicates(seqPath, field string) Validator {
	return &noDuplicatesValidator{seqPath: seqPath, field: field}
}

type noDuplicatesValidator struct{ seqPath, field string }

func (v *noDuplicatesValidator) Validate(raw []byte, _ []document.Block) []Violation {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil || len(doc.Content) == 0 {
		return nil
	}
	seqNode := nodeAtStr(doc.Content[0], strings.Split(v.seqPath, "."))
	if seqNode == nil || seqNode.Kind != yaml.SequenceNode {
		return nil
	}
	seen := make(map[string]int)
	var errs []Violation
	for i, item := range seqNode.Content {
		val := scalarAt(item, []string{v.field})
		if val == "" {
			continue
		}
		if firstIdx, dup := seen[val]; dup {
			errs = append(errs, Violation{
				Path:    fmt.Sprintf("%s[%d].%s", v.seqPath, i, v.field),
				Message: fmt.Sprintf("duplicate value %q (first seen at %s[%d])", val, v.seqPath, firstIdx),
			})
		} else {
			seen[val] = i
		}
	}
	return errs
}

// Required reports a violation when any of the given paths is absent or holds
// an empty/null scalar. A non-scalar value (mapping or sequence) counts as
// present.
//
// A path with no dots is required unconditionally at the document root. A
// dotted path is conditional: the validator navigates to the leaf's parent —
// expanding sequences and dict-style mappings like MutuallyExclusive — and
// only requires the leaf where that parent exists, so a required field inside
// an optional block is not reported while the block is absent.
//
//	editor.Required("version")          // top-level, unconditional
//	editor.Required("categories.name")  // every category entry needs "name"
//
// To enforce the schema's validate:"required" tags without listing paths by
// hand, use RequiredFromSchema.
func Required(paths ...string) Validator {
	return &requiredValidator{paths: paths}
}

type requiredValidator struct{ paths []string }

func (v *requiredValidator) Validate(raw []byte, _ []document.Block) []Violation {
	root, ok := rootMapping(raw)
	if !ok {
		return nil
	}
	var errs []Violation
	for _, p := range v.paths {
		// Unlike forEachLeaf, Required must see absent leaves, so it navigates to
		// the leaf's parent and checks the leaf there. The dict-of-structs
		// fallback therefore applies to intermediate segments only.
		segs := strings.Split(p, ".")
		parent, leaf := segs[:len(segs)-1], segs[len(segs)-1]
		navigateYAML(root, parent, "", &errs, func(n *yaml.Node, path string, e *[]Violation) {
			if n.Kind != yaml.MappingNode {
				return
			}
			if !presentNonEmpty(childOf(n, leaf)) {
				*e = append(*e, Violation{Path: joinPath(path, leaf), Message: "required"})
			}
		})
	}
	return errs
}

// RequiredFromSchema enforces the schema's required markers
// (validate:"required" / jsonschema:"required") at validate/save time. Without
// it the marker is display-only: the "*" in the tree and the "Required: yes"
// hint line do not block saving.
//
// A required field is only enforced where its parent exists — a required field
// inside an optional block is not reported while the whole block is absent.
// Top-level required fields are always enforced. Sequence and dictionary
// entries are checked individually.
//
// The editor wires the discovered schema into this validator when the session
// starts; outside editor.Run it reports nothing.
func RequiredFromSchema() Validator { return &requiredFromSchemaValidator{} }

type requiredFromSchemaValidator struct{ defs []schema.FieldDef }

func (v *requiredFromSchemaValidator) Validate(raw []byte, _ []document.Block) []Violation {
	if len(v.defs) == 0 {
		return nil
	}
	root, ok := rootMapping(raw)
	if !ok {
		return nil
	}
	var errs []Violation
	checkRequiredDefs(root, v.defs, "", &errs)
	return errs
}

// checkRequiredDefs walks node guided by defs, reporting every required field
// that is absent or an empty scalar, and recursing into present children
// according to their schema kind.
func checkRequiredDefs(node *yaml.Node, defs []schema.FieldDef, path string, errs *[]Violation) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	for _, def := range defs {
		child := childOf(node, def.YAMLName)
		childPath := joinPath(path, def.YAMLName)
		if def.Required && !presentNonEmpty(child) {
			*errs = append(*errs, Violation{Path: childPath, Message: "required"})
		}
		// KindVariant children describe union alternatives, not required structure.
		if child == nil || len(def.Children) == 0 || def.Kind == schema.KindVariant {
			continue
		}
		switch def.Kind {
		case schema.KindObject:
			checkRequiredDefs(child, def.Children, childPath, errs)
		case schema.KindList:
			if child.Kind == yaml.SequenceNode {
				for i, item := range child.Content {
					checkRequiredDefs(item, def.Children, fmt.Sprintf("%s[%d]", childPath, i), errs)
				}
			}
		case schema.KindDictionary:
			if child.Kind == yaml.MappingNode {
				for i := 0; i+1 < len(child.Content); i += 2 {
					checkRequiredDefs(child.Content[i+1], def.Children, joinPath(childPath, child.Content[i].Value), errs)
				}
			}
		}
	}
}

// ValueInRange reports a violation when the scalar at path is present but
// outside the inclusive [min, max] range. Bounds and value may be plain
// numbers ("1", "0.5"), time.Duration strings ("24h"), or size strings
// ("10MB", "256MiB" — KB/MB/GB/TB decimal, KiB/MiB/GiB/TiB binary); all three
// must be of the same kind. An absent or empty value reports nothing —
// combine with Required when the field is mandatory.
//
//	editor.ValueInRange("server.port", "1", "65535")
//	editor.ValueInRange("filter.max-age", "1h", "8760h")
func ValueInRange(path, minVal, maxVal string) Validator {
	return &valueInRangeValidator{path: path, min: minVal, max: maxVal}
}

type valueInRangeValidator struct{ path, min, max string }

func (v *valueInRangeValidator) Validate(raw []byte, _ []document.Block) []Violation {
	root, ok := rootMapping(raw)
	if !ok {
		return nil
	}
	lo, loKind, okLo := parseComparable(v.min)
	hi, hiKind, okHi := parseComparable(v.max)
	if !okLo || !okHi || loKind != hiKind {
		return []Violation{{
			Path:    v.path,
			Message: fmt.Sprintf("invalid range [%s, %s] — bounds must both be durations, sizes, or numbers", v.min, v.max),
		}}
	}
	var errs []Violation
	forEachLeaf(root, v.path, &errs, func(node *yaml.Node, where string, e *[]Violation) {
		if node.Kind != yaml.ScalarNode {
			*e = append(*e, Violation{Path: where, Message: "expected a scalar value"})
			return
		}
		if node.Value == "" {
			return
		}
		val, kind, okVal := parseComparable(node.Value)
		if !okVal || kind != loKind {
			*e = append(*e, Violation{
				Path:    where,
				Message: fmt.Sprintf("value %q is not comparable with range [%s, %s]", node.Value, v.min, v.max),
			})
			return
		}
		if val < lo || val > hi {
			*e = append(*e, Violation{
				Path:    where,
				Message: fmt.Sprintf("value %q out of range [%s, %s]", node.Value, v.min, v.max),
			})
		}
	})
	return errs
}

// ValueMatches reports a violation when the scalar at path is present but does
// not match the regular expression pattern. An absent or empty value reports
// nothing — combine with Required when the field is mandatory. An invalid
// pattern is itself reported as a violation so the misconfiguration surfaces
// on the first validate.
//
//	editor.ValueMatches("version", `^\d+\.\d+\.\d+$`)
func ValueMatches(path, pattern string) Validator {
	re, err := regexp.Compile(pattern)
	return &valueMatchesValidator{path: path, pattern: pattern, re: re, err: err}
}

type valueMatchesValidator struct {
	path    string
	pattern string
	re      *regexp.Regexp
	err     error // non-nil when pattern failed to compile
}

func (v *valueMatchesValidator) Validate(raw []byte, _ []document.Block) []Violation {
	if v.err != nil {
		return []Violation{{Path: v.path, Message: fmt.Sprintf("invalid pattern %q: %v", v.pattern, v.err)}}
	}
	root, ok := rootMapping(raw)
	if !ok {
		return nil
	}
	var errs []Violation
	forEachLeaf(root, v.path, &errs, func(node *yaml.Node, where string, e *[]Violation) {
		if node.Kind != yaml.ScalarNode {
			*e = append(*e, Violation{Path: where, Message: "expected a scalar value"})
			return
		}
		if node.Value == "" {
			return
		}
		if !v.re.MatchString(node.Value) {
			*e = append(*e, Violation{
				Path:    where,
				Message: fmt.Sprintf("value %q does not match pattern %q", node.Value, v.pattern),
			})
		}
	})
	return errs
}

// AllOrNone reports a violation when only some of the listed keys are present:
// they must appear together or not at all (e.g. a TLS cert/key pair).
//
// Like MutuallyExclusive it supports two forms: plain keys are checked against
// the document's top-level blocks, and dotted paths — all sharing the same
// parent prefix — are checked inside every mapping reached by that parent,
// with sequences and dict-style mappings expanded automatically:
//
//	editor.AllOrNone("tls-cert", "tls-key")
//	editor.AllOrNone("server.tls-cert", "server.tls-key")
func AllOrNone(keys ...string) Validator {
	for _, k := range keys {
		if strings.Contains(k, ".") {
			parent, leaves, ok := splitSharedParent(keys)
			if !ok {
				return &allOrNoneValidator{keys: keys}
			}
			return &pathAllOrNoneValidator{parentSegs: parent, keys: leaves}
		}
	}
	return &allOrNoneValidator{keys: keys}
}

type allOrNoneValidator struct{ keys []string }

func (v *allOrNoneValidator) Validate(_ []byte, blocks []document.Block) []Violation {
	present := keysPresent(blocks)
	var found, missing []string
	for _, k := range v.keys {
		if present[k] {
			found = append(found, k)
		} else {
			missing = append(missing, k)
		}
	}
	if len(found) > 0 && len(missing) > 0 {
		return []Violation{{Message: fmt.Sprintf(
			"all or none of %s must be set — missing: %s",
			joinQuoted(v.keys), joinQuoted(missing),
		)}}
	}
	return nil
}

type pathAllOrNoneValidator struct {
	parentSegs []string // path segments to the parent mapping
	keys       []string // leaf keys that must appear together within that mapping
}

func (v *pathAllOrNoneValidator) Validate(raw []byte, _ []document.Block) []Violation {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil || len(doc.Content) == 0 {
		return nil
	}
	var errs []Violation
	navigateYAML(doc.Content[0], v.parentSegs, "", &errs, v.check)
	return errs
}

func (v *pathAllOrNoneValidator) check(node *yaml.Node, path string, errs *[]Violation) {
	if node.Kind != yaml.MappingNode {
		return
	}
	var found, missing []string
	for _, k := range v.keys {
		if childOf(node, k) != nil {
			found = append(found, k)
		} else {
			missing = append(missing, k)
		}
	}
	if len(found) > 0 && len(missing) > 0 {
		where := path
		if where == "" {
			where = strings.Join(v.parentSegs, ".")
		}
		*errs = append(*errs, Violation{
			Path: where,
			Message: fmt.Sprintf(
				"all or none of %s must be set — missing: %s",
				joinQuoted(v.keys), joinQuoted(missing),
			),
		})
	}
}

// CountRange reports a violation when the collection at path has fewer than
// minCount or more than maxCount entries. maxCount < 0 means no upper bound.
// Sequences count items; mappings count keys. An absent path reports nothing —
// combine with Required when the collection itself is mandatory.
//
//	editor.CountRange("workers", 1, 10)
//	editor.CountRange("categories", 1, -1) // at least one, no upper bound
func CountRange(path string, minCount, maxCount int) Validator {
	return &countRangeValidator{path: path, min: minCount, max: maxCount}
}

type countRangeValidator struct {
	path     string
	min, max int
}

func (v *countRangeValidator) Validate(raw []byte, _ []document.Block) []Violation {
	root, ok := rootMapping(raw)
	if !ok {
		return nil
	}
	var errs []Violation
	forEachLeaf(root, v.path, &errs, func(node *yaml.Node, where string, e *[]Violation) {
		var count int
		switch node.Kind {
		case yaml.SequenceNode:
			count = len(node.Content)
		case yaml.MappingNode:
			count = len(node.Content) / 2
		default:
			*e = append(*e, Violation{Path: where, Message: "expected a list or mapping"})
			return
		}
		if count < v.min || (v.max >= 0 && count > v.max) {
			want := fmt.Sprintf("between %d and %d", v.min, v.max)
			if v.max < 0 {
				want = fmt.Sprintf("at least %d", v.min)
			}
			*e = append(*e, Violation{
				Path:    where,
				Message: fmt.Sprintf("has %d entries — expected %s", count, want),
			})
		}
	})
	return errs
}

// UniqueValues reports a violation when two or more scalar items in the
// sequence at seqPath share the same value. Non-scalar items are skipped — use
// NoDuplicates to deduplicate struct entries by one of their fields.
//
//	editor.UniqueValues("tags")
func UniqueValues(seqPath string) Validator {
	return &uniqueValuesValidator{seqPath: seqPath}
}

type uniqueValuesValidator struct{ seqPath string }

func (v *uniqueValuesValidator) Validate(raw []byte, _ []document.Block) []Violation {
	root, ok := rootMapping(raw)
	if !ok {
		return nil
	}
	var errs []Violation
	forEachLeaf(root, v.seqPath, &errs, func(seqNode *yaml.Node, where string, e *[]Violation) {
		if seqNode.Kind != yaml.SequenceNode {
			return
		}
		seen := make(map[string]int)
		for i, item := range seqNode.Content {
			if item.Kind != yaml.ScalarNode || item.Value == "" {
				continue
			}
			if firstIdx, dup := seen[item.Value]; dup {
				*e = append(*e, Violation{
					Path:    fmt.Sprintf("%s[%d]", where, i),
					Message: fmt.Sprintf("duplicate value %q (first seen at %s[%d])", item.Value, where, firstIdx),
				})
			} else {
				seen[item.Value] = i
			}
		}
	})
	return errs
}

// Deprecated reports a violation whenever path is present, carrying a
// migration hint for the user. Combine with Config.NoValidateOnSave to make it
// a non-blocking warning instead of a save blocker.
//
//	editor.Deprecated("dockerFile", "use build.dockerfile instead")
func Deprecated(path, message string) Validator {
	return &deprecatedValidator{path: path, message: message}
}

type deprecatedValidator struct{ path, message string }

func (v *deprecatedValidator) Validate(raw []byte, _ []document.Block) []Violation {
	root, ok := rootMapping(raw)
	if !ok {
		return nil
	}
	var errs []Violation
	forEachLeaf(root, v.path, &errs, func(_ *yaml.Node, where string, e *[]Violation) {
		*e = append(*e, Violation{Path: where, Message: "deprecated — " + v.message})
	})
	return errs
}

// forEachLeaf calls fn with every node reached by path and its full expanded
// path. Sequences are expanded at every level, and — once at least one segment
// has matched — a missing segment falls back to dict-of-structs descent
// (every mapping value is searched), mirroring navigateYAML. The leaf node is
// delivered as-is (scalar, sequence, or mapping); fn never receives nil —
// absent paths simply produce no calls.
func forEachLeaf(root *yaml.Node, path string, errs *[]Violation, fn func(node *yaml.Node, where string, errs *[]Violation)) {
	walkLeaf(root, strings.Split(path, "."), "", false, errs, fn)
}

// walkLeaf implements forEachLeaf. matched tracks whether any segment has been
// consumed yet: the dict-of-structs fallback is disabled at the root so a
// missing top-level key is "absent" rather than a document-wide search.
func walkLeaf(node *yaml.Node, segs []string, path string, matched bool, errs *[]Violation,
	fn func(node *yaml.Node, where string, errs *[]Violation)) {
	if node.Kind == yaml.SequenceNode {
		for i, item := range node.Content {
			walkLeaf(item, segs, fmt.Sprintf("%s[%d]", path, i), matched, errs, fn)
		}
		return
	}
	if node.Kind != yaml.MappingNode {
		return
	}
	key, rest := segs[0], segs[1:]
	if child := childOf(node, key); child != nil {
		childPath := joinPath(path, key)
		if len(rest) == 0 {
			fn(child, childPath, errs)
			return
		}
		walkLeaf(child, rest, childPath, true, errs, fn)
		return
	}
	if !matched {
		return
	}
	// Key not found at this level — treat as a dict-of-structs: search all values.
	for i := 0; i+1 < len(node.Content); i += 2 {
		walkLeaf(node.Content[i+1], segs, joinPath(path, node.Content[i].Value), matched, errs, fn)
	}
}

// scalarChild returns the scalar value of mapping node's direct key, or ""
// when the key is absent or its value is not a scalar.
func scalarChild(node *yaml.Node, key string) string {
	c := childOf(node, key)
	if c == nil || c.Kind != yaml.ScalarNode {
		return ""
	}
	return c.Value
}

// rootMapping unmarshals raw and returns its root node. An empty document
// yields an empty mapping (so unconditional checks like Required still run);
// invalid YAML yields ok=false (the parse error is reported elsewhere).
func rootMapping(raw []byte) (*yaml.Node, bool) {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, false
	}
	if len(doc.Content) == 0 {
		return &yaml.Node{Kind: yaml.MappingNode}, true
	}
	return doc.Content[0], true
}

// childOf returns the value node for the direct key k of mapping node, or nil.
func childOf(node *yaml.Node, k string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == k {
			return node.Content[i+1]
		}
	}
	return nil
}

// presentNonEmpty reports whether node exists and is not an empty/null scalar.
// Mappings and sequences count as present even when empty.
func presentNonEmpty(node *yaml.Node) bool {
	return node != nil && (node.Kind != yaml.ScalarNode || node.Value != "")
}

// joinPath joins a dot-separated prefix with a key, omitting the dot when the
// prefix is empty.
func joinPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
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

// parseOrderedPair tries to parse a and b as comparable values of the same
// kind (plain number, time.Duration, or size string). Mixed kinds are not
// comparable.
func parseOrderedPair(a, b string) (float64, float64, bool) {
	av, aKind, okA := parseComparable(a)
	bv, bKind, okB := parseComparable(b)
	if !okA || !okB || aKind != bKind {
		return 0, 0, false
	}
	return av, bv, true
}

// compKind classifies a comparable scalar so that only like kinds compare.
type compKind int

const (
	compNumber compKind = iota
	compDuration
	compSize
)

// parseComparable parses s as a plain number ("8080", "0.5"), a time.Duration
// ("24h"), or a size string ("10MB", "256MiB"). Plain numbers are tried first
// so unit-less strings ("0") never classify as durations.
func parseComparable(s string) (float64, compKind, bool) {
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f, compNumber, true
	}
	if d, err := time.ParseDuration(s); err == nil {
		return float64(d), compDuration, true
	}
	if n, ok := parseSize(s); ok {
		return float64(n), compSize, true
	}
	return 0, 0, false
}

// sizeUnits maps suffix → byte multiplier, ordered longest-suffix-first to
// avoid "B" matching before "MB" or "GiB". SI suffixes (KB/MB/GB/TB) are
// decimal (powers of 1000); IEC suffixes (KiB/MiB/GiB/TiB) are binary
// (powers of 1024). Matching is case-insensitive.
var sizeUnits = []struct {
	suffix string
	mult   int64
}{
	{"TIB", 1 << 40},
	{"GIB", 1 << 30},
	{"MIB", 1 << 20},
	{"KIB", 1 << 10},
	{"TB", 1_000_000_000_000},
	{"GB", 1_000_000_000},
	{"MB", 1_000_000},
	{"KB", 1_000},
	{"B", 1},
}

// parseSize parses strings like "10MB", "500KB", "1.5GB", "256MiB".
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
