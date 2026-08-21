// Package editor provides the bubbletea TUI for editing a YAML file driven by
// a struct-based schema and a preset source.
package editor

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/lucasassuncao/yedit/document"
	"github.com/lucasassuncao/yedit/presets"
	"github.com/lucasassuncao/yedit/spec"
	"github.com/lucasassuncao/yedit/theme"
)

// These names live in yedit/spec so metadata, docgenerator, validate, and
// third-party rules can describe a field without importing the TUI. They are
// aliases, not new types, so consumer code keeps compiling.
type (
	FieldMeta       = spec.FieldMeta
	MetadataSource  = spec.MetadataSource
	MetadataFunc    = spec.MetadataFunc
	Format          = spec.Format
	Violation       = spec.Violation
	ValidationInput = spec.ValidationInput
	Validator       = spec.Validator
	ValidatorFunc   = spec.ValidatorFunc
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

// Trace bundles the editor's session-observability hooks and the built-in
// Dump-to-JSONL recorder built on them. See docs/SESSION-TRACING.md.
type Trace struct {
	OnAction      func(blockKey string, a BlockAction) // optional; called after every BlockAction, with the key of the block editor it applied to
	OnModelAction func(ModelAction)                    // optional; called after every ModelAction
	OnMsg         func(where string, msg tea.Msg)      // optional; called for every raw tea.Msg before it is routed. where describes the active pane/block/panel (e.g. "block:categories:tree:editing")
	Dump          bool                                 // record every action and keystroke to a JSONL file, reported in Result.DumpPath; composes with the callbacks above
	DumpPath      string                               // explicit path for the Dump file; empty falls back to a timestamped file in the OS temp dir
}

// Config bundles everything the editor needs from the embedding application.
//
// Schema must be a pointer to the Go type describing the YAML document's top
// level (e.g. &MyConfig{}), introspected through yedit/schema.
//
// Validators run before every save and on the explicit "validate" shortcut. Use
// editor.MutuallyExclusive and editor.RequiredWith for the common cases.
//
// Metadata populates each field's Hint/Example panel. FieldMeta values are used
// as-is; yedit never falls back to struct tags. FieldMeta.PreChecked lists
// sub-fields that start checked in a new block, and FieldMeta.Snippet is the YAML
// inserted when a sub-field is toggled on, defaulting to "<fieldName>: \n".
type Config struct {
	Path                 string         // YAML file to load; also the default save target when SavePath is empty
	Schema               any            // non-nil struct pointer, typed as any because the editor uses reflection (e.g. &MyConfig{})
	Title                string         // label shown in the TUI header
	BlockPresets         presets.Source // optional; nil disables the preset picker inside block editors
	DocPresets           presets.Source // optional; when set, p on the root list opens a whole-document template picker
	EnableHints          bool           // show the Hint/Example panel; warns when Metadata is unset
	Metadata             MetadataSource // field metadata shown in the hint panel and enforced by the FromMetadata validators
	Validators           []Validator    // rules evaluated before every save and on the validate shortcut
	Hidden               []string       // top-level keys to omit from the UI entirely
	PassthroughKeys      []string       // top-level keys preserved as-is: hidden from all sections and exempt from unknown-key validation
	Theme                theme.Theme    // zero-value resolves to ThemePlain
	NoDeleteConfirm      bool           // skip the "Remove block?" dialog; deletion is still undoable via ctrl+u
	NoValidateOnSave     bool           // allow saving despite validator errors; a warning alert is shown but does not block
	NoSaveConfirm        bool           // skip the "Save changes?" dialog; warning confirms are still shown
	SavePath             string         // write here instead of Path, which is still used for loading
	SchemaRecursionDepth int            // extra levels a self-referential type expands; 0 uses the default (1)
	AnimationDuration    time.Duration  // when > 0, the Hint/Example panel eases open and closed over this duration; 0 keeps the toggle instant and emits no timer messages
	Trace                Trace          // session-observability hooks and the built-in Dump recorder
}
