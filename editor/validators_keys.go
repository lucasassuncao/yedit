package editor

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/yamlnode"
)

// This file holds the key-combination and presence rules of the explicit
// validator family: which keys may, must, or must not appear together - at
// the top level, at a fixed path, or recursively (the *Nested variants) - and
// the conditional/cross-field variants (RequiredIf, ForbiddenIf,
// CrossFieldOrdered). Shared traversal and violation helpers live in
// validators.go.

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
	return &topLevelKeysValidator{keys: keys, violation: mutualExclusionViolation}
}

// topLevelKeysValidator is the shared implementation for key-combination rules
// that operate on top-level document blocks (MutuallyExclusive, AtLeastOneOf,
// ExactlyOneOf, AllOrNone). violation encodes the specific rule semantics.
type topLevelKeysValidator struct {
	keys      []string
	violation func(keys []string, has func(string) bool, where string) []Violation
}

func (v *topLevelKeysValidator) Validate(in ValidationInput) []Violation {
	present := keysPresent(in.Blocks)
	return v.violation(v.keys, func(k string) bool { return present[k] }, "")
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
	var errs []Violation
	walkScopedMappings(in.Root, v.navSegs, v.parentKey, func(n *yaml.Node, where string) {
		checkMutualExclusion(n, v.keys, where, &errs)
	})
	return errs
}

// MutuallyExclusiveGroupsNested walks the YAML tree recursively — same
// traversal as MutuallyExclusiveNested — and fires at every mapping whose
// direct parent key is the last segment of scopedPath, reporting a violation
// for every pair of groups that both have at least one key present simultaneously.
//
// Use this when N sets of fields are mutually exclusive as groups: any mapping
// that contains at least one key from two different groups is a violation.
//
//	editor.MutuallyExclusiveGroupsNested(
//	    "categories.source.filter",
//	    []string{"any", "all"},
//	    []string{"match", "age", "size", "not"},
//	)
func MutuallyExclusiveGroupsNested(scopedPath string, groups ...[]string) Validator {
	segs := strings.Split(scopedPath, ".")
	return &mutuallyExclusiveGroupsNestedValidator{
		navSegs:   segs[:len(segs)-1],
		parentKey: segs[len(segs)-1],
		groups:    groups,
	}
}

type mutuallyExclusiveGroupsNestedValidator struct {
	navSegs   []string
	parentKey string
	groups    [][]string
}

func (v *mutuallyExclusiveGroupsNestedValidator) Validate(in ValidationInput) []Violation {
	var errs []Violation
	walkScopedMappings(in.Root, v.navSegs, v.parentKey, func(n *yaml.Node, where string) {
		v.checkGroups(n, where, &errs)
	})
	return errs
}

func (v *mutuallyExclusiveGroupsNestedValidator) checkGroups(node *yaml.Node, where string, errs *[]Violation) {
	found := make([][]string, len(v.groups))
	for i, group := range v.groups {
		for _, k := range group {
			if yamlnode.ChildByKey(node, k) != nil {
				found[i] = append(found[i], k)
			}
		}
	}
	for i := 0; i < len(found); i++ {
		for j := i + 1; j < len(found); j++ {
			if len(found[i]) > 0 && len(found[j]) > 0 {
				*errs = append(*errs, Violation{
					Path: where,
					Message: fmt.Sprintf(
						"cannot mix fields (%s) with fields (%s)",
						strings.Join(found[i], ", "), strings.Join(found[j], ", "),
					),
				})
			}
		}
	}
}

// CrossFieldOrderedNested walks the YAML tree recursively — same traversal as
// MutuallyExclusiveNested — and fires at every mapping whose direct parent key
// is the last segment of scopedPath, reporting a violation when both
// smallerLeaf and largerLeaf are present but their values are not strictly
// ordered (smaller < larger). Values are compared as plain numbers,
// time.Duration strings, or size strings (same semantics as CrossFieldOrdered).
//
// Use this to enforce min/max ordering at any nesting depth without listing
// every possible path explicitly:
//
//	// catches age.min >= age.max at filter, filter.any[i], filter.any[i].all[j], …
//	editor.CrossFieldOrderedNested("categories.source.filter.age", "min", "max")
func CrossFieldOrderedNested(scopedPath, smallerLeaf, largerLeaf string) Validator {
	segs := strings.Split(scopedPath, ".")
	return &crossFieldOrderedNestedValidator{
		navSegs:     segs[:len(segs)-1],
		parentKey:   segs[len(segs)-1],
		smallerLeaf: smallerLeaf,
		largerLeaf:  largerLeaf,
	}
}

type crossFieldOrderedNestedValidator struct {
	navSegs     []string
	parentKey   string
	smallerLeaf string
	largerLeaf  string
}

