package editor

import (
	"testing"

	"charm.land/bubbles/v2/key"
)

func TestSaveTailMap_ShortHelp(t *testing.T) {
	km := saveTailMap{}
	bindings := km.ShortHelp()
	if len(bindings) == 0 {
		t.Fatal("saveTailMap.ShortHelp() returned no bindings")
	}
	assertContainsHelp(t, bindings, "ctrl+s")
	assertContainsHelp(t, bindings, "esc")
}

func TestListExistingMap_IncludesHintBinding(t *testing.T) {
	hint := key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "hint"))
	km := listExistingMap{hint: hint}
	bindings := km.ShortHelp()
	assertContainsHelp(t, bindings, "h")
	assertContainsHelp(t, bindings, "ctrl+s")
	assertContainsHelp(t, bindings, "enter")
}

func TestListExistingMap_DisabledHintNotIncluded(t *testing.T) {
	hint := key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "hint"))
	hint.SetEnabled(false)
	km := listExistingMap{hint: hint}
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

func TestListNewMap_ShortHelp(t *testing.T) {
	hint := key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "hint"))
	km := listNewMap{hint: hint}
	bindings := km.ShortHelp()
	assertContainsHelp(t, bindings, "enter")
	assertContainsHelp(t, bindings, "ctrl+s")
}

func TestListFilteringMap_ShortHelp(t *testing.T) {
	km := listFilteringMap{}
	bindings := km.ShortHelp()
	assertContainsHelp(t, bindings, "esc")
	assertContainsHelp(t, bindings, "enter")
}

func TestPresetMaps_ShortHelp(t *testing.T) {
	scalar := presetListScalarMap{}
	if b := scalar.ShortHelp(); len(b) == 0 {
		t.Error("presetListScalarMap returned no bindings")
	}
	collection := presetListCollectionMap{}
	cb := collection.ShortHelp()
	assertContainsHelp(t, cb, "a")

	preview := presetPreviewMap{}
	if b := preview.ShortHelp(); len(b) == 0 {
		t.Error("presetPreviewMap returned no bindings")
	}
}

func TestDynamicKeyMap_ShortHelp(t *testing.T) {
	bindings := []key.Binding{
		key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "foo")),
		key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "bar")),
	}
	km := dynamicKeyMap(bindings)
	got := km.ShortHelp()
	if len(got) != 2 {
		t.Errorf("expected 2 bindings, got %d", len(got))
	}
}

func TestAllKeyMaps_FullHelpNil(t *testing.T) {
	maps := []interface{ FullHelp() [][]key.Binding }{
		saveTailMap{},
		listPreviewMap{},
		listFilteringMap{},
		listExistingMap{},
		listNewMap{},
		presetPreviewMap{},
		presetListScalarMap{},
		presetListCollectionMap{},
		dynamicKeyMap(nil),
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
