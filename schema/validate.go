package schema

import (
	"sort"

	"gopkg.in/yaml.v3"
)

// KnownChildren collapses a FieldDef tree into a map of dotted paths to the
// set of allowed direct children. Used by UnknownKeys to detect typos.
//
// A nil value at a path means "free-form" — children at that path are not
// validated (e.g. customizations.vscode.settings has no fixed schema).
func KnownChildren(fields []FieldDef) map[string]map[string]bool {
	out := make(map[string]map[string]bool)
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
// are not validated.
func UnknownKeys(raw []byte, known map[string]map[string]bool) []string {
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	var unknown []string
	walkKnown(doc, "", known, &unknown)
	sort.Strings(unknown)
	return unknown
}

// reservedTopLevelKeys are ignored during validation regardless of schema.
// "import" is a common YAML convention for file includes/merges.
var reservedTopLevelKeys = map[string]bool{
	"import": true,
}

func walkKnown(obj map[string]any, prefix string, known map[string]map[string]bool, unknown *[]string) {
	allowed, validated := known[prefix]
	if !validated {
		return
	}
	for key, val := range obj {
		if prefix == "" && reservedTopLevelKeys[key] {
			continue
		}
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		if !allowed[key] {
			*unknown = append(*unknown, path)
			continue
		}
		switch v := val.(type) {
		case map[string]any:
			walkKnown(v, path, known, unknown)
		case []any:
			for _, item := range v {
				if nested, ok := item.(map[string]any); ok {
					walkKnown(nested, path, known, unknown)
				}
			}
		}
	}
}
