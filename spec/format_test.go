package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.yaml.in/yaml/v3"
)

func TestFormatCustomRegistersByName(t *testing.T) {
	f := FormatCustom("test-upper", func(v string) bool { return v == "UP" })

	got, ok := FormatByName("test-upper")
	require.True(t, ok, "an app's own format resolves by name")
	assert.Equal(t, "test-upper", got.Label())
	assert.True(t, got.Matches("UP"))
	assert.False(t, f.Matches("up"))

	_, ok = FormatByName("no-such-format")
	assert.False(t, ok)
}

func TestFormatZeroValue(t *testing.T) {
	var f Format
	assert.True(t, f.IsZero())
	assert.False(t, f.Matches("anything"), "a zero format matches nothing")
	assert.False(t, FormatURL.IsZero())
}

func TestFormatYAMLRoundTrip(t *testing.T) {
	var holder struct {
		F Format `yaml:"f"`
	}
	require.NoError(t, yaml.Unmarshal([]byte("f: uuid\n"), &holder))
	assert.Equal(t, "uuid", holder.F.Label())
	assert.True(t, holder.F.Matches("123e4567-e89b-12d3-a456-426614174000"))

	out, err := yaml.Marshal(holder)
	require.NoError(t, err)
	assert.Equal(t, "f: uuid\n", string(out))

	err = yaml.Unmarshal([]byte("f: nope\n"), &holder)
	assert.ErrorContains(t, err, `unknown format "nope"`, "an unregistered name would enforce nothing")
}

func TestBuiltInFormats(t *testing.T) {
	cases := []struct {
		f         Format
		good, bad string
	}{
		{FormatURL, "https://example.com/x", "ftp://example.com"},
		{FormatUUID, "123E4567-E89B-12D3-A456-426614174000", "123e4567"},
		{FormatSemver, "1.2.3-rc.1+build.5", "1.2"},
		{FormatDate, "2026-02-28", "2026-13-01"},
		{FormatEmail, "a@b.io", "a@b"},
		{FormatIPv4, "10.0.0.1", "::1"},
		{FormatIPv6, "::1", "10.0.0.1"},
		{FormatIP, "::1", "300.0.0.1"},
		{FormatHost, "api.example.com", "-bad-.com"},
		{FormatHostPort, "localhost:8080", "localhost:0"},
		{FormatPort, "65535", "65536"},
		{FormatCIDR, "10.0.0.0/8", "10.0.0.0"},
		{FormatFQDN, "api.example.com", "localhost"},
		{FormatDuration, "1h30m", "90"},
		{FormatGitRef, "release/v1.2", "bad ref"},
		{FormatDirectoryPath, "/var/data", ""},
		{FormatPublicKey, "-----BEGIN PUBLIC KEY-----\nabc", "abc"},
		{FormatPrivateKey, "-----BEGIN RSA PRIVATE KEY-----\nabc", "-----BEGIN PUBLIC KEY-----"},
		{FormatTerraformSource, "git::https://example.com/mod.git", "./local"},
	}
	for _, tc := range cases {
		t.Run(tc.f.Label(), func(t *testing.T) {
			assert.True(t, tc.f.Matches(tc.good), "%q should match", tc.good)
			assert.False(t, tc.f.Matches(tc.bad), "%q should not match", tc.bad)
			byName, ok := FormatByName(tc.f.Label())
			require.True(t, ok, "built-ins register by name")
			assert.Equal(t, tc.f.Label(), byName.Label())
		})
	}
}
