package schema

import "strings"

// containsTagOption reports whether opt is present as a comma-separated entry
// in tag (e.g. checks for "required" in `validate:"required,omitempty"`).
func containsTagOption(tag, opt string) bool {
	for _, p := range strings.Split(tag, ",") {
		if strings.TrimSpace(p) == opt {
			return true
		}
	}
	return false
}

// extractValue returns the value of a prefix=value option in a comma-separated
// tag (e.g. extractValue(`required,default=Dockerfile`, "default=") → "Dockerfile").
func extractValue(tag, prefix string) string {
	for _, p := range strings.Split(tag, ",") {
		p = strings.TrimSpace(p)
		if rest, ok := strings.CutPrefix(p, prefix); ok {
			return rest
		}
	}
	return ""
}

// extractList returns the space-separated list following prefix in tag
// (e.g. extractList(`omitempty,oneof=a b c`, "oneof=") → ["a", "b", "c"]).
func extractList(tag, prefix string) []string {
	for _, p := range strings.Split(tag, ",") {
		p = strings.TrimSpace(p)
		if rest, ok := strings.CutPrefix(p, prefix); ok {
			return strings.Fields(rest)
		}
	}
	return nil
}
