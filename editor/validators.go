package editor

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/yamlnode"

	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/document"
)

// # Validator families
//
// FromMetadata family (RequiredFromMetadata, OneOfFromMetadata, etc.):
// requires a MetadataSource in editor.Config.Metadata, built via metadata.New
// (structs implement MetadataProvider) or metadata.NewFromTree (third-party
// structs). Inert until wired — editor.Run wires automatically; for standalone
// use call Wire before RunAll.
//
// Explicit family (Required, ValueOneOf, MutuallyExclusive, etc.):
// operates directly on raw YAML via path strings. No MetadataSource or
// MetadataProvider needed — the only option for apps whose structs cannot
// implement MetadataProvider, and the right choice for cross-field rules.
//
// Both families can be mixed in the same Validators slice.

// WiredValidators is an opaque handle produced by Wire. RunAll only accepts
// this type, which guarantees that FromMetadata validators have been wired
// before any validation run. The zero value is valid and produces no violations.
type WiredValidators struct{ validators []Validator }

// NewValidationInput parses raw once and bundles it with blocks for a
// validation run. Root is nil when raw is not valid YAML; an empty document
// yields an empty mapping so unconditional checks still run.
func NewValidationInput(raw []byte, blocks []document.Block) ValidationInput {
	root, _ := yamlnode.RootMapping(raw)
	return ValidationInput{Raw: raw, Root: root, Blocks: blocks}
}

// RunAll executes all validators against raw/blocks and collects violations.
// The document is parsed once and shared across validators.
// w must be produced by Wire; passing a zero WiredValidators is valid and
// always returns nil.
func RunAll(w WiredValidators, raw []byte, blocks []document.Block) []Violation {
	if len(w.validators) == 0 {
		return nil
	}
	in := NewValidationInput(raw, blocks)
	var errs []Violation
	for _, v := range w.validators {
		errs = append(errs, v.Validate(in)...)
	}
	return errs
}

// Wire prepares a validator slice for use with RunAll. It returns a
// WiredValidators where every FromMetadata validator (*metadataRuleValidator)
// is replaced by a shallow copy with the schema tree and MetadataSource
// injected. Explicit validators (MutuallyExclusive, Required, ValidatorFunc,
// etc.) are included as-is.
//
// The original slice is never modified, so the same global validator slice
// can be passed safely from multiple call sites or goroutines without
// interference. Wire is cheap to call repeatedly — schema discovery only
// runs when cfg.Schema is non-nil.
//
// Typical usage:
//
//	wired := editor.Wire(MyValidators, editor.Config{
//	    Schema:   &MySchema{},
//	    Metadata: hints,
//	})
//	violations := editor.RunAll(wired, raw, blocks)
//
// cfg.Schema must be non-nil for FromMetadata validators to fire; cfg.Metadata
// may be nil (FromMetadata validators will report nothing without a source).
//
// Callers that already hold the discovered schema tree (like the editor, which
// needs the same tree for its UI) should use WireWithSchema instead, so both
// sides are guaranteed to see the same schema.
func Wire(validators []Validator, cfg Config) WiredValidators {
	if cfg.Schema == nil {
		out := make([]Validator, len(validators))
		copy(out, validators)
		return WiredValidators{validators: out}
	}
	return WireWithSchema(validators, discoverSchema(cfg), cfg.Metadata)
}

// WireWithSchema is Wire for callers that already discovered the schema tree:
// it injects tree and metadata into every FromMetadata validator without
// re-running discovery. The editor uses it with the exact tree that drives the
// UI, making a divergence between what the screen shows and what the
// validators check impossible by construction.
func WireWithSchema(validators []Validator, tree []schema.FieldDef, metadata MetadataSource) WiredValidators {
	out := make([]Validator, len(validators))
	copy(out, validators)
	for i, v := range out {
		if rv, ok := v.(*metadataRuleValidator); ok {
			wired := *rv
			wired.defs = tree
			wired.hints = metadata
			out[i] = &wired
		}
	}
	return WiredValidators{validators: out}
}

// misconfiguredValidator reports a fixed configuration error on every run, so
// a rule built from invalid arguments surfaces on the first validate instead
// of silently never firing (same pattern as ValueMatches with a bad regex).
type misconfiguredValidator struct{ message string }

func (v *misconfiguredValidator) Validate(ValidationInput) []Violation {
	return []Violation{{Message: v.message}}
}

// walkScopedMappings navigates root to navSegs, then recursively visits every
// mapping whose direct parent key equals parentKey, calling onMatch with that
// mapping and its violation path (parentKey itself when the match is the
// navigated root). Sequences are expanded at every level. A nil root visits
// nothing. This is the shared traversal behind the three *Nested validators.
func walkScopedMappings(root *yaml.Node, navSegs []string, parentKey string, onMatch func(n *yaml.Node, where string)) {
	if root == nil {
		return
	}
	yamlnode.Navigate(root, navSegs, "", func(n *yaml.Node, p string) {
		walkScopedRec(n, "", p, parentKey, onMatch)
	})
}

