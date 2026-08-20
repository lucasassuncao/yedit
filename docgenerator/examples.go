package docgenerator

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNoPresetSource is returned when WithExamples was given a nil presets.Source.
var ErrNoPresetSource = errors.New("docgenerator: WithExamples requires a non-nil presets.Source")

// emitExamples writes one markdown file per preset field listed in
// cfg.exampleTitles, containing the YAML of every preset that field exposes. The
// file is named after the lowercased title so it matches the reference page for
// the same type. Fields absent from the titles map, or with no presets, are
// skipped.
func emitExamples(cfg *config) ([]GeneratedFile, error) {
	if cfg.examplesSrc == nil {
		return nil, ErrNoPresetSource
	}
	if err := os.MkdirAll(cfg.examplesDir, 0750); err != nil {
		return nil, fmt.Errorf("create examples dir: %w", err)
	}

	var files []GeneratedFile
	for _, field := range cfg.examplesSrc.ListFields() {
		title, ok := cfg.exampleTitles[field]
		if !ok {
			continue
		}
		names := cfg.examplesSrc.ListPresets(field)
		if len(names) == 0 {
			continue
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "# %s Examples\n\n", title)
		for _, name := range names {
			y, err := cfg.examplesSrc.PresetYAML(field, name)
			if err != nil {
				return nil, fmt.Errorf("preset yaml for %s/%s: %w", field, name, err)
			}
			fmt.Fprintf(&sb, "## Preset: %s\n\n", name)
			sb.WriteString("```yaml\n")
			sb.WriteString(y)
			if !strings.HasSuffix(y, "\n") {
				sb.WriteString("\n")
			}
			sb.WriteString("```\n\n")
		}

		out := filepath.Join(filepath.Clean(cfg.examplesDir), strings.ToLower(title)+".md")
		valid, ok := validatePathWithinBase(cfg.examplesDir, out)
		if !ok {
			return nil, fmt.Errorf("invalid examples path: %s", out)
		}
		if err := os.WriteFile(valid, []byte(sb.String()), 0600); err != nil {
			return nil, fmt.Errorf("write example doc %s: %w", valid, err)
		}
		files = append(files, GeneratedFile{Name: title, Path: valid})
	}
	return files, nil
}
