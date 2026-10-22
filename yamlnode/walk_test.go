package yamlnode

import (
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestNavigate(t *testing.T) {
	t.Run("plain path", func(t *testing.T) {
		root := mustRoot(t, "server:\n  tls:\n    cert: x\n")
		var paths []string
		Navigate(root, []string{"server", "tls"}, "", func(_ *yaml.Node, p string) {
			paths = append(paths, p)
		})
		if len(paths) != 1 || paths[0] != "server.tls" {
			t.Errorf("Navigate paths = %v, want [server.tls]", paths)
		}
	})

	t.Run("sequence expansion", func(t *testing.T) {
		root := mustRoot(t, "workers:\n  - name: a\n  - name: b\n")
		var paths []string
		// Arrival expands the sequence: one call per item, not one for the sequence.
		Navigate(root, []string{"workers"}, "", func(_ *yaml.Node, p string) {
			paths = append(paths, p)
		})
		if len(paths) != 2 || paths[0] != "workers[0]" || paths[1] != "workers[1]" {
			t.Errorf("Navigate sequence paths = %v, want [workers[0] workers[1]]", paths)
		}
	})

	t.Run("dict-of-structs fallback", func(t *testing.T) {
		root := mustRoot(t, "categories:\n  docs:\n    source: a\n  media:\n    source: b\n")
		var paths []string
		// "source" is not a direct child of categories - every value is searched.
		Navigate(root, []string{"categories", "source"}, "", func(_ *yaml.Node, p string) {
			paths = append(paths, p)
		})
		want := []string{"categories.docs.source", "categories.media.source"}
		if len(paths) != 2 || paths[0] != want[0] || paths[1] != want[1] {
			t.Errorf("Navigate dict fallback paths = %v, want %v", paths, want)
		}
	})

	t.Run("absent path produces no calls", func(t *testing.T) {
		root := mustRoot(t, "a: 1\n")
		called := false
		Navigate(root, []string{"nope", "deeper"}, "", func(_ *yaml.Node, _ string) { called = true })
		if called {
			t.Error("Navigate should not arrive anywhere for an absent path")
		}
	})

	t.Run("nil root is absent", func(t *testing.T) {
		called := false
		Navigate(nil, []string{"a"}, "", func(_ *yaml.Node, _ string) { called = true })
		if called {
			t.Error("Navigate on a nil root must produce no calls, not panic")
		}
	})

	t.Run("no document-wide search for a missing top-level key", func(t *testing.T) {
		root := mustRoot(t, "outer:\n  source: a\n")
		called := false
		// "source" exists only nested; the root segment must not fall back.
		Navigate(root, []string{"source"}, "", func(_ *yaml.Node, _ string) { called = true })
		if called {
			t.Error("Navigate must not search the whole document for a missing root key")
		}
	})
}

func TestForEachLeaf(t *testing.T) {
	t.Run("plain leaf", func(t *testing.T) {
		root := mustRoot(t, "server:\n  port: 8080\n")
		var got []string
		ForEachLeaf(root, "server.port", func(n *yaml.Node, where string) {
			got = append(got, where+"="+n.Value)
		})
		if len(got) != 1 || got[0] != "server.port=8080" {
			t.Errorf("ForEachLeaf = %v, want [server.port=8080]", got)
		}
	})

	t.Run("sequence expansion", func(t *testing.T) {
		root := mustRoot(t, "workers:\n  - name: a\n  - name: b\n")
		var got []string
		ForEachLeaf(root, "workers.name", func(n *yaml.Node, where string) {
			got = append(got, where+"="+n.Value)
		})
		want := []string{"workers[0].name=a", "workers[1].name=b"}
		if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("ForEachLeaf sequence = %v, want %v", got, want)
		}
	})

	t.Run("dict-of-structs fallback after first match", func(t *testing.T) {
		root := mustRoot(t, "categories:\n  docs:\n    source: a\n  media:\n    source: b\n")
		var got []string
		ForEachLeaf(root, "categories.source", func(n *yaml.Node, where string) {
			got = append(got, where+"="+n.Value)
		})
		want := []string{"categories.docs.source=a", "categories.media.source=b"}
		if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("ForEachLeaf dict fallback = %v, want %v", got, want)
		}
	})

	t.Run("no document-wide search for a missing top-level key", func(t *testing.T) {
		root := mustRoot(t, "outer:\n  source: a\n")
		called := false
		// "source" exists only nested; the root segment must not fall back.
		ForEachLeaf(root, "source", func(_ *yaml.Node, _ string) { called = true })
		if called {
			t.Error("ForEachLeaf must not search the whole document for a missing root key")
		}
	})

	t.Run("absent path produces no calls", func(t *testing.T) {
		root := mustRoot(t, "a:\n  b: 1\n")
		called := false
		ForEachLeaf(root, "a.nope", func(_ *yaml.Node, _ string) { called = true })
		if called {
			t.Error("ForEachLeaf should not call fn for an absent leaf")
		}
	})

	t.Run("nil root is absent", func(t *testing.T) {
		called := false
		ForEachLeaf(nil, "a.b", func(_ *yaml.Node, _ string) { called = true })
		if called {
			t.Error("ForEachLeaf on a nil root must produce no calls, not panic")
		}
	})
}
