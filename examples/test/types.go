package main

import "github.com/lucasassuncao/yedit/editor"

var testPresets = testPresetSource{}
var testHints = buildMetadataSource(map[string]*metadataNode{
	"server": {
		FieldMeta: editor.FieldMeta{
			Description: "HTTP server configuration.",
			Type:        "object",
			Required:    true,
		},
		Children: map[string]*metadataNode{
			"host": {FieldMeta: editor.FieldMeta{
				Description: "Address the server binds to.",
				Type:        "string",
				Default:     "localhost",
				Example:     "host: 0.0.0.0",
				Snippet:     "  host: localhost\n",
				PreChecked:  true,
			}},
			"port": {FieldMeta: editor.FieldMeta{
				Description: "TCP port to listen on.",
				Type:        "int",
				Default:     "8080",
				Example:     "port: 8080",
				Snippet:     "  port: 8080\n",
				PreChecked:  true,
			}},
			"tls": {FieldMeta: editor.FieldMeta{
				Description: "Enable TLS (HTTPS). Requires a certificate and key.",
				Type:        "bool",
				Default:     "false",
				Example:     "tls: true",
				Snippet:     "  tls: false\n",
			}},
			"allowed-ips": {FieldMeta: editor.FieldMeta{
				Description: "CIDR ranges or IPs allowed to connect. Empty means allow all.",
				Type:        "[]string",
				Example:     "allowed-ips:\n  - 127.0.0.1\n  - 192.168.0.0/24\n  - 10.0.0.0/8",
				Snippet:     "  allowed-ips:\n    - 127.0.0.1\n",
			}},
			"headers": {FieldMeta: editor.FieldMeta{
				Description: "HTTP response headers added to every reply.",
				Type:        "map[string]string",
				Example:     "headers:\n  X-Request-ID: \"\"\n  X-Forwarded-For: \"\"",
				Snippet:     "  headers:\n    X-Request-ID: \"\"\n",
			}},
		},
	},
	"database": {
		FieldMeta: editor.FieldMeta{
			Description: "Database connection configuration.",
			Type:        "object",
		},
		Children: map[string]*metadataNode{
			"driver": {FieldMeta: editor.FieldMeta{
				Type:    "string",
				OneOf:   []string{"postgres", "mysql", "sqlite"},
				Snippet: "  driver: postgres\n",
			}},
			"dsn": {FieldMeta: editor.FieldMeta{
				Type:    "string",
				Snippet: "  dsn: \"postgres://localhost/mydb\"\n",
			}},
			"max-conns": {FieldMeta: editor.FieldMeta{
				Type:    "int",
				Snippet: "  max-conns: 10\n",
			}},
			"pool": {FieldMeta: editor.FieldMeta{
				Type:    "object",
				Snippet: "  pool:\n    min-size: 2\n    max-size: 10\n    timeout: 30\n",
			}},
		},
	},
	"logging": {
		FieldMeta: editor.FieldMeta{
			Description: "Application logging configuration.",
			Type:        "object",
		},
		Children: map[string]*metadataNode{
			"level": {FieldMeta: editor.FieldMeta{
				Description: "Minimum severity to emit. Lower levels produce more output.",
				Type:        "string",
				Required:    true,
				OneOf:       []string{"debug", "info", "warn", "error"},
				Default:     "info",
				Example:     "level: info",
				Snippet:     "  level: info\n",
				PreChecked:  true,
			}},
			"file": {FieldMeta: editor.FieldMeta{
				Description: "Path to the log file. Supports ~ for the home directory.",
				Type:        "string",
				Example:     "file: /var/log/app.log",
				Snippet:     "  file: \"/var/log/app.log\"\n",
			}},
			"show-caller": {FieldMeta: editor.FieldMeta{
				Description: "Append source file and line number to each log entry.",
				Type:        "bool",
				Default:     "false",
				Example:     "show-caller: false",
				Snippet:     "  show-caller: false\n",
			}},
		},
	},
	"deploy": {
		FieldMeta: editor.FieldMeta{
			Description: "Deployment configuration.",
			Type:        "object",
		},
		Children: map[string]*metadataNode{
			"strategy": {FieldMeta: editor.FieldMeta{
				Type:       "string",
				OneOf:      []string{"rolling", "blue-green", "canary"},
				Snippet:    "  strategy: rolling\n",
				PreChecked: true,
			}},
			"replicas": {FieldMeta: editor.FieldMeta{
				Type:       "int",
				Snippet:    "  replicas: 1\n",
				PreChecked: true,
			}},
			"enabled": {FieldMeta: editor.FieldMeta{
				Type:    "bool",
				Snippet: "  enabled: true\n",
			}},
		},
	},

	// Pattern 6: new FieldMeta capabilities

	"network": {
		FieldMeta: editor.FieldMeta{
			Description: "Network configuration (exercises Formats, MinLength/MaxLength, NotOneOf).",
			Type:        "object",
		},
		Children: map[string]*metadataNode{
			"endpoint": {FieldMeta: editor.FieldMeta{
				Type:        "string",
				Description: "HTTP/HTTPS endpoint URL.",
				Formats:     []editor.Format{editor.FormatURL},
				Required:    true,
			}},
			"host": {FieldMeta: editor.FieldMeta{
				Type:        "string",
				Description: "Hostname or IPv4 address (OR semantics: matches either).",
				Formats:     []editor.Format{editor.FormatHost, editor.FormatIPv4},
			}},
			"cidr": {FieldMeta: editor.FieldMeta{
				Type:        "string",
				Description: "CIDR block (e.g. 10.0.0.0/8).",
				Formats:     []editor.Format{editor.FormatCIDR},
			}},
			"uuid": {FieldMeta: editor.FieldMeta{
				Type:    "string",
				Formats: []editor.Format{editor.FormatUUID},
			}},
			"tag": {FieldMeta: editor.FieldMeta{
				Type:      "string",
				Formats:   []editor.Format{editor.FormatSemver},
				MinLength: 5,
			}},
			"protocol": {FieldMeta: editor.FieldMeta{
				Type:     "string",
				OneOf:    []string{"http", "https", "grpc", "ws"},
				NotOneOf: []string{"ftp", "telnet"},
			}},
			"note-name": {FieldMeta: editor.FieldMeta{
				Type:      "string",
				MinLength: 3,
				MaxLength: 64,
			}},
			"any-ip": {FieldMeta: editor.FieldMeta{
				Type:        "string",
				Description: "IPv4 or IPv6 address.",
				Formats:     []editor.Format{editor.FormatIP},
			}},
			"listen": {FieldMeta: editor.FieldMeta{
				Type:    "string",
				Formats: []editor.Format{editor.FormatHostPort},
				Example: "listen: \"0.0.0.0:8080\"",
			}},
			"http-port": {FieldMeta: editor.FieldMeta{
				Type:    "string",
				Formats: []editor.Format{editor.FormatPort},
			}},
			"timeout": {FieldMeta: editor.FieldMeta{
				Type:    "string",
				Formats: []editor.Format{editor.FormatDuration},
				Example: "timeout: 30s",
			}},
			"expiry": {FieldMeta: editor.FieldMeta{
				Type:    "string",
				Formats: []editor.Format{editor.FormatDate},
				Example: "expiry: 2026-12-31",
			}},
			"leanix-id": {FieldMeta: editor.FieldMeta{
				Type:        "string",
				Description: "LeanIX identifier (FormatCustom example).",
				Formats:     []editor.Format{formatLeanIXID},
				Example:     "leanix-id: TEAM-ABC123",
			}},
		},
	},

	"security": {
		FieldMeta: editor.FieldMeta{
			Description: "Security configuration (exercises remaining built-in formats).",
			Type:        "object",
		},
		Children: map[string]*metadataNode{
			"ipv6-addr": {FieldMeta: editor.FieldMeta{
				Type:    "string",
				Formats: []editor.Format{editor.FormatIPv6},
				Example: "ipv6-addr: \"::1\"",
			}},
			"public-key": {FieldMeta: editor.FieldMeta{
				Type:      "string",
				Multiline: true,
				Formats:   []editor.Format{editor.FormatPublicKey},
				Example:   "public-key: |\n  -----BEGIN PUBLIC KEY-----\n  ...\n  -----END PUBLIC KEY-----\n",
			}},
			"private-key": {FieldMeta: editor.FieldMeta{
				Type:      "string",
				Multiline: true,
				Formats:   []editor.Format{editor.FormatPrivateKey},
				Example:   "private-key: |\n  -----BEGIN PRIVATE KEY-----\n  ...\n  -----END PRIVATE KEY-----\n",
			}},
			"fqdn": {FieldMeta: editor.FieldMeta{
				Type:    "string",
				Formats: []editor.Format{editor.FormatFQDN},
				Example: "fqdn: api.example.com",
			}},
			"email": {FieldMeta: editor.FieldMeta{
				Type:    "string",
				Formats: []editor.Format{editor.FormatEmail},
				Example: "email: admin@example.com",
			}},
		},
	},

	"deploy-ext": {
		FieldMeta: editor.FieldMeta{
			Description: "Extended deploy configuration (exercises Multiline, Snippet, PreChecked).",
			Type:        "object",
		},
		Children: map[string]*metadataNode{
			"source": {FieldMeta: editor.FieldMeta{
				Type:        "string",
				Description: "Terraform module source reference.",
				Formats:     []editor.Format{editor.FormatTerraformSource},
				Snippet:     "  source: git::https://github.com/company/module.git?ref=v1.0.0\n",
				PreChecked:  true,
			}},
			"script": {FieldMeta: editor.FieldMeta{
				Type:        "string",
				Description: "Deploy shell script (Multiline=true, no Example - auto-generation exercised).",
				Multiline:   true,
				PreChecked:  true,
				// No Example: auto-generates "script: |\n  line 1\n  line 2\n"
			}},
			"readme": {FieldMeta: editor.FieldMeta{
				Type:        "string",
				Description: "Deployment notes (Multiline=true with explicit Example).",
				Multiline:   true,
				Example:     "readme: |\n  # My Deploy\n  Step 1: do X\n  Step 2: do Y\n",
			}},
			"git-ref": {FieldMeta: editor.FieldMeta{
				Type:    "string",
				Formats: []editor.Format{editor.FormatGitRef},
				Example: "git-ref: v1.2.3",
			}},
			"dir-path": {FieldMeta: editor.FieldMeta{
				Type:        "string",
				Description: "Cross-platform directory path (no existence check).",
				Formats:     []editor.Format{editor.FormatDirectoryPath},
				Example:     "dir-path: /opt/deploy",
			}},
		},
	},
})
