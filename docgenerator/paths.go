package docgenerator

import (
	"path/filepath"
	"reflect"
	"strings"

	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/spec"
)

// discoverEntry runs schema discovery for one entry, honouring its
// RecursionLimit override.
func discoverEntry(e Entry) []schema.FieldDef {
	if e.RecursionLimit != nil {
		return schema.Discover(e.Config, *e.RecursionLimit)
	}
	return schema.Discover(e.Config)
}

// typeName returns the Go type name of v with pointers dereferenced. It is the
// display title and the basename of every file generated for an entry.
func typeName(v any) string {
	t := reflect.TypeOf(v)
	if t == nil {
		return ""
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Name()
}

// fieldMetaFor translates (sectionPath, fieldName) to MetadataSource's
// (blockKey, fieldPath) coordinates:
//
//   - sectionPath empty → blockKey = fieldName, fieldPath = ""
//   - sectionPath ["build"] → blockKey = "build", fieldPath = fieldName
//   - sectionPath ["categories","source"] → blockKey = "categories", fieldPath = "source.fieldName"
//
// A nil source yields a zero FieldMeta, which every consumer reads as "declares
// nothing".
func fieldMetaFor(src spec.MetadataSource, sectionPath []string, fieldName string) spec.FieldMeta {
	if src == nil {
		return spec.FieldMeta{}
	}
	if len(sectionPath) == 0 {
		return src.FieldMeta(fieldName, "")
	}
	blockKey := sectionPath[0]
	fieldPath := fieldName
	if len(sectionPath) > 1 {
		fieldPath = strings.Join(sectionPath[1:], ".") + "." + fieldName
	}
	return src.FieldMeta(blockKey, fieldPath)
}

// validatePathWithinBase rejects a target that escapes baseDir, guarding against
// a type or title name that resolves outside the output directory.
func validatePathWithinBase(baseDir, targetPath string) (string, bool) {
	cleanBase := filepath.Clean(baseDir)
	cleanTarget := filepath.Clean(targetPath)
	rel, err := filepath.Rel(cleanBase, cleanTarget)
	if err != nil {
		return "", false
	}
	if rel == "." {
		return cleanBase, true
	}
	if strings.HasPrefix(rel, "..") {
		return "", false
	}
	return cleanTarget, true
}

// childSectionPath appends name to path without aliasing path's backing array.
func childSectionPath(path []string, name string) []string {
	return append(append([]string(nil), path...), name)
}
