package viewer

import (
	"fmt"
	"testing"
)

var testFields = []string{"name", "image", "build"}
var testPresets = map[string][]string{
	"name":  {"base"},
	"image": {"base", "golang"},
	"build": {"base", "multi-stage-dev"},
}

// stubSource is a minimal presets.Source used by NewModel tests.
type stubSource struct {
	fields  []string
	presets map[string][]string
}

func (s stubSource) ListFields() []string          { return s.fields }
func (s stubSource) ListPresets(f string) []string { return s.presets[f] }
func (s stubSource) PresetYAML(f, n string) (string, error) {
	for _, p := range s.presets[f] {
		if p == n {
			return fmt.Sprintf("# %s/%s\n", f, n), nil
		}
	}
	return "", fmt.Errorf("preset %q for field %q not found", n, f)
}

func TestListInitialStateShowsFields(t *testing.T) {
	l := newListModel(testFields, testPresets)
	f, p := l.Selected()
	if f != "name" || p != "base" {
		t.Errorf("initial selection = (%q, %q), want (\"name\", \"base\")", f, p)
	}
	if l.Mode() != modeFields {
		t.Error("initial mode should be modeFields")
	}
}

func TestListDownMovesFieldCursor(t *testing.T) {
	l := newListModel(testFields, testPresets)
	l.MoveDown()
	f, p := l.Selected()
	if f != "image" || p != "base" {
		t.Errorf("after Down = (%q, %q), want (\"image\", \"base\")", f, p)
	}
	if l.Mode() != modeFields {
		t.Error("should still be in modeFields after MoveDown")
	}
}

func TestListUpMovesFieldCursorBack(t *testing.T) {
	l := newListModel(testFields, testPresets)
	l.MoveDown() // image
	l.MoveDown() // build
	l.MoveUp()   // image
	f, _ := l.Selected()
	if f != "image" {
		t.Errorf("after 2 Down + Up = %q, want \"image\"", f)
	}
}

func TestDrillInSwitchesToPresets(t *testing.T) {
	l := newListModel(testFields, testPresets)
	l.MoveDown() // image
	l.DrillIn()
	if l.Mode() != modePresets {
		t.Error("after DrillIn mode should be modePresets")
	}
	f, p := l.Selected()
	if f != "image" || p != "base" {
		t.Errorf("after DrillIn = (%q, %q), want (\"image\", \"base\")", f, p)
	}
}

func TestMoveDownInsidePresets(t *testing.T) {
	l := newListModel(testFields, testPresets)
	l.MoveDown() // image
	l.DrillIn()
	l.MoveDown() // golang
	f, p := l.Selected()
	if f != "image" || p != "golang" {
		t.Errorf("after DrillIn + Down = (%q, %q), want (\"image\", \"golang\")", f, p)
	}
}

func TestBackReturnsToFields(t *testing.T) {
	l := newListModel(testFields, testPresets)
	l.MoveDown() // image
	l.DrillIn()
	l.MoveDown() // golang
	l.Back()
	if l.Mode() != modeFields {
		t.Error("after Back mode should be modeFields")
	}
	f, _ := l.Selected()
	if f != "image" {
		t.Errorf("after Back field = %q, want \"image\"", f)
	}
}

func TestModelEmptyStateRendersMessage(t *testing.T) {
	m := NewModel(stubSource{})
	_ = m.View()
}
