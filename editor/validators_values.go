package editor

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/yamlnode"
)

// This file holds the single-value rules of the explicit validator family:
// each rule inspects the scalar (or collection) reached by one dotted path.
// Shared traversal and violation helpers live in validators.go.

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
		oneOfViolation(value, where, v.allowed, &errs)
	})
	return errs
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
		patternMatchViolation(value, v.pattern, where, v.re, &errs)
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
		count, ok := collectionCount(node)
		if !ok {
			// A null or empty scalar ("key:", "key: null", "key: ~") is an
			// empty collection, consistent with checkHintCount; anything else
			// scalar really is the wrong shape.
			if node.Kind != yaml.ScalarNode || !isEmptyScalar(node) {
				errs = append(errs, Violation{Path: where, Message: "expected a list or mapping"})
				return
			}
			count = 0
		}
		countRangeViolation(count, v.min, v.max, where, &errs)
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
		reportDuplicateScalars(seqNode, where, "", &errs)
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

// ValueNotOneOf reports a violation when the scalar at path is present and its
// value is in the denied list. Case-sensitive. An absent or empty value reports
// nothing - the inverse of ValueOneOf.
//
//	editor.ValueNotOneOf("protocol", "ftp", "telnet")
func ValueNotOneOf(path string, denied ...string) Validator {
	return &valueNotOneOfValidator{path: path, denied: denied}
}

type valueNotOneOfValidator struct {
	path   string
	denied []string
}

func (v *valueNotOneOfValidator) Validate(in ValidationInput) []Violation {
	var errs []Violation
	forEachScalar(in.Root, v.path, &errs, func(value, where string) {
		for _, d := range v.denied {
			if value == d {
				errs = append(errs, Violation{
					Path:    where,
					Message: fmt.Sprintf("value %q is not allowed", value),
				})
				return
			}
		}
	})
	return errs
}

// ValueHasLength reports a violation when the scalar at path is present but its
// Unicode code point count falls outside [min, max]. A zero bound means no rule
// for that side. An absent or empty value reports nothing - combine with Required
// when the field is mandatory. Sequences and dict-style mappings along the path
// are expanded automatically.
//
//	editor.ValueHasLength("name", 3, 64)
//	editor.ValueHasLength("description", 0, 500) // max only
func ValueHasLength(path string, min, max int) Validator {
	return &valueHasLengthValidator{path: path, min: min, max: max}
}

type valueHasLengthValidator struct {
	path     string
	min, max int
}

func (v *valueHasLengthValidator) Validate(in ValidationInput) []Violation {
	var errs []Violation
	forEachScalar(in.Root, v.path, &errs, func(value, where string) {
		n := len([]rune(value))
		switch {
		case v.min > 0 && v.max > 0 && (n < v.min || n > v.max):
			errs = append(errs, Violation{Path: where, Message: fmt.Sprintf("must be between %d and %d chars", v.min, v.max)})
		case v.min > 0 && n < v.min:
			errs = append(errs, Violation{Path: where, Message: fmt.Sprintf("must be at least %d chars", v.min)})
		case v.max > 0 && n > v.max:
			errs = append(errs, Violation{Path: where, Message: fmt.Sprintf("must be at most %d chars", v.max)})
		}
	})
	return errs
}

// ValueMatchesFormat reports a violation when the scalar at path is present but
// does not match any of the given formats (OR semantics: valid if any one
// matches). An absent or empty value reports nothing. Sequences and dict-style
// mappings along the path are expanded automatically.
//
//	editor.ValueMatchesFormat("endpoint", editor.FormatURL, editor.FormatHost)
func ValueMatchesFormat(path string, formats ...Format) Validator {
	return &valueMatchesFormatValidator{path: path, formats: formats}
}

type valueMatchesFormatValidator struct {
	path    string
	formats []Format
}

func (v *valueMatchesFormatValidator) Validate(in ValidationInput) []Violation {
	var errs []Violation
	forEachScalar(in.Root, v.path, &errs, func(value, where string) {
		for _, f := range v.formats {
			if !f.IsZero() && f.validate(value) {
				return
			}
		}
		labels := make([]string, 0, len(v.formats))
		for _, f := range v.formats {
			if !f.IsZero() {
				labels = append(labels, f.Label())
			}
		}
		errs = append(errs, Violation{
			Path:    where,
			Message: "value does not match expected format: " + strings.Join(labels, " | "),
		})
	})
	return errs
}
