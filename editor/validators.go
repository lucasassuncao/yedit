package editor

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/lucasassuncao/yedit/internal/yamlnode"

	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/document"
	"github.com/lucasassuncao/yedit/schema"
)

// # Validator families
//
// There are two families of validators. Choose based on whether you have a
// MetadataSource configured in editor.Config.Metadata:
//
// FromMetadata family - RequiredFromMetadata, OneOfFromMetadata,
// RangeFromMetadata, PatternFromMetadata, CountFromMetadata,
// UniqueFromMetadata, DeprecatedFromMetadata:
//   - Use when you have a MetadataSource (e.g. metadata.Build).
//   - Constraints are declared once in the metadata tree and reused by both
//     the hint panel and the validators - no duplication.
//   - These validators are inert until editor.Run wires them; they cannot be
//     used standalone outside a session.
//
// Explicit family - Required, ValueOneOf, ValueInRange, ValueMatches,
// CountRange, UniqueValues, Deprecated:
//   - Use for one-off rules that do not need a metadata tree, or for
//     cross-field rules that cannot live in per-field metadata
//     (MutuallyExclusive, RequiredWith, CrossFieldOrdered, etc.).
//   - Work standalone: pass raw YAML to RunAll and they evaluate immediately.
//
// Mixing both families is valid. A typical setup uses FromMetadata for all
// per-field constraints and explicit validators only for cross-field rules.

// NewValidationInput parses raw once and bundles it with blocks for a
// validation run. Root is nil when raw is not valid YAML; an empty document
// yields an empty mapping so unconditional checks still run.
func NewValidationInput(raw []byte, blocks []document.Block) ValidationInput {
	root, _ := yamlnode.RootMapping(raw)
	return ValidationInput{Raw: raw, Root: root, Blocks: blocks}
}

// RunAll executes all validators against raw/blocks and collects violations.
// The document is parsed once and shared across validators.
func RunAll(validators []Validator, raw []byte, blocks []document.Block) []Violation {
	if len(validators) == 0 {
		return nil
	}
	in := NewValidationInput(raw, blocks)
	var errs []Violation
	for _, v := range validators {
		errs = append(errs, v.Validate(in)...)
	}
	return errs
}

// MutuallyExclusive reports a violation when more than one of the listed keys
// is present at the same time.
//
// Two forms are supported:
//
// Top-level keys (no dots) - checks the document's root-level blocks:
//
//	editor.MutuallyExclusive("image", "build", "dockerComposeFile")
//
// Dotted paths - all paths must share the same parent prefix. The validator
// navigates to that parent in the YAML tree, automatically expanding sequences
// (all items are checked) and dict-style mappings (all values are checked).
// Use this for constraints that live at a specific location in the document:
//
//	editor.MutuallyExclusive(
//	    "categories.installers.source.filter.any",
//	    "categories.installers.source.filter.all",
//	)
//
// Dotted paths that do not share the same parent prefix (or have different
// depths) are a configuration error, reported as a violation on every
// validate so the mistake cannot go unnoticed.
//
// For constraints that must hold at every occurrence of a key regardless of
// depth (e.g. recursive schemas), use MutuallyExclusiveNested instead.
func MutuallyExclusive(keys ...string) Validator {
	for _, k := range keys {
		if strings.Contains(k, ".") {
			return newPathKeysValidator("MutuallyExclusive", keys, mutualExclusionViolation)
		}
	}
	return &mutuallyExclusiveValidator{keys: keys}
}

type mutuallyExclusiveValidator struct{ keys []string }

func (v *mutuallyExclusiveValidator) Validate(in ValidationInput) []Violation {
	present := keysPresent(in.Blocks)
	return mutualExclusionViolation(v.keys, func(k string) bool { return present[k] }, "")
}

// misconfiguredValidator reports a fixed configuration error on every run, so
// a rule built from invalid arguments surfaces on the first validate instead
// of silently never firing (same pattern as ValueMatches with a bad regex).
type misconfiguredValidator struct{ message string }

func (v *misconfiguredValidator) Validate(ValidationInput) []Violation {
	return []Violation{{Message: v.message}}
}

