// Package presets defines the Source interface for per-field YAML presets and
// provides a filesystem-backed implementation.
//
// Clients can either implement Source directly (e.g. as a thin adapter over
// an existing preset registry) or pass an fs.FS to FromFS to get a default
// implementation that loads presets from a YAML tree.
package presets

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// Source supplies YAML preset snippets keyed by (field, preset name). The
// editor uses it to populate the preset picker and to seed the YAML editor
// when a block is opened.
type Source interface {
	// ListFields returns the field names that have at least one preset.
	ListFields() []string

	// ListPresets returns the preset names available for the given field,
	// or an empty slice if the field has no presets.
	ListPresets(field string) []string

	// PresetYAML returns the YAML snippet for (field, name) or an error if
	// either is unknown.
	PresetYAML(field, name string) (string, error)
}

// FromFS returns a Source backed by an fs.FS rooted at root (use "." for the
// whole filesystem).
//
// Layout convention:
//
//	<root>/<field>/<preset>.yaml
//
// ListFields enumerates the directories under root; ListPresets enumerates
// the .yaml files within each field directory (stripping the extension).
//
// FromFS is convenient for clients that ship presets as a Go embed.FS.
func FromFS(fsys fs.FS, root string) Source {
	return &fsSource{fsys: fsys, root: root}
}

type fsSource struct {
	fsys fs.FS
	root string
}

func (s *fsSource) ListFields() []string {
	entries, err := fs.ReadDir(s.fsys, s.root)
	if err != nil {
		return nil
	}
	var fields []string
	for _, e := range entries {
		if e.IsDir() {
			fields = append(fields, e.Name())
		}
	}
	sort.Strings(fields)
	return fields
}

func (s *fsSource) ListPresets(field string) []string {
	dir := s.root + "/" + field
	entries, err := fs.ReadDir(s.fsys, dir)
	if err != nil {
		return nil
	}
	var presets []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".yaml") {
			presets = append(presets, strings.TrimSuffix(name, ".yaml"))
		}
	}
	sort.Strings(presets)
	return presets
}

func (s *fsSource) PresetYAML(field, name string) (string, error) {
	path := s.root + "/" + field + "/" + name + ".yaml"
	data, err := fs.ReadFile(s.fsys, path)
	if err != nil {
		return "", fmt.Errorf("preset %q for field %q: %w", name, field, err)
	}
	return string(data), nil
}
