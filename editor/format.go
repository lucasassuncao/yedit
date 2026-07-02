package editor

import (
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Format describes the expected string format for a field.
// Use built-in vars (FormatURL, FormatUUID, ...) or FormatCustom for
// app-specific formats.
type Format struct {
	name     string
	validate func(string) bool
}

// FormatCustom creates an app-specific format with a display name and validator.
//
//	var FormatLeanIXID = editor.FormatCustom("leanix-id", func(v string) bool {
//	    ok, _ := regexp.MatchString(`^(TEAM|CMP|APP)-[A-Z0-9]+$`, v)
//	    return ok
//	})
func FormatCustom(name string, validate func(string) bool) Format {
	return Format{name: name, validate: validate}
}

// IsZero reports whether f is the zero value (not a real format).
func (f Format) IsZero() bool { return f.name == "" }

// Label returns the display name used in the hint panel and docgenerator.
func (f Format) Label() string { return f.name }

// ─── Built-in formats ─────────────────────────────────────────────────────────

var FormatURL = FormatCustom("url", func(v string) bool {
	u, err := url.Parse(v)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
})

var reUUID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

var FormatUUID = FormatCustom("uuid", func(v string) bool {
	return reUUID.MatchString(v)
})

var reSemver = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)` +
	`(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?` +
	`(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

var FormatSemver = FormatCustom("semver", func(v string) bool {
	return reSemver.MatchString(v)
})

var reDate = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])-(0[1-9]|[12]\d|3[01])$`)

var FormatDate = FormatCustom("date", func(v string) bool {
	return reDate.MatchString(v)
})

var reEmail = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

var FormatEmail = FormatCustom("email", func(v string) bool {
	return reEmail.MatchString(v)
})

var FormatIPv4 = FormatCustom("ipv4", func(v string) bool {
	ip := net.ParseIP(v)
	return ip != nil && ip.To4() != nil
})

var FormatIPv6 = FormatCustom("ipv6", func(v string) bool {
	ip := net.ParseIP(v)
	return ip != nil && ip.To4() == nil
})

var FormatIP = FormatCustom("ip", func(v string) bool {
	return net.ParseIP(v) != nil
})

var reHostname = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*$`)

var FormatHost = FormatCustom("host", func(v string) bool {
	if net.ParseIP(v) != nil {
		return true
	}
	return reHostname.MatchString(v)
})

var FormatHostPort = FormatCustom("host:port", func(v string) bool {
	_, p, err := net.SplitHostPort(v)
	if err != nil {
		return false
	}
	port, err := strconv.Atoi(p)
	return err == nil && port >= 1 && port <= 65535
})

var FormatPort = FormatCustom("port", func(v string) bool {
	n, err := strconv.Atoi(v)
	return err == nil && n >= 1 && n <= 65535
})

var FormatCIDR = FormatCustom("cidr", func(v string) bool {
	_, _, err := net.ParseCIDR(v)
	return err == nil
})

var reFQDN = regexp.MustCompile(`^(?:[a-zA-Z0-9](?:[a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

var FormatFQDN = FormatCustom("fqdn", func(v string) bool {
	return reFQDN.MatchString(v)
})

// FormatDuration accepts any time.ParseDuration value ("1h30m", "1.5h", "0"),
// the same parser used by the range validators (ValueInRange /
// RangeFromMetadata), so the two rules can never disagree on the same field.
var FormatDuration = FormatCustom("duration", func(v string) bool {
	_, err := time.ParseDuration(v)
	return err == nil
})

var reGitRef = regexp.MustCompile(`^[a-zA-Z0-9_./-]{1,250}$`)

var FormatGitRef = FormatCustom("git-ref", func(v string) bool {
	return reGitRef.MatchString(v)
})

var FormatDirectoryPath = FormatCustom("directory", func(v string) bool {
	return v != "" && !strings.ContainsRune(v, 0)
})

var rePEMPublic = regexp.MustCompile(`(?s)-----BEGIN[^-]*PUBLIC KEY-----`)

var FormatPublicKey = FormatCustom("public-key", func(v string) bool {
	return rePEMPublic.MatchString(v)
})

var rePEMPrivate = regexp.MustCompile(`(?s)-----BEGIN[^-]*PRIVATE KEY-----`)

var FormatPrivateKey = FormatCustom("private-key", func(v string) bool {
	return rePEMPrivate.MatchString(v)
})

var reTerraformSource = regexp.MustCompile(`^(git::|https?://|registry\.terraform\.io/)`)

var FormatTerraformSource = FormatCustom("terraform-source", func(v string) bool {
	return reTerraformSource.MatchString(v)
})
