package render

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/lucasassuncao/yedit/spec"
)

var groupedViolations = []spec.Violation{
	{Group: spec.GroupUnknownKeys, Path: "server.bogus"},
	{Group: spec.GroupUnknownKeys, Path: "extra", Message: "not in schema"},
	{Group: spec.GroupMutuallyExclusive, Message: "image and build"},
	{Group: spec.GroupRules, Path: "server.id", Message: "bad"},
	{Group: spec.GroupRules, Path: "server.host", Message: "required"},
	{Group: spec.GroupRules, Path: "workers[0].name", Message: "required"},
	{Group: spec.GroupRules, Path: "version", Message: "required"},
}

func TestViolationsGroupsAndDrawsRulesAsATree(t *testing.T) {
	want := strings.Join([]string{
		"• Unknown Key(s):",
		"  - server.bogus",
		"  - extra: not in schema",
		"",
		"• Mutually Exclusive:",
		"  - image and build",
		"",
		"• Rules:",
		"  - server",
		"    ├ id    bad",
		"    └ host  required",
		"  - workers",
		"    └ [0].name  required",
		"  - version",
		"    └ required",
	}, "\n")
	assert.Equal(t, want, Violations(groupedViolations, 0))
}

func TestViolationsCapsTheLines(t *testing.T) {
	want := strings.Join([]string{
		"• Unknown Key(s):",
		"  - server.bogus",
		"  - extra: not in schema",
		"",
		"... and 11 more line(s).",
	}, "\n")
	assert.Equal(t, want, Violations(groupedViolations, 4))
}

func TestSplitRulesPath(t *testing.T) {
	cases := []struct{ path, sec, label string }{
		{"server.tls.cert", "server", "tls.cert"},
		{"workers[0].name", "workers", "[0].name"},
		{"server.ports[1]", "server", "ports[1]"},
		{"version", "version", ""},
	}
	for _, tc := range cases {
		sec, label := splitRulesPath(tc.path)
		assert.Equal(t, tc.sec, sec, tc.path)
		assert.Equal(t, tc.label, label, tc.path)
	}
}
