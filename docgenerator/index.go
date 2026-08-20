package docgenerator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// emitIndex writes a README.md to dir linking to the generated reference and
// example pages. Sections with no files are omitted. Links are computed from
// each GeneratedFile.Path, so the filename rule lives in exactly one place.
func emitIndex(dir string, pages, examples []GeneratedFile) (GeneratedFile, error) {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return GeneratedFile{}, fmt.Errorf("create index dir: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("# Documentation Index\n\n")
	sb.WriteString("This documentation describes all available configuration structures.\n\n")
	if err := writeIndexSection(&sb, dir, "Available Configurations", pages); err != nil {
		return GeneratedFile{}, err
	}
	if err := writeIndexSection(&sb, dir, "Examples", examples); err != nil {
		return GeneratedFile{}, err
	}

	out := filepath.Join(dir, "README.md")
	valid, ok := validatePathWithinBase(dir, out)
	if !ok {
		return GeneratedFile{}, fmt.Errorf("invalid index path: %s", out)
	}
	if err := os.WriteFile(valid, []byte(sb.String()), 0600); err != nil {
		return GeneratedFile{}, fmt.Errorf("write index %s: %w", valid, err)
	}
	return GeneratedFile{Name: "Documentation Index", Path: valid}, nil
}

func writeIndexSection(sb *strings.Builder, baseDir, title string, files []GeneratedFile) error {
	if len(files) == 0 {
		return nil
	}
	fmt.Fprintf(sb, "## %s\n\n", title)
	for _, f := range files {
		rel, err := filepath.Rel(baseDir, f.Path)
		if err != nil {
			return fmt.Errorf("compute relative path for %s: %w", f.Name, err)
		}
		fmt.Fprintf(sb, "- [%s](./%s)\n", f.Name, filepath.ToSlash(rel))
	}
	sb.WriteString("\n")
	return nil
}
