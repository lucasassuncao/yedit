package editor

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/lucasassuncao/yedit/yamlnode"

	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/schema"
)

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
	oneOfViolation(val, path, meta.OneOf, errs)
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
	patternMatchViolation(val, meta.Pattern, path, re, errs)
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
	count, ok := collectionCount(child)
	if !ok {
		if child.Value != "" { // a non-null scalar is not a collection
			*errs = append(*errs, Violation{Path: path, Message: "expected a list or mapping"})
			return
		}
		// null scalar: an empty collection - count stays 0
	}
	maxCount := meta.MaxCount
	if maxCount == 0 {
		maxCount = -1 // MaxCount 0 means no upper bound
	}
	countRangeViolation(count, meta.MinCount, maxCount, path, errs)
}

// UniqueFromMetadata enforces FieldMeta.Unique from the MetadataSource (UniqueValues
// semantics): scalar items in the sequence must not repeat. Non-sequence
// fields and non-scalar items are skipped.
func UniqueFromMetadata() Validator { return &metadataRuleValidator{check: checkHintUnique} }

func checkHintUnique(meta FieldMeta, child *yaml.Node, path string, errs *[]Violation) {
	if !meta.Unique || child == nil || child.Kind != yaml.SequenceNode {
		return
	}
	reportDuplicateScalars(child, path, "", errs)
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

// FormatFromMetadata enforces FieldMeta.Formats from the MetadataSource.
// A present, non-empty scalar value is valid if it matches any of the declared
// formats (OR semantics). Skips fields where Formats is empty or value is empty.
func FormatFromMetadata() Validator { return &metadataRuleValidator{check: checkHintFormat} }

func checkHintFormat(meta FieldMeta, child *yaml.Node, path string, errs *[]Violation) {
	if len(meta.Formats) == 0 {
		return
	}
	if child == nil || child.Kind != yaml.ScalarNode || child.Value == "" {
		return
	}
	for _, f := range meta.Formats {
		if !f.IsZero() && f.validate(child.Value) {
			return
		}
	}
	labels := make([]string, 0, len(meta.Formats))
	for _, f := range meta.Formats {
		if !f.IsZero() {
			labels = append(labels, f.Label())
		}
	}
	*errs = append(*errs, Violation{
		Path:    path,
		Message: "value does not match expected format: " + strings.Join(labels, " | "),
	})
}

// LengthFromMetadata enforces FieldMeta.MinLength/MaxLength from the
// MetadataSource. Length is measured in Unicode code points. A zero value
// for either bound means no rule for that bound.
func LengthFromMetadata() Validator { return &metadataRuleValidator{check: checkHintLength} }

func checkHintLength(meta FieldMeta, child *yaml.Node, path string, errs *[]Violation) {
	if meta.MinLength == 0 && meta.MaxLength == 0 {
		return
	}
	if child == nil || child.Kind != yaml.ScalarNode || child.Value == "" {
		return
	}
	n := len([]rune(child.Value))
	switch {
	case meta.MinLength > 0 && meta.MaxLength > 0 && (n < meta.MinLength || n > meta.MaxLength):
		*errs = append(*errs, Violation{
			Path:    path,
			Message: fmt.Sprintf("must be between %d and %d chars", meta.MinLength, meta.MaxLength),
		})
	case meta.MinLength > 0 && n < meta.MinLength:
		*errs = append(*errs, Violation{
			Path:    path,
			Message: fmt.Sprintf("must be at least %d chars", meta.MinLength),
		})
	case meta.MaxLength > 0 && n > meta.MaxLength:
		*errs = append(*errs, Violation{
			Path:    path,
			Message: fmt.Sprintf("must be at most %d chars", meta.MaxLength),
		})
	}
}

// NotOneOfFromMetadata enforces FieldMeta.NotOneOf from the MetadataSource.
// A present, non-empty scalar whose value is in the denylist is a violation.
// Matching is case-sensitive. Skips fields where NotOneOf is empty or value
// is empty.
func NotOneOfFromMetadata() Validator { return &metadataRuleValidator{check: checkHintNotOneOf} }

func checkHintNotOneOf(meta FieldMeta, child *yaml.Node, path string, errs *[]Violation) {
	if len(meta.NotOneOf) == 0 {
		return
	}
	if child == nil || child.Kind != yaml.ScalarNode || child.Value == "" {
		return
	}
	for _, denied := range meta.NotOneOf {
		if child.Value == denied {
			*errs = append(*errs, Violation{
				Path:    path,
				Message: fmt.Sprintf("value %q is not allowed", child.Value),
			})
			return
		}
	}
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
