package theme

import "testing"

// TestThemeRegistryHasNoDuplicateNames guards themeRegistry against a
// copy-paste mistake: since All() and Categories() are both built from this
// one slice, a name listed twice would silently overwrite one theme in All()
// and print twice in Categories() output, with no compiler error either way.
func TestThemeRegistryHasNoDuplicateNames(t *testing.T) {
	seen := make(map[string]string) // theme name -> category that claimed it

	for _, cat := range themeRegistry {
		for _, nt := range cat.themes {
			if prev, dup := seen[nt.name]; dup {
				t.Errorf("themeRegistry: %q appears in both %q and %q", nt.name, prev, cat.category)
			}
			seen[nt.name] = cat.category
		}
	}
}
