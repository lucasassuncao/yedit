// Package editor provides the bubbletea TUI for editing a YAML file driven by
// a struct-based schema and a preset source.
package editor

import (
	tea "charm.land/bubbletea/v2"

	"github.com/lucasassuncao/yedit/document"
	"github.com/lucasassuncao/yedit/presets"
	"github.com/lucasassuncao/yedit/spec"
	"github.com/lucasassuncao/yedit/theme"
)

// ─── Shared vocabulary ───────────────────────────────────────────────────────

// These names moved to yedit/spec so that metadata, docgenerator, validate, and
// third-party rules can describe a field without importing the TUI. They are
// aliases, not new types: editor.FieldMeta and spec.FieldMeta are the same type,
// so existing consumer code keeps compiling.
type (
	FieldMeta       = spec.FieldMeta
	MetadataSource  = spec.MetadataSource
	MetadataFunc    = spec.MetadataFunc
	Format          = spec.Format
	Violation       = spec.Violation
	ValidationInput = spec.ValidationInput
	Validator       = spec.Validator
	ValidatorFunc   = spec.ValidatorFunc

	// group keeps the old unexported spelling working inside this package.
	group = spec.Group
)

const (
	GroupMutuallyExclusive = spec.GroupMutuallyExclusive
	GroupUnknownKeys       = spec.GroupUnknownKeys
	GroupRules             = spec.GroupRules
)

// FormatCustom builds an app-specific format. See spec.FormatCustom.
func FormatCustom(name string, validate func(string) bool) Format {
	return spec.FormatCustom(name, validate)
}

// Built-in formats, re-exported from spec.
var (
	FormatCIDR            = spec.FormatCIDR
	FormatDate            = spec.FormatDate
	FormatDirectoryPath   = spec.FormatDirectoryPath
	FormatDuration        = spec.FormatDuration
	FormatEmail           = spec.FormatEmail
	FormatFQDN            = spec.FormatFQDN
	FormatGitRef          = spec.FormatGitRef
	FormatHost            = spec.FormatHost
	FormatHostPort        = spec.FormatHostPort
	FormatIP              = spec.FormatIP
	FormatIPv4            = spec.FormatIPv4
	FormatIPv6            = spec.FormatIPv6
	FormatPort            = spec.FormatPort
	FormatPrivateKey      = spec.FormatPrivateKey
	FormatPublicKey       = spec.FormatPublicKey
	FormatSemver          = spec.FormatSemver
	FormatTerraformSource = spec.FormatTerraformSource
	FormatURL             = spec.FormatURL
	FormatUUID            = spec.FormatUUID
)

// NewValidationInput parses raw once and bundles it with blocks for a
// validation run. See spec.NewValidationInput.
func NewValidationInput(raw []byte, blocks []document.Block) ValidationInput {
	return spec.NewValidationInput(raw, blocks)
}

// ─── Session tracing ─────────────────────────────────────────────────────────

// Trace bundles the editor's session-observability hooks: the OnAction/
// OnModelAction/OnMsg callbacks and the built-in Dump-to-JSONL recorder built
// on top of them. See docs/SESSION-TRACING.md for the full picture.
type Trace struct {
	OnAction      func(blockKey string, a BlockAction) // optional; called synchronously after every BlockAction is dispatched, with the key of the block editor it was applied to (e.g. for session tracing)
	OnModelAction func(ModelAction)                    // optional; called synchronously after every ModelAction is dispatched (e.g. for session tracing)
	OnMsg         func(where string, msg tea.Msg)      // optional; called synchronously for every raw tea.Msg the program receives (every keystroke, resize, etc.), before it is routed. where describes the active pane/block/panel at the time (e.g. "list", "block:categories:tree:editing")
	Dump          bool                                 // when true, records every action and keystroke to a JSONL file; the path is reported in Result.DumpPath. Composes with OnAction/OnModelAction/OnMsg if those are also set.
	DumpPath      string                               // optional explicit path for the Dump trace file; ignored when Dump is false. Empty falls back to a timestamped file in the OS temp dir.
}

// ─── Config ──────────────────────────────────────────────────────────────────

// Config bundles everything the editor needs from the embedding application.
//
// Schema must be a pointer to the Go type describing the YAML document's top
// level (e.g. &MyConfig{}). The editor introspects it through yedit/schema.
//
// Presets is optional - when nil the editor opens fresh blocks with a minimal
// "<key>:\n" template and the preset picker is disabled.
//
// Validators run before every save and on the explicit "validate" shortcut.
// Use editor.MutuallyExclusive and editor.RequiredWith for the common cases.
//
// Hints is optional - when set, each field's Hint/Example panel is populated
// from the returned FieldMeta. All FieldMeta fields are used as-is; yedit
// does not fall back to struct tag values. When Hints is nil, the panel shows
// only a generated example.
//
// FieldMeta.PreChecked lists sub-fields that start checked when a new block
// overlay opens. FieldMeta.Snippet provides the YAML inserted when a sub-field
// is toggled on; falls back to "<fieldName>: \n" when empty.
type Config struct {
	Path                 string         // YAML file to load; also the default save target when SavePath is empty
	Schema               any            // non-nil struct pointer; typed as any because the editor uses reflection (e.g. &MyConfig{})
	Title                string         // label shown in the TUI header
	BlockPresets         presets.Source // optional; nil disables the preset picker inside block editors
	DocPresets           presets.Source // optional; when set, p on the root list opens a whole-document template picker
	EnableHints          bool           // show the Hint/Example panel; requires Metadata to be set (a warning is shown if it is not)
	Metadata             MetadataSource // field metadata displayed in the hint panel and enforced by the FromMetadata validators
	Validators           []Validator    // rules evaluated before every save and on the validate shortcut
	Hidden               []string       // top-level keys to omit from the UI entirely
	PassthroughKeys      []string       // top-level keys preserved as-is; hidden from all sections and excluded from unknown-key validation
	Theme                theme.Theme    // zero-value resolves to ThemePlain
	NoDeleteConfirm      bool           // skip the "Remove block?" confirmation dialog; deletion is still undoable via ctrl+u
	NoValidateOnSave     bool           // allow saving even when validators report errors; a warning alert is shown but does not block
	NoSaveConfirm        bool           // skip the "Save changes?" confirmation dialog; warning confirms (NoValidateOnSave) are still shown
	SavePath             string         // write to this path instead of Path; Path is still used for loading
	SchemaRecursionDepth int            // extra levels a self-referential type expands (e.g. CategoryFilter.Any []CategoryFilter); 0 uses the default (1)
	Trace                Trace          // session-observability hooks (OnAction/OnModelAction/OnMsg) and the built-in Dump recorder
}