// walkScopedRec is the recursion behind walkScopedMappings. curParentKey is the
// mapping key whose value is node; path is node's dot/index YAML path; target is
// the parent key that triggers onMatch.
func walkScopedRec(node *yaml.Node, curParentKey, path, target string, onMatch func(n *yaml.Node, where string)) {
	switch node.Kind {
	case yaml.MappingNode:
		if curParentKey == target {
			where := path
			if where == "" {
				where = target
			}
			onMatch(node, where)
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			walkScopedRec(node.Content[i+1], key, childPath, target, onMatch)
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			walkScopedRec(child, curParentKey, fmt.Sprintf("%s[%d]", path, i), target, onMatch)
		}
	}
}

// reportDuplicates appends a violation for every value that repeats an earlier
// one. Empty values are skipped. The violation path is "<where>[<i>]<suffix>".
func reportDuplicates(values []string, where, suffix string, errs *[]Violation) {
	seen := make(map[string]int, len(values))
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

// reportDuplicateScalars collects the scalar values of seq (non-scalar items
// yield empty entries, which reportDuplicates skips) and reports duplicates at
// where+suffix. Shared by UniqueValues and UniqueFromMetadata.
func reportDuplicateScalars(seq *yaml.Node, where, suffix string, errs *[]Violation) {
	values := make([]string, len(seq.Content))
	for i, item := range seq.Content {
		if item.Kind == yaml.ScalarNode {
			values[i] = item.Value
		}
	}
	reportDuplicates(values, where, suffix, errs)
}

// oneOfViolation appends a "value not allowed" violation when value is not
// among allowed. Shared by ValueOneOf and OneOfFromMetadata.
func oneOfViolation(value, where string, allowed []string, errs *[]Violation) {
	for _, a := range allowed {
		if value == a {
			return
		}
	}
	*errs = append(*errs, Violation{
		Path:    where,
		Message: fmt.Sprintf("value %q is not allowed - use one of: %s", value, joinQuoted(allowed)),
	})
}

// patternMatchViolation appends a "does not match pattern" violation when value
// fails re. Shared by ValueMatches and PatternFromMetadata; each family handles
// pattern compilation and invalid-pattern reporting on its own.
func patternMatchViolation(value, pattern, where string, re *regexp.Regexp, errs *[]Violation) {
	if !re.MatchString(value) {
		*errs = append(*errs, Violation{
			Path:    where,
			Message: fmt.Sprintf("value %q does not match pattern %q", value, pattern),
		})
	}
}

// collectionCount returns the entry count of a sequence (items) or mapping
// (keys). ok is false for any other node kind.
func collectionCount(node *yaml.Node) (int, bool) {
	switch node.Kind {
	case yaml.SequenceNode:
		return len(node.Content), true
	case yaml.MappingNode:
		return len(node.Content) / 2, true
	}
	return 0, false
}

// countRangeViolation appends a violation when count is below minCount or above
// maxCount. maxCount < 0 means no upper bound. Shared by CountRange and
// CountFromMetadata (the latter maps its MaxCount==0 sentinel to -1).
func countRangeViolation(count, minCount, maxCount int, where string, errs *[]Violation) {
	if count < minCount || (maxCount >= 0 && count > maxCount) {
		want := fmt.Sprintf("between %d and %d", minCount, maxCount)
		if maxCount < 0 {
			want = fmt.Sprintf("at least %d", minCount)
		}
		*errs = append(*errs, Violation{
			Path:    where,
			Message: fmt.Sprintf("has %d entries - expected %s", count, want),
		})
	}
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
			// ParseFloat rejects trailing garbage that Sscanf would accept as a
			// prefix ("10xB", "1.5junkGB"), so malformed values are reported as
			// not comparable instead of silently compared. The longest suffix
			// wins; a shorter one must not reinterpret the malformed rest.
			n, err := strconv.ParseFloat(numStr, 64)
			if err == nil && n >= 0 {
				return int64(n * float64(u.mult)), true
			}
			return 0, false
		}
	}
	return 0, false
}

// isEmptyScalar reports whether a scalar node carries no usable value: an
// explicit null ("key: null", "key: ~", or a bare "key:") or an empty string.
// yaml.v3 gives explicit nulls the literal Value "null"/"~" with Tag "!!null",
// so checking Value alone would feed the literal string to value rules.
func isEmptyScalar(n *yaml.Node) bool {
	return n.Value == "" || n.Tag == "!!null"
}

// forEachScalar visits every scalar reached by the dotted path - sequences and
// dict-style mappings along the path are expanded automatically - and calls fn
// with the value and its expanded path. It encodes the shared contract of the
// value validators: a non-scalar leaf is flagged as a violation, and absent,
// null, or empty values report nothing (combine with Required when the field
// is mandatory).
func forEachScalar(root *yaml.Node, path string, errs *[]Violation, fn func(value, where string)) {
	if root == nil {
		return
	}
	yamlnode.ForEachLeaf(root, path, func(node *yaml.Node, where string) {
		if node.Kind != yaml.ScalarNode {
			*errs = append(*errs, Violation{Path: where, Message: "expected a scalar value"})
			return
		}
		if isEmptyScalar(node) {
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
