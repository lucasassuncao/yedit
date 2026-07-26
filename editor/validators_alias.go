package editor

import (
	"github.com/lucasassuncao/yedit/spec"
	"github.com/lucasassuncao/yedit/validate"
)

// The validation rules moved to yedit/validate so they can be written and run
// without importing the TUI. Everything below re-exports them under their
// original editor names, so existing consumer code keeps compiling.
//
// Wire is the exception: it takes an editor.Config and has to discover the
// schema from it, which is why it stays here rather than moving with the rules.

// WiredValidators is an opaque handle produced by Wire. See validate.WiredValidators.
type WiredValidators = validate.WiredValidators

// Wire prepares a validator slice for use with RunAll, discovering the schema
// tree from cfg so that FromMetadata validators can fire. It returns a handle
// where every FromMetadata validator carries the schema and MetadataSource;
// explicit validators are included as-is. The original slice is never modified.
//
// cfg.Schema must be non-nil for FromMetadata validators to report anything;
// cfg.Metadata may be nil. Callers that already hold the discovered tree should
// use validate.WireWithSchema directly, so both sides see the same schema.
func Wire(validators []spec.Validator, cfg Config) WiredValidators {
	if cfg.Schema == nil {
		return validate.WireNoSchema(validators)
	}
	return validate.WireWithSchema(validators, discoverSchema(cfg), cfg.Metadata)
}

// RunAll executes all validators against raw/blocks. See validate.RunAll.
var RunAll = validate.RunAll

// Explicit rules: operate directly on raw YAML via path strings.
var (
	AllOrNone                     = validate.AllOrNone
	AllOrNoneNested               = validate.AllOrNoneNested
	AtLeastOneOf                  = validate.AtLeastOneOf
	AtLeastOneOfNested            = validate.AtLeastOneOfNested
	CountRange                    = validate.CountRange
	CrossFieldOrdered             = validate.CrossFieldOrdered
	CrossFieldOrderedNested       = validate.CrossFieldOrderedNested
	Deprecated                    = validate.Deprecated
	ExactlyOneOf                  = validate.ExactlyOneOf
	ExactlyOneOfNested            = validate.ExactlyOneOfNested
	ForbiddenIf                   = validate.ForbiddenIf
	MutuallyExclusive             = validate.MutuallyExclusive
	MutuallyExclusiveGroupsNested = validate.MutuallyExclusiveGroupsNested
	MutuallyExclusiveNested       = validate.MutuallyExclusiveNested
	NoDuplicates                  = validate.NoDuplicates
	Required                      = validate.Required
	RequiredIf                    = validate.RequiredIf
	RequiredWith                  = validate.RequiredWith
	UniqueValues                  = validate.UniqueValues
	ValueHasLength                = validate.ValueHasLength
	ValueHasPrefix                = validate.ValueHasPrefix
	ValueHasSuffix                = validate.ValueHasSuffix
	ValueInRange                  = validate.ValueInRange
	ValueMatches                  = validate.ValueMatches
	ValueMatchesFormat            = validate.ValueMatchesFormat
	ValueNotOneOf                 = validate.ValueNotOneOf
	ValueOneOf                    = validate.ValueOneOf
)

// FromMetadata rules: driven by Config.Metadata, inert until wired.
var (
	CountFromMetadata      = validate.CountFromMetadata
	DeprecatedFromMetadata = validate.DeprecatedFromMetadata
	FormatFromMetadata     = validate.FormatFromMetadata
	LengthFromMetadata     = validate.LengthFromMetadata
	NotOneOfFromMetadata   = validate.NotOneOfFromMetadata
	OneOfFromMetadata      = validate.OneOfFromMetadata
	PatternFromMetadata    = validate.PatternFromMetadata
	RangeFromMetadata      = validate.RangeFromMetadata
	RequiredFromMetadata   = validate.RequiredFromMetadata
	UniqueFromMetadata     = validate.UniqueFromMetadata
)
