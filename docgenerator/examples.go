package docgenerator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lucasassuncao/yedit/presets"
)

// GenerateExampleDocs writes one markdown file per preset field that has an
// entry in titles, each containing the YAML for every preset that field
// exposes. titles maps a presets.Source field to the display title used for
// its page (e.g. "authorizationpolicies" -> "AuthorizationPolicy"); the file is
// named after the lowercased title so it matches the documentation page for the
// same type. Fields absent from titles (or with no presets) are skipped.
func GenerateExampleDocs(examplesDir string, src presets.Source, titles map[string]string) ([]GeneratedFile, error) {
	if err := os.MkdirAll(examplesDir, 0750); err != nil {
		return nil, fmt.Errorf("create examples dir: %w", err)
	}

	var files []GeneratedFile
	for _, field := range src.ListFields() {
		title, ok := titles[field]
		if !ok {
			continue
		}
		names := src.ListPresets(field)
		if len(names) == 0 {
			continue
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# %s Examples\n\n", title))
		for _, name := range names {
			yaml, err := src.PresetYAML(field, name)
			if err != nil {
				return nil, fmt.Errorf("preset yaml for %s/%s: %w", field, name, err)
			}
			sb.WriteString(fmt.Sprintf("## Preset: %s\n\n", name))
			sb.WriteString("```yaml\n")
			sb.WriteString(yaml)
			if !strings.HasSuffix(yaml, "\n") {
				sb.WriteString("\n")
			}
			sb.WriteString("```\n\n")
		}

		out := filepath.Join(examplesDir, strings.ToLower(title)+".md")
		if err := os.WriteFile(out, []byte(sb.String()), 0600); err != nil {
			return nil, fmt.Errorf("write example doc %s: %w", out, err)
		}
		files = append(files, GeneratedFile{Name: title, DocsDir: examplesDir})
	}

	return files, nil
}
