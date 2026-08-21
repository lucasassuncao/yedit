package legend

import (
	"testing"

	"charm.land/bubbles/v2/key"
)

func TestSaveTail_ShortHelp(t *testing.T) {
	km := SaveTail{}
	bindings := km.ShortHelp()
	if len(bindings) == 0 {
		t.Fatal("SaveTail.ShortHelp() returned no bindings")
	}
	assertContainsHelp(t, bindings, "ctrl+s")
	assertContainsHelp(t, bindings, "esc")
}

func TestListExisting_IncludesHintBinding(t *testing.T) {
	hint := key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "hint"))
	km := ListExisting{Hint: hint}
	bindings := km.ShortHelp()
	assertContainsHelp(t, bindings, "h")
	assertContainsHelp(t, bindings, "ctrl+s")
	assertContainsHelp(t, bindings, "enter")
}

func TestListExisting_DisabledHintNotIncluded(t *testing.T) {
	hint := key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "hint"))
	hint.SetEnabled(false)
	km := ListExisting{Hint: hint}
	bindings := km.ShortHelp()
	for _, b := range bindings {
		if !b.Enabled() {
			continue
		}
		if b.Help().Key == "h" {
			t.Error("disabled hint binding should not be enabled in ShortHelp")
		}
	}
}

func TestListNew_ShortHelp(t *testing.T) {
	hint := key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "hint"))
	km := ListNew{Hint: hint}
	bindings := km.ShortHelp()
	assertContainsHelp(t, bindings, "enter")
	assertContainsHelp(t, bindings, "ctrl+s")
}

func TestListFiltering_ShortHelp(t *testing.T) {
	km := ListFiltering{}
	bindings := km.ShortHelp()
	assertContainsHelp(t, bindings, "esc")
	assertContainsHelp(t, bindings, "enter")
}

func TestPresetMaps_ShortHelp(t *testing.T) {
	scalar := PresetListScalar{}
	if b := scalar.ShortHelp(); len(b) == 0 {
		t.Error("PresetListScalar returned no bindings")
	}
	collection := PresetListCollection{}
	cb := collection.ShortHelp()
	assertContainsHelp(t, cb, "a")

	preview := PresetPreview{}
	if b := preview.ShortHelp(); len(b) == 0 {
		t.Error("PresetPreview returned no bindings")
	}
}

func TestDynamic_ShortHelp(t *testing.T) {
	bindings := []key.Binding{
		key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "foo")),
		key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "bar")),
	}
	km := Dynamic(bindings)
	got := km.ShortHelp()
	if len(got) != 2 {
		t.Errorf("expected 2 bindings, got %d", len(got))
	}
}

func TestAllKeyMaps_FullHelpNil(t *testing.T) {
	maps := []interface{ FullHelp() [][]key.Binding }{
		SaveTail{},
		ListPreview{},
		ListFiltering{},
		ListExisting{},
		ListNew{},
		PresetPreview{},
		PresetListScalar{},
		PresetListCollection{},
		Dynamic(nil),
	}
	for _, km := range maps {
		if km.FullHelp() != nil {
			t.Errorf("%T.FullHelp() should return nil (short mode only)", km)
		}
	}
}

// assertContainsHelp checks that at least one binding in bs has the given display key.
func assertContainsHelp(t *testing.T, bs []key.Binding, wantKey string) {
	t.Helper()
	for _, b := range bs {
		if b.Help().Key == wantKey {
			return
		}
	}
	t.Errorf("no binding with key %q found in ShortHelp", wantKey)
}