func (v *crossFieldOrderedNestedValidator) Validate(in ValidationInput) []Violation {
	var errs []Violation
	walkScopedMappings(in.Root, v.navSegs, v.parentKey, func(n *yaml.Node, where string) {
		checkOrdered(
			yamlnode.ScalarChild(n, v.smallerLeaf),
			yamlnode.ScalarChild(n, v.largerLeaf),
			v.smallerLeaf, v.largerLeaf, where, &errs,
		)
	})
	return errs
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
		Path:    where,
		Group:   GroupMutuallyExclusive,
		Message: fmt.Sprintf("use only one of: %s", joinQuoted(found)),
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
	return &topLevelKeysValidator{keys: keys, violation: atLeastOneViolation}
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
	return &topLevelKeysValidator{keys: keys, violation: exactlyOneViolation}
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
	return &topLevelKeysValidator{keys: keys, violation: allOrNoneViolation}
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

// ForbiddenIf reports a violation when key is present and condPath equals
// condValue - the inverse of RequiredIf.
//
// When key and condPath share the same parent prefix, the rule is evaluated
// inside every mapping reached by that parent - sequences and dict-style
// mappings are expanded automatically, so each entry is checked against its
// own condition value:
//
//	// read-only mode must not carry a write-token field
//	editor.ForbiddenIf("server.write-token", "server.mode", "readonly")
//
// Paths with unrelated parents are both resolved from the document root.
func ForbiddenIf(key, condPath, condValue string) Validator {
	return &forbiddenIfValidator{key: key, condPath: condPath, condValue: condValue}
}

type forbiddenIfValidator struct{ key, condPath, condValue string }

func (v *forbiddenIfValidator) Validate(in ValidationInput) []Violation {
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
			if yamlnode.ChildByKey(n, keyLeaf) != nil {
				errs = append(errs, Violation{
					Path:    yamlnode.JoinPath(p, keyLeaf),
					Message: fmt.Sprintf("not allowed when %q is %q", v.condPath, v.condValue),
				})
			}
		})
		return errs
	}
	// Unrelated parents: both paths are resolved from the root.
	if yamlnode.ScalarAt(root, strings.Split(v.condPath, ".")) != v.condValue {
		return nil
	}
	if yamlnode.NodeAtPath(root, strings.Split(v.key, ".")) != nil {
		errs = append(errs, Violation{
			Path:    v.key,
			Message: fmt.Sprintf("not allowed when %q is %q", v.condPath, v.condValue),
		})
	}
	return errs
}

// AtLeastOneOfNested walks the YAML tree and fires at every mapping whose
// direct parent key is the last segment of scopedPath, checking that at least
// one of keys is present - the nested counterpart of AtLeastOneOf.
//
//	editor.AtLeastOneOfNested("categories.source.auth", "token", "password")
func AtLeastOneOfNested(scopedPath string, keys ...string) Validator {
	segs := strings.Split(scopedPath, ".")
	return &atLeastOneOfNestedValidator{
		navSegs:   segs[:len(segs)-1],
		parentKey: segs[len(segs)-1],
		keys:      keys,
	}
}

type atLeastOneOfNestedValidator struct {
	navSegs   []string
	parentKey string
	keys      []string
}

func (v *atLeastOneOfNestedValidator) Validate(in ValidationInput) []Violation {
	var errs []Violation
	walkScopedMappings(in.Root, v.navSegs, v.parentKey, func(n *yaml.Node, where string) {
		errs = append(errs, atLeastOneViolation(v.keys, func(k string) bool {
			return yamlnode.ChildByKey(n, k) != nil
		}, where)...)
	})
	return errs
}

// ExactlyOneOfNested walks the YAML tree and fires at every mapping whose
// direct parent key is the last segment of scopedPath, checking that exactly
// one of keys is present - the nested counterpart of ExactlyOneOf.
//
//	editor.ExactlyOneOfNested("categories.source", "git", "local")
func ExactlyOneOfNested(scopedPath string, keys ...string) Validator {
	segs := strings.Split(scopedPath, ".")
	return &exactlyOneOfNestedValidator{
		navSegs:   segs[:len(segs)-1],
		parentKey: segs[len(segs)-1],
		keys:      keys,
	}
}

type exactlyOneOfNestedValidator struct {
	navSegs   []string
	parentKey string
	keys      []string
}

func (v *exactlyOneOfNestedValidator) Validate(in ValidationInput) []Violation {
	var errs []Violation
	walkScopedMappings(in.Root, v.navSegs, v.parentKey, func(n *yaml.Node, where string) {
		errs = append(errs, exactlyOneViolation(v.keys, func(k string) bool {
			return yamlnode.ChildByKey(n, k) != nil
		}, where)...)
	})
	return errs
}

// AllOrNoneNested walks the YAML tree and fires at every mapping whose direct
// parent key is the last segment of scopedPath, checking that either all or
// none of keys are present - the nested counterpart of AllOrNone.
//
//	editor.AllOrNoneNested("servers.tls", "cert", "key")
func AllOrNoneNested(scopedPath string, keys ...string) Validator {
	segs := strings.Split(scopedPath, ".")
	return &allOrNoneNestedValidator{
		navSegs:   segs[:len(segs)-1],
		parentKey: segs[len(segs)-1],
		keys:      keys,
	}
}

type allOrNoneNestedValidator struct {
	navSegs   []string
	parentKey string
	keys      []string
}

func (v *allOrNoneNestedValidator) Validate(in ValidationInput) []Violation {
	var errs []Violation
	walkScopedMappings(in.Root, v.navSegs, v.parentKey, func(n *yaml.Node, where string) {
		errs = append(errs, allOrNoneViolation(v.keys, func(k string) bool {
			return yamlnode.ChildByKey(n, k) != nil
		}, where)...)
	})
	return errs
}
