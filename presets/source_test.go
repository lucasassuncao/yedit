package presets

import (
	"fmt"
	"strings"
	"testing"
)

type serverConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

// Presets land in the file as they are, so they use the 2-space indentation
// yamledit writes edits with, not yaml.Marshal's 4.
func TestForFieldIndentsWithTwoSpaces(t *testing.T) {
	src := ForField("servers", map[string][]serverConfig{
		"pair": {{Host: "a", Port: 1}, {Host: "b", Port: 2}},
	})

	out, err := src.PresetYAML("servers", "pair")
	if err != nil {
		t.Fatalf("PresetYAML(servers, pair) error = %v, want nil", err)
	}
	want := "servers:\n  - host: a\n    port: 1\n  - host: b\n    port: 2\n"
	if out != want {
		t.Errorf("PresetYAML(servers, pair) = %q, want %q", out, want)
	}
}

func TestCombineFuncFallback(t *testing.T) {
	src := Combine(
		ForField("server", map[string]serverConfig{
			"minimal": {Host: "localhost", Port: 8080},
		}),
		Func(func(field, name string) (string, error) {
			if field == "dynamic" && name == "generated" {
				return "dynamic:\n  ok: true\n", nil
			}
			return "", fmt.Errorf("presets: unknown field %q", field)
		}),
	)

	out, err := src.PresetYAML("dynamic", "generated")
	if err != nil {
		t.Fatalf("PresetYAML(dynamic, generated) error = %v, want nil", err)
	}
	if !strings.Contains(out, "ok: true") {
		t.Errorf("PresetYAML(dynamic, generated) = %q, want the Func answer", out)
	}
}

func TestCombineFirstSourceStillWins(t *testing.T) {
	src := Combine(
		ForField("server", map[string]serverConfig{
			"minimal": {Host: "localhost", Port: 8080},
		}),
		Func(func(field, name string) (string, error) {
			return "from-func: true\n", nil
		}),
	)

	out, err := src.PresetYAML("server", "minimal")
	if err != nil {
		t.Fatalf("PresetYAML(server, minimal) error = %v, want nil", err)
	}
	if !strings.Contains(out, "host: localhost") {
		t.Errorf("PresetYAML(server, minimal) = %q, want the ForField answer", out)
	}
}

func TestCombineUnknownPresetKeepsFieldError(t *testing.T) {
	src := Combine(
		ForField("server", map[string]serverConfig{
			"minimal": {Host: "localhost", Port: 8080},
		}),
	)

	_, err := src.PresetYAML("server", "missing")
	if err == nil {
		t.Fatal("PresetYAML(server, missing) error = nil, want unknown-preset error")
	}
	if !strings.Contains(err.Error(), "unknown preset") {
		t.Errorf("error = %q, want it to mention the unknown preset", err)
	}
}

func TestCombineUnknownFieldError(t *testing.T) {
	src := Combine(
		ForField("server", map[string]serverConfig{
			"minimal": {Host: "localhost", Port: 8080},
		}),
	)

	_, err := src.PresetYAML("nope", "minimal")
	if err == nil {
		t.Fatal("PresetYAML(nope, minimal) error = nil, want unknown-field error")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("error = %q, want it to mention the unknown field", err)
	}
}
