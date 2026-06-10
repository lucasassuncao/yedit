package main

import "github.com/lucasassuncao/yedit/editor"

var testPresets = testPresetSource{}
var testHints = buildHintSource(map[string]*hintNode{
	"server": {
		FieldMeta: editor.FieldMeta{
			Description: "HTTP server configuration.",
			Type:        "object",
			Required:    true,
		},
		Children: map[string]*hintNode{
			"host": {FieldMeta: editor.FieldMeta{
				Description: "Address the server binds to.",
				Type:        "string",
				Default:     "localhost",
				Example:     "host: 0.0.0.0",
			}},
			"port": {FieldMeta: editor.FieldMeta{
				Description: "TCP port to listen on.",
				Type:        "int",
				Default:     "8080",
				Example:     "port: 8080",
			}},
			"tls": {FieldMeta: editor.FieldMeta{
				Description: "Enable TLS (HTTPS). Requires a certificate and key.",
				Type:        "bool",
				Default:     "false",
				Example:     "tls: true",
			}},
			"allowed-ips": {FieldMeta: editor.FieldMeta{
				Description: "CIDR ranges or IPs allowed to connect. Empty means allow all.",
				Type:        "[]string",
				Example:     "allowed-ips:\n  - 127.0.0.1\n  - 192.168.0.0/24\n  - 10.0.0.0/8",
			}},
			"headers": {FieldMeta: editor.FieldMeta{
				Description: "HTTP response headers added to every reply.",
				Type:        "map[string]string",
				Example:     "headers:\n  X-Request-ID: \"\"\n  X-Forwarded-For: \"\"",
			}},
		},
	},
	"logging": {
		FieldMeta: editor.FieldMeta{
			Description: "Application logging configuration.",
			Type:        "object",
		},
		Children: map[string]*hintNode{
			"level": {FieldMeta: editor.FieldMeta{
				Description: "Minimum severity to emit. Lower levels produce more output.",
				Type:        "string",
				Required:    true,
				OneOf:       []string{"debug", "info", "warn", "error"},
				Default:     "info",
				Example:     "level: info",
			}},
			"file": {FieldMeta: editor.FieldMeta{
				Description: "Path to the log file. Supports ~ for the home directory.",
				Type:        "string",
				Example:     "file: /var/log/app.log",
			}},
			"show-caller": {FieldMeta: editor.FieldMeta{
				Description: "Append source file and line number to each log entry.",
				Type:        "bool",
				Default:     "false",
				Example:     "show-caller: false",
			}},
		},
	},
})

var testFieldSnippets = map[string]map[string]string{
	"server": {
		"host":        "  host: localhost\n",
		"port":        "  port: 8080\n",
		"tls":         "  tls: false\n",
		"allowed-ips": "  allowed-ips:\n    - 127.0.0.1\n",
		"headers":     "  headers:\n    X-Request-ID: \"\"\n",
	},
	"database": {
		"driver":    "  driver: postgres\n",
		"dsn":       "  dsn: \"postgres://localhost/mydb\"\n",
		"max-conns": "  max-conns: 10\n",
		"pool":      "  pool:\n    min-size: 2\n    max-size: 10\n    timeout: 30\n",
	},
	"logging": {
		"level":       "  level: info\n",
		"file":        "  file: \"/var/log/app.log\"\n",
		"show-caller": "  show-caller: false\n",
	},
	"deploy": {
		"strategy": "  strategy: rolling\n",
		"replicas": "  replicas: 1\n",
		"enabled":  "  enabled: true\n",
	},
}

var testPreCheckedFields = map[string][]string{
	"server":  {"host", "port"},
	"logging": {"level"},
	"deploy":  {"strategy", "replicas"},
}
