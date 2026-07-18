package schema

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// KnownChildren collapses a FieldDef tree into a map of dotted paths to the
// set of allowed direct children. Used by UnknownKeys to detect typos.
//
// A nil value at a path means "free-form" - children at that path are not
// validated (e.g. customizations.vscode.settings has no fixed schema).
func KnownChildren(fields []FieldDef) map[string]map[string]bool {
	out := make(map[string]map[string]bool, len(fields))
	walkChildren(out, "", fields)
	return out
}

func walkChildren(out map[string]map[string]bool, prefix string, fields []FieldDef) {
	if len(fields) == 0 {
		return
	}
	allowed := make(map[string]bool, len(fields))
	for _, f := range fields {
		allowed[f.YAMLName] = true
	}
	out[prefix] = allowed
	for _, f := range fields {
		// A map's keys are free-form (user-chosen), so its value-struct's fields
		// must not be registered as the allowed keys under it. Leaving the path
		// unregistered makes UnknownKeys treat the sub-tree as free-form.
		if f.Kind == KindDictionary {
			continue
		}
		path := f.YAMLName
		if prefix != "" {
			path = prefix + "." + f.YAMLName
		}
		walkChildren(out, path, f.Children)
	}
}

// UnknownKeys returns the dotted paths of any YAML keys not present in the
// schema described by known. Free-form sub-trees (paths missing from known)
// are not validated. Returns an error if raw does not parse as YAML.
func UnknownKeys(raw []byte, known map[string]map[string]bool) ([]string, error) {
	// Decode into map[any]any so documents with non-string keys still parse;
	// keys are stringified for path purposes during the walk.
	var doc map[any]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parsing yaml: %w", err)
	}
	var unknown []string
	walkKnown(doc, "", "", known, &unknown)
	sort.Strings(unknown)
	return unknown, nil
}

// reservedTopLevelKeys are ignored during validation regardless of schema.
// "import" is a common YAML convention for file includes/merges.
var reservedTopLevelKeys = map[string]bool{
	"import": true,
}

// walkKnown validates obj against known using schemaPath for lookups and
// displayPath for error reporting. The two paths diverge when a sequence is
// encountered: the schema path stays as "categories" (the registered key)
// while the display path becomes "categories[0]", "categories[1]", etc.
func walkKnown(obj map[any]any, schemaPath, displayPath string, known map[string]map[string]bool, unknown *[]string) {
	allowed, validated := known[schemaPath]
	if !validated {
		return
	}
	for rawKey, val := range obj {
		key := fmt.Sprint(rawKey)
		if schemaPath == "" && reservedTopLevelKeys[key] {
			continue
		}
		var schemaKey, displayKey string
		if schemaPath == "" {
			schemaKey = key
			displayKey = key
		} else {
			schemaKey = schemaPath + "." + key
			displayKey = displayPath + "." + key
		}
		if !allowed[key] {
			*unknown = append(*unknown, displayKey)
			continue
		}
		if nested, ok := asMap(val); ok {
			walkKnown(nested, schemaKey, displayKey, known, unknown)
		} else if items, ok := val.([]any); ok {
			for i, item := range items {
				if nested, ok := asMap(item); ok {
					walkKnown(nested, schemaKey, fmt.Sprintf("%s[%d]", displayKey, i), known, unknown)
				}
			}
		}
	}
}

// asMap normalises a decoded YAML mapping to map[any]any. yaml.v3 decodes a
// mapping into map[string]any when every key is a string and falls back to
// map[any]any otherwise; both must keep being walked so string-keyed siblings
// of non-string keys are still validated.
func asMap(v any) (map[any]any, bool) {
	switch m := v.(type) {
	case map[any]any:
		return m, true
	case map[string]any:
		out := make(map[any]any, len(m))
		for k, val := range m {
			out[k] = val
		}
		return out, true
	}
	return nil, false
}
