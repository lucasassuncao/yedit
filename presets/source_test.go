package presets

import (
	"reflect"
	"testing"
	"testing/fstest"
)

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"build/base.yaml":               {Data: []byte("build:\n  dockerfile: Dockerfile\n")},
		"build/multi-stage.yml":         {Data: []byte("build:\n  target: final\n")},
		"customizations/vscode-go.yaml": {Data: []byte("customizations:\n  vscode: {}\n")},
		"build/notes.txt":               {Data: []byte("not a preset")},
		"README.md":                     {Data: []byte("not a field dir")},
	}
}

func TestFSSource_ListFields(t *testing.T) {
	s := FromFS(testFS(), ".")
	got := s.ListFields()
	want := []string{"build", "customizations"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListFields() = %v, want %v", got, want)
	}
}

func TestFSSource_ListFields_missingRoot(t *testing.T) {
	s := FromFS(testFS(), "no-such-dir")
	if got := s.ListFields(); got != nil {
		t.Errorf("ListFields() on missing root = %v, want nil", got)
	}
}

func TestFSSource_ListPresets(t *testing.T) {
	s := FromFS(testFS(), ".")

	got := s.ListPresets("build")
	want := []string{"base", "multi-stage"} // sorted; .txt files skipped
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListPresets(build) = %v, want %v", got, want)
	}

	if got := s.ListPresets("no-such-field"); got != nil {
		t.Errorf("ListPresets(no-such-field) = %v, want nil", got)
	}
}

func TestFSSource_PresetYAML(t *testing.T) {
	s := FromFS(testFS(), ".")

	got, err := s.PresetYAML("build", "base")
	if err != nil {
		t.Fatalf("PresetYAML(build, base): %v", err)
	}
	if want := "build:\n  dockerfile: Dockerfile\n"; got != want {
		t.Errorf("PresetYAML(build, base) = %q, want %q", got, want)
	}

	// .yml fallback when no .yaml file exists.
	got, err = s.PresetYAML("build", "multi-stage")
	if err != nil {
		t.Fatalf("PresetYAML(build, multi-stage): %v", err)
	}
	if want := "build:\n  target: final\n"; got != want {
		t.Errorf("PresetYAML(build, multi-stage) = %q, want %q", got, want)
	}

	if _, err := s.PresetYAML("build", "no-such-preset"); err == nil {
		t.Error("PresetYAML with unknown preset should return an error")
	}
	if _, err := s.PresetYAML("no-such-field", "base"); err == nil {
		t.Error("PresetYAML with unknown field should return an error")
	}
}

func TestFSSource_subdirectoryRoot(t *testing.T) {
	fsys := fstest.MapFS{
		"my-presets/build/base.yaml": {Data: []byte("build: {}\n")},
	}
	s := FromFS(fsys, "my-presets")

	if got, want := s.ListFields(), []string{"build"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ListFields() = %v, want %v", got, want)
	}
	if _, err := s.PresetYAML("build", "base"); err != nil {
		t.Errorf("PresetYAML(build, base): %v", err)
	}
}
