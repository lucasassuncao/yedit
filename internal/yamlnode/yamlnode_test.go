package yamlnode

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func mustRoot(t *testing.T, raw string) *yaml.Node {
	t.Helper()
	root, ok := RootMapping([]byte(raw))
	if !ok {
		t.Fatalf("RootMapping failed for %q", raw)
	}
	return root
}

func TestRootMapping(t *testing.T) {
	t.Run("valid document", func(t *testing.T) {
		root, ok := RootMapping([]byte("a: 1\nb: 2\n"))
		if !ok || root == nil || root.Kind != yaml.MappingNode {
			t.Fatalf("expected mapping root, got ok=%v root=%v", ok, root)
		}
	})
	t.Run("empty document yields empty mapping", func(t *testing.T) {
		root, ok := RootMapping(nil)
		if !ok || root == nil || root.Kind != yaml.MappingNode || len(root.Content) != 0 {
			t.Fatalf("expected empty mapping, got ok=%v root=%v", ok, root)
		}
	})
	t.Run("invalid YAML yields ok=false", func(t *testing.T) {
		if root, ok := RootMapping([]byte("a: [unclosed\n  b: }{")); ok {
			t.Fatalf("expected ok=false, got root=%v", root)
		}
	})
}

func TestChildByKey(t *testing.T) {
	root := mustRoot(t, "a: 1\nb:\n  c: 2\n")

	if n := ChildByKey(root, "a"); n == nil || n.Value != "1" {
		t.Errorf("ChildByKey(a) = %v, want scalar 1", n)
	}
	if n := ChildByKey(root, "missing"); n != nil {
		t.Errorf("ChildByKey(missing) = %v, want nil", n)
	}
	if n := ChildByKey(nil, "a"); n != nil {
		t.Errorf("ChildByKey(nil) = %v, want nil", n)
	}
	scalar := ChildByKey(root, "a")
	if n := ChildByKey(scalar, "a"); n != nil {
		t.Errorf("ChildByKey on a scalar = %v, want nil", n)
	}
}

func TestNodeAtPathAndScalarAt(t *testing.T) {
	root := mustRoot(t, "server:\n  tls:\n    cert: /etc/cert.pem\n")

	if n := NodeAtPath(root, []string{"server", "tls"}); n == nil || n.Kind != yaml.MappingNode {
		t.Errorf("NodeAtPath(server.tls) = %v, want mapping", n)
	}
	if n := NodeAtPath(root, []string{"server", "nope"}); n != nil {
		t.Errorf("NodeAtPath(server.nope) = %v, want nil", n)
	}
	if got := ScalarAt(root, []string{"server", "tls", "cert"}); got != "/etc/cert.pem" {
		t.Errorf("ScalarAt = %q, want /etc/cert.pem", got)
	}
	if got := ScalarAt(root, []string{"server", "tls"}); got != "" {
		t.Errorf("ScalarAt on a mapping = %q, want empty", got)
	}
	if got := ScalarAt(root, []string{"absent"}); got != "" {
		t.Errorf("ScalarAt on absent path = %q, want empty", got)
	}
}

func TestScalarChild(t *testing.T) {
	root := mustRoot(t, "name: web\nnested:\n  x: 1\n")

	if got := ScalarChild(root, "name"); got != "web" {
		t.Errorf("ScalarChild(name) = %q, want web", got)
	}
	if got := ScalarChild(root, "nested"); got != "" {
		t.Errorf("ScalarChild(nested mapping) = %q, want empty", got)
	}
	if got := ScalarChild(root, "missing"); got != "" {
		t.Errorf("ScalarChild(missing) = %q, want empty", got)
	}
}

func TestPresentNonEmpty(t *testing.T) {
	root := mustRoot(t, "filled: yes\nempty:\nlist: []\nmap: {}\n")

	if !PresentNonEmpty(ChildByKey(root, "filled")) {
		t.Error("filled scalar should be present")
	}
	if PresentNonEmpty(ChildByKey(root, "empty")) {
		t.Error("empty scalar should not be present")
	}
	if !PresentNonEmpty(ChildByKey(root, "list")) {
		t.Error("empty sequence should count as present")
	}
	if !PresentNonEmpty(ChildByKey(root, "map")) {
		t.Error("empty mapping should count as present")
	}
	if PresentNonEmpty(nil) {
		t.Error("nil node should not be present")
	}
}

func TestJoinPath(t *testing.T) {
	if got := JoinPath("", "key"); got != "key" {
		t.Errorf("JoinPath(empty) = %q, want key", got)
	}
	if got := JoinPath("a.b", "c"); got != "a.b.c" {
		t.Errorf("JoinPath = %q, want a.b.c", got)
	}
}

func TestCloneNode(t *testing.T) {
	if CloneNode(nil) != nil {
		t.Error("CloneNode(nil) should be nil")
	}

	root := mustRoot(t, "a:\n  b: original\n")
	clone := CloneNode(root)

	// Mutating the clone must not affect the original.
	ChildByKey(ChildByKey(clone, "a"), "b").Value = "changed"
	if got := ScalarAt(root, []string{"a", "b"}); got != "original" {
		t.Errorf("original mutated through clone: %q", got)
	}
	if got := ScalarAt(clone, []string{"a", "b"}); got != "changed" {
		t.Errorf("clone not mutated: %q", got)
	}
}

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
		// "source" is not a direct child of categories — every value is searched.
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
}