// newPathKeysValidator builds the path-aware variant of a key-combination
// rule. All paths must share the same parent path (everything before the last
// dot); the leaf segments become the keys checked inside every mapping reached
// by that parent. funcName labels the misconfiguration message; violation
// receives the leaf keys, a presence probe for the current mapping, and the
// violation path.
func newPathKeysValidator(funcName string, fullPaths []string, violation func(keys []string, has func(string) bool, where string) []Violation) Validator {
	parent, leaves, ok := splitSharedParent(fullPaths)
	if !ok {
		return &misconfiguredValidator{message: fmt.Sprintf(
			"invalid %s(%s): dotted paths must share the same parent prefix",
			funcName, joinQuoted(fullPaths),
		)}
	}
	return &pathKeysValidator{parentSegs: parent, keys: leaves, violation: violation}
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

type pathKeysValidator struct {
	parentSegs []string // path segments to the parent mapping
	keys       []string // leaf keys checked within that mapping
	violation  func(keys []string, has func(string) bool, where string) []Violation
}

func (v *pathKeysValidator) Validate(in ValidationInput) []Violation {
	var errs []Violation
	forEachParentMapping(in.Root, v.parentSegs, func(n *yaml.Node, p string) {
		where := p
		if where == "" {
			where = strings.Join(v.parentSegs, ".")
		}
		errs = append(errs, v.violation(v.keys, func(k string) bool {
			return yamlnode.ChildByKey(n, k) != nil
		}, where)...)
	})
	return errs
}

// RequiredWith reports a violation when key is present but parent is not.
//
// Like MutuallyExclusive it supports two forms: plain keys are checked against
// the document's top-level blocks, and dotted paths - both sharing the same
// parent prefix - are checked inside every mapping reached by that parent,
// with sequences and dict-style mappings expanded automatically:
//
//	editor.RequiredWith("service", "dockerComposeFile")
//	editor.RequiredWith("server.tls-key", "server.tls-cert")
//
// Dotted paths that do not share the same parent prefix (or have different
// depths) are a configuration error, reported as a violation on every
// validate so the mistake cannot go unnoticed.
func RequiredWith(key, parent string) Validator {
	if strings.Contains(key, ".") || strings.Contains(parent, ".") {
		parentSegs, leaves, ok := splitSharedParent([]string{key, parent})
		if !ok {
			return &misconfiguredValidator{message: fmt.Sprintf(
				"invalid RequiredWith(%s): dotted paths must share the same parent prefix",
				joinQuoted([]string{key, parent}),
			)}
		}
		return &pathRequiredWithValidator{parentSegs: parentSegs, key: leaves[0], parentKey: leaves[1]}
	}
	return &requiredWithValidator{key: key, parent: parent}
}

type requiredWithValidator struct{ key, parent string }

func (v *requiredWithValidator) Validate(in ValidationInput) []Violation {
	present := keysPresent(in.Blocks)
	if present[v.key] && !present[v.parent] {
		return []Violation{{Message: fmt.Sprintf(
			"%q requires %q to be set", v.key, v.parent,
		)}}
	}
	return nil
}

type pathRequiredWithValidator struct {
	parentSegs []string // path segments to the parent mapping
	key        string   // leaf key that triggers the requirement
	parentKey  string   // leaf key that must accompany key
}

func (v *pathRequiredWithValidator) Validate(in ValidationInput) []Violation {
	var errs []Violation
	forEachParentMapping(in.Root, v.parentSegs, func(n *yaml.Node, p string) {
		if yamlnode.ChildByKey(n, v.key) != nil && yamlnode.ChildByKey(n, v.parentKey) == nil {
			where := p
			if where == "" {
				where = strings.Join(v.parentSegs, ".")
			}
			errs = append(errs, Violation{
				Path:    where,
				Message: fmt.Sprintf("%q requires %q to be set", v.key, v.parentKey),
			})
		}
	})
	return errs
}

// MutuallyExclusiveNested walks the YAML tree and fires at every mapping whose
// direct parent key is the last segment of scopedPath, checking that at most
// one of keys is present.
//
// Two forms:
//
// Single key - searches the entire document (backward-compatible):
//
//	editor.MutuallyExclusiveNested("filter", "any", "all")
//
// Dotted path - navigates to the scoped root first, then recurses only within
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

func (v *mutuallyExclusiveNestedValidator) Validate(in ValidationInput) []Violation {
	root := in.Root
	if root == nil {
		return nil
	}
	var errs []Violation
	yamlnode.Navigate(root, v.navSegs, "", func(n *yaml.Node, p string) {
		v.walk(n, "", p, &errs)
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

// checkMutualExclusion appends to errs when more than one of keys appears as a
// direct child key of node (which must be a MappingNode). where is the
// dot-separated path reported in the violation.
func checkMutualExclusion(node *yaml.Node, keys []string, where string, errs *[]Violation) {
	*errs = append(*errs, mutualExclusionViolation(keys, func(k string) bool {
		return yamlnode.ChildByKey(node, k) != nil
	}, where)...)
}

// mutualExclusionViolation returns a violation when more than one of keys is
// present according to has. where is the violation path (may be empty).
func mutualExclusionViolation(keys []string, has func(string) bool, where string) []Violation {
	var found []string
	for _, k := range keys {
		if has(k) {
			found = append(found, k)
		}
	}
	if len(found) <= 1 {
		return nil
	}
	return []Violation{{
		Path: where,
		Message: fmt.Sprintf(
			"mutually exclusive - use only one of: %s",
			joinQuoted(found),
		),
	}}
}

// AtLeastOneOf reports a violation when none of the listed keys is present.
//
// Like MutuallyExclusive it supports two forms: plain keys are checked against
// the document's top-level blocks, and dotted paths - all sharing the same
// parent prefix - are checked inside every mapping reached by that parent,
// with sequences and dict-style mappings expanded automatically. The rule only
// fires where the parent mapping exists:
//
//	editor.AtLeastOneOf("image", "build")
//	editor.AtLeastOneOf("auth.token", "auth.password")
//
// Dotted paths that do not share the same parent prefix (or have different
// depths) are a configuration error, reported as a violation on every
// validate so the mistake cannot go unnoticed.
func AtLeastOneOf(keys ...string) Validator {
	for _, k := range keys {
		if strings.Contains(k, ".") {
			return newPathKeysValidator("AtLeastOneOf", keys, atLeastOneViolation)
		}
	}
	return &atLeastOneOfValidator{keys: keys}
}

type atLeastOneOfValidator struct{ keys []string }

func (v *atLeastOneOfValidator) Validate(in ValidationInput) []Violation {
	present := keysPresent(in.Blocks)
	return atLeastOneViolation(v.keys, func(k string) bool { return present[k] }, "")
}

// atLeastOneViolation returns a violation when none of keys is present
// according to has. where is the violation path (may be empty).
func atLeastOneViolation(keys []string, has func(string) bool, where string) []Violation {
	for _, k := range keys {
		if has(k) {
			return nil
		}
	}
	return []Violation{{
		Path:    where,
		Message: fmt.Sprintf("at least one of %s is required", joinQuoted(keys)),
	}}
}

// ExactlyOneOf reports a violation when none or more than one of the listed keys is present.
//
// Like MutuallyExclusive it supports two forms: plain keys are checked against
// the document's top-level blocks, and dotted paths - all sharing the same
// parent prefix - are checked inside every mapping reached by that parent,
// with sequences and dict-style mappings expanded automatically. The rule only
// fires where the parent mapping exists:
//
//	editor.ExactlyOneOf("image", "build", "dockerComposeFile")
//	editor.ExactlyOneOf("source.git", "source.local")
//
// Dotted paths that do not share the same parent prefix (or have different
// depths) are a configuration error, reported as a violation on every
// validate so the mistake cannot go unnoticed.
func ExactlyOneOf(keys ...string) Validator {
	for _, k := range keys {
		if strings.Contains(k, ".") {
			return newPathKeysValidator("ExactlyOneOf", keys, exactlyOneViolation)
		}
	}
	return &exactlyOneOfValidator{keys: keys}
}

type exactlyOneOfValidator struct{ keys []string }

func (v *exactlyOneOfValidator) Validate(in ValidationInput) []Violation {
	present := keysPresent(in.Blocks)
	return exactlyOneViolation(v.keys, func(k string) bool { return present[k] }, "")
}

// exactlyOneViolation returns a violation when none or more than one of keys
// is present according to has. where is the violation path (may be empty).
func exactlyOneViolation(keys []string, has func(string) bool, where string) []Violation {
	var found []string
	for _, k := range keys {
		if has(k) {
			found = append(found, k)
		}
	}
	switch len(found) {
	case 1:
		return nil
	case 0:
		return []Violation{{
			Path:    where,
			Message: fmt.Sprintf("exactly one of %s is required", joinQuoted(keys)),
		}}
	default:
		return []Violation{{
			Path: where,
			Message: fmt.Sprintf(
				"exactly one of %s must be set - found: %s",
				joinQuoted(keys), joinQuoted(found),
			),
		}}
	}
}

// RequiredIf reports a violation when key is absent but condPath equals condValue.
//
// When key and condPath share the same parent prefix, the rule is evaluated
// inside every mapping reached by that parent - sequences and dict-style
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

func (v *requiredIfValidator) Validate(in ValidationInput) []Violation {
	root := in.Root
	if root == nil {
		return nil
	}
	var errs []Violation
	if parent, leaves, shared := splitSharedParent([]string{v.key, v.condPath}); shared {
		keyLeaf, condLeaf := leaves[0], leaves[1]
		forEachParentMapping(root, parent, func(n *yaml.Node, p string) {
			if yamlnode.ScalarChild(n, condLeaf) != v.condValue {
				return
			}
			// A non-scalar value (mapping/sequence) counts as present; only a
			// missing key or an empty scalar is a violation.
			if !yamlnode.PresentNonEmpty(yamlnode.ChildByKey(n, keyLeaf)) {
				errs = append(errs, Violation{
					Path:    yamlnode.JoinPath(p, keyLeaf),
					Message: fmt.Sprintf("required when %q is %q", v.condPath, v.condValue),
				})
			}
		})
		return errs
	}
	// Unrelated parents: both paths are resolved from the root.
	if yamlnode.ScalarAt(root, strings.Split(v.condPath, ".")) != v.condValue {
		return nil
	}
	if !yamlnode.PresentNonEmpty(yamlnode.NodeAtPath(root, strings.Split(v.key, "."))) {
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

func (v *valueOneOfValidator) Validate(in ValidationInput) []Violation {
	var errs []Violation
	forEachScalar(in.Root, v.path, &errs, func(value, where string) {
		for _, a := range v.allowed {
			if value == a {
				return
			}
		}
		errs = append(errs, Violation{
			Path:    where,
			Message: fmt.Sprintf("value %q is not allowed - use one of: %s", value, joinQuoted(v.allowed)),
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
// every mapping reached by that parent - sequences and dict-style mappings are
// expanded automatically, so each entry's own min/max pair is checked. Paths
// with unrelated parents are both resolved from the document root.
func CrossFieldOrdered(smallerPath, largerPath string) Validator {
	return &crossFieldOrderedValidator{smallerPath: smallerPath, largerPath: largerPath}
}

type crossFieldOrderedValidator struct{ smallerPath, largerPath string }

func (v *crossFieldOrderedValidator) Validate(in ValidationInput) []Violation {
	root := in.Root
	if root == nil {
		return nil
	}
	var errs []Violation
	if parent, leaves, shared := splitSharedParent([]string{v.smallerPath, v.largerPath}); shared {
		smallLeaf, largeLeaf := leaves[0], leaves[1]
		forEachParentMapping(root, parent, func(n *yaml.Node, p string) {
			checkOrdered(yamlnode.ScalarChild(n, smallLeaf), yamlnode.ScalarChild(n, largeLeaf), smallLeaf, largeLeaf, p, &errs)
		})
		return errs
	}
	// Unrelated parents: both paths are resolved from the root.
	aStr := yamlnode.ScalarAt(root, strings.Split(v.smallerPath, "."))
	bStr := yamlnode.ScalarAt(root, strings.Split(v.largerPath, "."))
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
// share the same value for field. Sequences and dict-style mappings along
// seqPath are expanded automatically, and uniqueness is checked per reached
// list - entries in different lists may repeat. field may be a dotted path
// inside each item.
//
//	editor.NoDuplicates("servers", "name")
//	editor.NoDuplicates("categories.installers", "meta.name")
func NoDuplicates(seqPath, field string) Validator {
	return &noDuplicatesValidator{seqPath: seqPath, field: field}
}

type noDuplicatesValidator struct{ seqPath, field string }

func (v *noDuplicatesValidator) Validate(in ValidationInput) []Violation {
	root := in.Root
	if root == nil {
		return nil
	}
	fieldSegs := strings.Split(v.field, ".")
	var errs []Violation
	yamlnode.ForEachLeaf(root, v.seqPath, func(seqNode *yaml.Node, where string) {
		if seqNode.Kind != yaml.SequenceNode {
			return
		}
		values := make([]string, len(seqNode.Content))
		for i, item := range seqNode.Content {
			values[i] = yamlnode.ScalarAt(item, fieldSegs)
		}
		reportDuplicates(values, where, "."+v.field, &errs)
	})
	return errs
}

// Required reports a violation when any of the given paths is absent or holds
// an empty/null scalar. A non-scalar value (mapping or sequence) counts as
// present.
//
// A path with no dots is required unconditionally at the document root. A
// dotted path is conditional: the validator navigates to the leaf's parent -
// expanding sequences and dict-style mappings like MutuallyExclusive - and
// only requires the leaf where that parent exists, so a required field inside
// an optional block is not reported while the block is absent.
//
//	editor.Required("version")          // top-level, unconditional
//	editor.Required("categories.name")  // every category entry needs "name"
//
// To enforce the MetadataSource's Required markers without listing paths by hand,
// use RequiredFromMetadata.
func Required(paths ...string) Validator {
	return &requiredValidator{paths: paths}
}

type requiredValidator struct{ paths []string }

func (v *requiredValidator) Validate(in ValidationInput) []Violation {
	var errs []Violation
	for _, p := range v.paths {
		// Unlike forEachScalar, Required must see absent leaves, so it navigates
		// to the leaf's parent and checks the leaf there. The dict-of-structs
		// fallback therefore applies to intermediate segments only.
		segs := strings.Split(p, ".")
		parent, leaf := segs[:len(segs)-1], segs[len(segs)-1]
		forEachParentMapping(in.Root, parent, func(n *yaml.Node, path string) {
			if !yamlnode.PresentNonEmpty(yamlnode.ChildByKey(n, leaf)) {
				errs = append(errs, Violation{Path: yamlnode.JoinPath(path, leaf), Message: "required"})
			}
		})
	}
	return errs
}

// RequiredFromMetadata enforces the MetadataSource's required markers
// (FieldMeta.Required) at validate/save time, for applications that declare
// required-ness in their hints. Without it the marker is display-only: the
// "Required: yes" hint line does not block saving.
//
// The walk is guided by the discovered schema: for every schema path the
// validator asks the MetadataSource for that field's FieldMeta - using the same
// query convention as the hint panel, FieldMeta(block, "") for a top-level
// block and FieldMeta(block, "source.path") for nested fields - and, when
// Required is set, checks presence. A required field is only enforced where
// its parent exists; top-level required blocks are always enforced. Sequence
// and dictionary entries are checked individually.
//
// The editor wires the discovered schema and the configured MetadataSource into
// this validator when the session starts; outside editor.Run, or when no
// MetadataSource is configured, it reports nothing.
func RequiredFromMetadata() Validator { return &metadataRuleValidator{check: checkHintRequired} }

// metadataRuleValidator is the shared engine of the FromMetadata validator family.
// It walks the YAML guided by the discovered schema, queries the MetadataSource
// for every field - FieldMeta(block, "") for top-level blocks,
// FieldMeta(block, "a.b") for nested fields, the hint panel's convention -
// and delegates to check. Sequence and dictionary entries are visited
// individually. The editor wires defs and hints at session start; outside
// editor.Run, or without a MetadataSource, the validator is inert.
type metadataRuleValidator struct {
	defs  []schema.FieldDef
	hints MetadataSource
	// check receives the field's hint metadata and its YAML node (nil when the
	// field is absent), and appends violations. Zero-valued metadata must
	// report nothing.
	check func(meta FieldMeta, child *yaml.Node, path string, errs *[]Violation)
}

func (v *metadataRuleValidator) Validate(in ValidationInput) []Violation {
	if v.hints == nil || len(v.defs) == 0 {
		return nil
	}
	root := in.Root
	if root == nil {
		return nil
	}
	var errs []Violation
	v.walk(root, v.defs, "", "", "", &errs)
	return errs
}

// walk visits node guided by defs. blockKey is empty at the document root,
// where each def is itself a top-level block queried as FieldMeta(name, "");
// below that, hintPath is the dot-joined schema path from the block root (no
// sequence indexes), matching the hint panel's query convention. yamlPath
// carries the expanded path used in violations.
func (v *metadataRuleValidator) walk(node *yaml.Node, defs []schema.FieldDef, blockKey, hintPath, yamlPath string, errs *[]Violation) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	for _, def := range defs {
		childBlock, childHint := blockKey, yamlnode.JoinPath(hintPath, def.YAMLName)
		if blockKey == "" {
			childBlock, childHint = def.YAMLName, ""
		}
		child := yamlnode.ChildByKey(node, def.YAMLName)
		childYAML := yamlnode.JoinPath(yamlPath, def.YAMLName)
		v.check(v.hints.FieldMeta(childBlock, childHint), child, childYAML, errs)
		// KindVariant children describe union alternatives, not required structure.
		if child == nil || len(def.Children) == 0 || def.Kind == schema.KindVariant {
			continue
		}
		switch def.Kind {
		case schema.KindObject:
			v.walk(child, def.Children, childBlock, childHint, childYAML, errs)
		case schema.KindList:
			if child.Kind == yaml.SequenceNode {
				for i, item := range child.Content {
					v.walk(item, def.Children, childBlock, childHint, fmt.Sprintf("%s[%d]", childYAML, i), errs)
				}
			}
		case schema.KindDictionary:
			if child.Kind == yaml.MappingNode {
				for i := 0; i+1 < len(child.Content); i += 2 {
					v.walk(child.Content[i+1], def.Children, childBlock, childHint, yamlnode.JoinPath(childYAML, child.Content[i].Value), errs)
				}
			}
		}
	}
}

// checkHintRequired enforces FieldMeta.Required: absent fields or empty
// scalars violate; a non-scalar value counts as present.
func checkHintRequired(meta FieldMeta, child *yaml.Node, path string, errs *[]Violation) {
	if meta.Required && !yamlnode.PresentNonEmpty(child) {
		*errs = append(*errs, Violation{Path: path, Message: "required"})
	}
}

// OneOfFromMetadata enforces FieldMeta.OneOf from the MetadataSource: a present,
// non-empty scalar must be one of the declared values (ValueOneOf semantics).
// Fields without OneOf declare nothing. Wired by the editor like
// RequiredFromMetadata.
func OneOfFromMetadata() Validator { return &metadataRuleValidator{check: checkHintOneOf} }

func checkHintOneOf(meta FieldMeta, child *yaml.Node, path string, errs *[]Violation) {
	if len(meta.OneOf) == 0 {
		return
	}
	val, ok := hintScalarValue(child, path, errs)
	if !ok {
		return
	}
	for _, a := range meta.OneOf {
		if val == a {
			return
		}
	}
	*errs = append(*errs, Violation{
		Path:    path,
		Message: fmt.Sprintf("value %q is not allowed - use one of: %s", val, joinQuoted(meta.OneOf)),
	})
}

// RangeFromMetadata enforces FieldMeta.Min/Max from the MetadataSource (ValueInRange
// semantics): bounds and value may be plain numbers, durations, or sizes, and
// must be of the same kind. One-sided bounds are allowed - only Min means "at
// least Min", only Max means "at most Max". Malformed or mixed-kind bounds in
// a hint are reported as a misconfiguration violation on every run.
func RangeFromMetadata() Validator { return &metadataRuleValidator{check: checkHintRange} }

func checkHintRange(meta FieldMeta, child *yaml.Node, path string, errs *[]Violation) {
	if meta.Min == "" && meta.Max == "" {
		return
	}
	loStr, hiStr := meta.Min, meta.Max
	lo, hi := math.Inf(-1), math.Inf(1)
	loKind, hiKind := compKind(-1), compKind(-1)
	var ok bool
	if loStr != "" {
		if lo, loKind, ok = parseComparable(loStr); !ok {
			reportInvalidHintRange(loStr, hiStr, path, errs)
			return
		}
	}
	if hiStr != "" {
		if hi, hiKind, ok = parseComparable(hiStr); !ok {
			reportInvalidHintRange(loStr, hiStr, path, errs)
			return
		}
	}
	if loKind >= 0 && hiKind >= 0 && loKind != hiKind {
		reportInvalidHintRange(loStr, hiStr, path, errs)
		return
	}
	wantKind := loKind
	if wantKind < 0 {
		wantKind = hiKind
	}
	if loStr == "" {
		loStr = "-∞"
	}
	if hiStr == "" {
		hiStr = "∞"
	}
	val, okVal := hintScalarValue(child, path, errs)
	if !okVal {
		return
	}
	v, kind, okParse := parseComparable(val)
	if !okParse || kind != wantKind {
		*errs = append(*errs, Violation{
			Path:    path,
			Message: fmt.Sprintf("value %q is not comparable with range [%s, %s]", val, loStr, hiStr),
		})
		return
	}
	if v < lo || v > hi {
		*errs = append(*errs, Violation{
			Path:    path,
			Message: fmt.Sprintf("value %q out of range [%s, %s]", val, loStr, hiStr),
		})
	}
}

func reportInvalidHintRange(lo, hi, path string, errs *[]Violation) {
	*errs = append(*errs, Violation{
		Path:    path,
		Message: fmt.Sprintf("invalid range [%s, %s] in hint - bounds must both be durations, sizes, or numbers", lo, hi),
	})
}

// PatternFromMetadata enforces FieldMeta.Pattern from the MetadataSource
// (ValueMatches semantics). Compiled patterns are cached per validator
// instance; an invalid pattern is reported as a misconfiguration violation
// wherever the hint declares it.
func PatternFromMetadata() Validator {
	cache := map[string]*regexp.Regexp{} // pattern → compiled; nil marks invalid
	return &metadataRuleValidator{check: func(meta FieldMeta, child *yaml.Node, path string, errs *[]Violation) {
		checkHintPattern(cache, meta, child, path, errs)
	}}
}

func checkHintPattern(cache map[string]*regexp.Regexp, meta FieldMeta, child *yaml.Node, path string, errs *[]Violation) {
	if meta.Pattern == "" {
		return
	}
	re, seen := cache[meta.Pattern]
	if !seen {
		re, _ = regexp.Compile(meta.Pattern)
		cache[meta.Pattern] = re
	}
	if re == nil {
		*errs = append(*errs, Violation{Path: path, Message: fmt.Sprintf("invalid pattern %q in hint", meta.Pattern)})
		return
	}
	val, ok := hintScalarValue(child, path, errs)
	if !ok {
		return
	}
	if !re.MatchString(val) {
		*errs = append(*errs, Violation{
			Path:    path,
			Message: fmt.Sprintf("value %q does not match pattern %q", val, meta.Pattern),
		})
	}
}

// CountFromMetadata enforces FieldMeta.MinCount/MaxCount from the MetadataSource
// (CountRange semantics): sequences count items, mappings count keys. Both
// zero declares nothing; MinCount > 0 with MaxCount == 0 means "at least
// MinCount, no upper bound". Absent fields report nothing - combine with
// Required when the collection is mandatory.
func CountFromMetadata() Validator { return &metadataRuleValidator{check: checkHintCount} }

func checkHintCount(meta FieldMeta, child *yaml.Node, path string, errs *[]Violation) {
	if (meta.MinCount == 0 && meta.MaxCount == 0) || child == nil {
		return
	}
	var count int
	switch child.Kind {
	case yaml.SequenceNode:
		count = len(child.Content)
	case yaml.MappingNode:
		count = len(child.Content) / 2
	default:
		if child.Value == "" { // null scalar: an empty collection
			break
		}
		*errs = append(*errs, Violation{Path: path, Message: "expected a list or mapping"})
		return
	}
	if count < meta.MinCount || (meta.MaxCount > 0 && count > meta.MaxCount) {
		want := fmt.Sprintf("between %d and %d", meta.MinCount, meta.MaxCount)
		if meta.MaxCount == 0 {
			want = fmt.Sprintf("at least %d", meta.MinCount)
		}
		*errs = append(*errs, Violation{
			Path:    path,
			Message: fmt.Sprintf("has %d entries - expected %s", count, want),
		})
	}
}

// UniqueFromMetadata enforces FieldMeta.Unique from the MetadataSource (UniqueValues
// semantics): scalar items in the sequence must not repeat. Non-sequence
// fields and non-scalar items are skipped.
func UniqueFromMetadata() Validator { return &metadataRuleValidator{check: checkHintUnique} }

func checkHintUnique(meta FieldMeta, child *yaml.Node, path string, errs *[]Violation) {
	if !meta.Unique || child == nil || child.Kind != yaml.SequenceNode {
		return
	}
	values := make([]string, len(child.Content))
	for i, item := range child.Content {
		if item.Kind == yaml.ScalarNode {
			values[i] = item.Value
		}
	}
	reportDuplicates(values, path, "", errs)
}

// DeprecatedFromMetadata enforces FieldMeta.Deprecated from the MetadataSource
// (Deprecated semantics): every present occurrence of the field is reported,
// carrying the hint's migration message. Combine with Config.NoValidateOnSave
// to make it a non-blocking warning.
func DeprecatedFromMetadata() Validator { return &metadataRuleValidator{check: checkHintDeprecated} }

func checkHintDeprecated(meta FieldMeta, child *yaml.Node, path string, errs *[]Violation) {
	if meta.Deprecated == "" || child == nil {
		return
	}
	*errs = append(*errs, Violation{Path: path, Message: "deprecated - " + meta.Deprecated})
}

// hintScalarValue applies the shared value-rule contract to a hint-checked
// field: nil or empty children report nothing (combine with Required when the
// field is mandatory); a non-scalar child is flagged.
func hintScalarValue(child *yaml.Node, path string, errs *[]Violation) (string, bool) {
	if child == nil {
		return "", false
	}
	if child.Kind != yaml.ScalarNode {
		*errs = append(*errs, Violation{Path: path, Message: "expected a scalar value"})
		return "", false
	}
	if child.Value == "" {
		return "", false
	}
	return child.Value, true
}

// ValueInRange reports a violation when the scalar at path is present but
// outside the inclusive [min, max] range. Bounds and value may be plain
// numbers ("1", "0.5"), time.Duration strings ("24h"), or size strings
// ("10MB", "256MiB" - KB/MB/GB/TB decimal, KiB/MiB/GiB/TiB binary); all three
// must be of the same kind. An absent or empty value reports nothing -
// combine with Required when the field is mandatory.
//
//	editor.ValueInRange("server.port", "1", "65535")
//	editor.ValueInRange("filter.max-age", "1h", "8760h")
func ValueInRange(path, minVal, maxVal string) Validator {
	return &valueInRangeValidator{path: path, min: minVal, max: maxVal}
}

type valueInRangeValidator struct{ path, min, max string }

func (v *valueInRangeValidator) Validate(in ValidationInput) []Violation {
	root := in.Root
	if root == nil {
		return nil
	}
	lo, loKind, okLo := parseComparable(v.min)
	hi, hiKind, okHi := parseComparable(v.max)
	if !okLo || !okHi || loKind != hiKind {
		return []Violation{{
			Path:    v.path,
			Message: fmt.Sprintf("invalid range [%s, %s] - bounds must both be durations, sizes, or numbers", v.min, v.max),
		}}
	}
	var errs []Violation
	forEachScalar(root, v.path, &errs, func(value, where string) {
		val, kind, okVal := parseComparable(value)
		if !okVal || kind != loKind {
			errs = append(errs, Violation{
				Path:    where,
				Message: fmt.Sprintf("value %q is not comparable with range [%s, %s]", value, v.min, v.max),
			})
			return
		}
		if val < lo || val > hi {
			errs = append(errs, Violation{
				Path:    where,
				Message: fmt.Sprintf("value %q out of range [%s, %s]", value, v.min, v.max),
			})
		}
	})
	return errs
}

// ValueMatches reports a violation when the scalar at path is present but does
// not match the regular expression pattern. An absent or empty value reports
// nothing - combine with Required when the field is mandatory. An invalid
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

func (v *valueMatchesValidator) Validate(in ValidationInput) []Violation {
	if v.err != nil {
		return []Violation{{Path: v.path, Message: fmt.Sprintf("invalid pattern %q: %v", v.pattern, v.err)}}
	}
	var errs []Violation
	forEachScalar(in.Root, v.path, &errs, func(value, where string) {
		if !v.re.MatchString(value) {
			errs = append(errs, Violation{
				Path:    where,
				Message: fmt.Sprintf("value %q does not match pattern %q", value, v.pattern),
			})
		}
	})
	return errs
}

// ValueHasPrefix reports a violation when the scalar at path is present but
// does not start with prefix - a simpler alternative to ValueMatches when the
// rule is a fixed prefix and no regex is needed. An absent or empty value
// reports nothing - combine with Required when the field is mandatory.
// Sequences and dict-style mappings along the path are expanded automatically.
//
//	editor.ValueHasPrefix("image", "registry.example.com/")
func ValueHasPrefix(path, prefix string) Validator {
	return &valueAffixValidator{path: path, affix: prefix, prefix: true}
}

// ValueHasSuffix reports a violation when the scalar at path is present but
// does not end with suffix. Same semantics as ValueHasPrefix.
//
//	editor.ValueHasSuffix("output", ".yaml")
func ValueHasSuffix(path, suffix string) Validator {
	return &valueAffixValidator{path: path, affix: suffix, prefix: false}
}

type valueAffixValidator struct {
	path   string
	affix  string
	prefix bool // true checks strings.HasPrefix, false checks strings.HasSuffix
}

func (v *valueAffixValidator) Validate(in ValidationInput) []Violation {
	var errs []Violation
	forEachScalar(in.Root, v.path, &errs, func(value, where string) {
		if v.prefix {
			if !strings.HasPrefix(value, v.affix) {
				errs = append(errs, Violation{
					Path:    where,
					Message: fmt.Sprintf("value %q does not start with %q", value, v.affix),
				})
			}
			return
		}
		if !strings.HasSuffix(value, v.affix) {
			errs = append(errs, Violation{
				Path:    where,
				Message: fmt.Sprintf("value %q does not end with %q", value, v.affix),
			})
		}
	})
	return errs
}

// AllOrNone reports a violation when only some of the listed keys are present:
// they must appear together or not at all (e.g. a TLS cert/key pair).
//
// Like MutuallyExclusive it supports two forms: plain keys are checked against
// the document's top-level blocks, and dotted paths - all sharing the same
// parent prefix - are checked inside every mapping reached by that parent,
// with sequences and dict-style mappings expanded automatically:
//
//	editor.AllOrNone("tls-cert", "tls-key")
//	editor.AllOrNone("server.tls-cert", "server.tls-key")
//
// Dotted paths that do not share the same parent prefix (or have different
// depths) are a configuration error, reported as a violation on every
// validate so the mistake cannot go unnoticed.
func AllOrNone(keys ...string) Validator {
	for _, k := range keys {
		if strings.Contains(k, ".") {
			return newPathKeysValidator("AllOrNone", keys, allOrNoneViolation)
		}
	}
	return &allOrNoneValidator{keys: keys}
}

type allOrNoneValidator struct{ keys []string }

func (v *allOrNoneValidator) Validate(in ValidationInput) []Violation {
	present := keysPresent(in.Blocks)
	return allOrNoneViolation(v.keys, func(k string) bool { return present[k] }, "")
}

// allOrNoneViolation returns a violation listing the missing keys when only
// some of keys are present according to has. where is the violation path (may
// be empty).
func allOrNoneViolation(keys []string, has func(string) bool, where string) []Violation {
	var found, missing []string
	for _, k := range keys {
		if has(k) {
			found = append(found, k)
		} else {
			missing = append(missing, k)
		}
	}
	if len(found) == 0 || len(missing) == 0 {
		return nil
	}
	return []Violation{{
		Path: where,
		Message: fmt.Sprintf(
			"all or none of %s must be set - missing: %s",
			joinQuoted(keys), joinQuoted(missing),
		),
	}}
}

// CountRange reports a violation when the collection at path has fewer than
// minCount or more than maxCount entries. maxCount < 0 means no upper bound.
// Sequences count items; mappings count keys. An absent path reports nothing -
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

func (v *countRangeValidator) Validate(in ValidationInput) []Violation {
	root := in.Root
	if root == nil {
		return nil
	}
	var errs []Violation
	yamlnode.ForEachLeaf(root, v.path, func(node *yaml.Node, where string) {
		var count int
		switch node.Kind {
		case yaml.SequenceNode:
			count = len(node.Content)
		case yaml.MappingNode:
			count = len(node.Content) / 2
		default:
			errs = append(errs, Violation{Path: where, Message: "expected a list or mapping"})
			return
		}
		if count < v.min || (v.max >= 0 && count > v.max) {
			want := fmt.Sprintf("between %d and %d", v.min, v.max)
			if v.max < 0 {
				want = fmt.Sprintf("at least %d", v.min)
			}
			errs = append(errs, Violation{
				Path:    where,
				Message: fmt.Sprintf("has %d entries - expected %s", count, want),
			})
		}
	})
	return errs
}

// UniqueValues reports a violation when two or more scalar items in the
// sequence at seqPath share the same value. Non-scalar items are skipped - use
// NoDuplicates to deduplicate struct entries by one of their fields.
//
//	editor.UniqueValues("tags")
func UniqueValues(seqPath string) Validator {
	return &uniqueValuesValidator{seqPath: seqPath}
}

type uniqueValuesValidator struct{ seqPath string }

func (v *uniqueValuesValidator) Validate(in ValidationInput) []Violation {
	root := in.Root
	if root == nil {
		return nil
	}
	var errs []Violation
	yamlnode.ForEachLeaf(root, v.seqPath, func(seqNode *yaml.Node, where string) {
		if seqNode.Kind != yaml.SequenceNode {
			return
		}
		values := make([]string, len(seqNode.Content))
		for i, item := range seqNode.Content {
			if item.Kind == yaml.ScalarNode {
				values[i] = item.Value
			}
		}
		reportDuplicates(values, where, "", &errs)
	})
	return errs
}

// reportDuplicates appends a violation for every value that repeats an earlier
// one. Empty values are skipped. The violation path is "<where>[<i>]<suffix>".
func reportDuplicates(values []string, where, suffix string, errs *[]Violation) {
	seen := make(map[string]int)
	for i, val := range values {
		if val == "" {
			continue
		}
		if firstIdx, dup := seen[val]; dup {
			*errs = append(*errs, Violation{
				Path:    fmt.Sprintf("%s[%d]%s", where, i, suffix),
				Message: fmt.Sprintf("duplicate value %q (first seen at %s[%d])", val, where, firstIdx),
			})
		} else {
			seen[val] = i
		}
	}
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

func (v *deprecatedValidator) Validate(in ValidationInput) []Violation {
	root := in.Root
	if root == nil {
		return nil
	}
	var errs []Violation
	yamlnode.ForEachLeaf(root, v.path, func(_ *yaml.Node, where string) {
		errs = append(errs, Violation{Path: where, Message: "deprecated - " + v.message})
	})
	return errs
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

// forEachScalar visits every scalar reached by the dotted path - sequences and
// dict-style mappings along the path are expanded automatically - and calls fn
// with the value and its expanded path. It encodes the shared contract of the
// value validators: a non-scalar leaf is flagged as a violation, and absent or
// empty values report nothing (combine with Required when the field is
// mandatory).
func forEachScalar(root *yaml.Node, path string, errs *[]Violation, fn func(value, where string)) {
	if root == nil {
		return
	}
	yamlnode.ForEachLeaf(root, path, func(node *yaml.Node, where string) {
		if node.Kind != yaml.ScalarNode {
			*errs = append(*errs, Violation{Path: where, Message: "expected a scalar value"})
			return
		}
		if node.Value == "" {
			return
		}
		fn(node.Value, where)
	})
}

// forEachParentMapping navigates root to every mapping reached by segs -
// sequences and dict-style mappings expanded automatically - and calls fn with
// the mapping and its dot/index path (empty when the parent is the document
// root). Non-mapping arrivals and a nil root report nothing.
func forEachParentMapping(root *yaml.Node, segs []string, fn func(n *yaml.Node, path string)) {
	if root == nil {
		return
	}
	yamlnode.Navigate(root, segs, "", func(n *yaml.Node, p string) {
		if n.Kind != yaml.MappingNode {
			return
		}
		fn(n, p)
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
