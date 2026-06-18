package main

import (
	"fmt"

	"github.com/lucasassuncao/yedit/editor"
	"github.com/lucasassuncao/yedit/metadata"
)

var testMetadata = mustBuildMetadata()

func mustBuildMetadata() editor.MetadataSource {
	src, err := metadata.NewFromTree(&TestConfig{}, TestConfig{}.Metadata())
	if err != nil {
		panic(fmt.Sprintf("testMetadata: %v", err))
	}
	return src
}

func (TestConfig) Metadata() map[string]*metadata.Node {
	return map[string]*metadata.Node{
		"app-name": {FieldMeta: editor.FieldMeta{
			Description: "Application display name.",
			Required:    true,
		}},
		"debug": {FieldMeta: editor.FieldMeta{
			Description: "Enable debug mode.",
			Default:     "false",
		}},
		"version": {FieldMeta: editor.FieldMeta{
			Description: "Application version (semver).",
			Required:    true,
			Formats:     []editor.Format{editor.FormatSemver},
		}},
		"port": {FieldMeta: editor.FieldMeta{
			Description: "Default listening port.",
			Default:     "8080",
		}},
		"ratio": {FieldMeta: editor.FieldMeta{
			Description: "Float64 ratio (0.0-1.0).",
		}},
		"build-timeout": {FieldMeta: editor.FieldMeta{
			Description: "Maximum build duration.",
			Formats:     []editor.Format{editor.FormatDuration},
			Example:     "build-timeout: 5m",
		}},
		"labels": {FieldMeta: editor.FieldMeta{
			Description: "Arbitrary key-value string labels.",
		}},
		"settings": {FieldMeta: editor.FieldMeta{
			Description: "Free-form settings map.",
		}},
		"tags": {FieldMeta: editor.FieldMeta{
			Description: "String tags (unique).",
			Unique:      true,
		}},
		"ports": {FieldMeta: editor.FieldMeta{
			Description: "Additional integer ports.",
		}},
		"timeout": {
			FieldMeta: editor.FieldMeta{
				Description: "Per-operation timeout configuration.",
			},
			Children: TimeoutValue{}.Metadata(),
		},
		"server": {
			FieldMeta: editor.FieldMeta{
				Description: "HTTP server configuration.",
				Required:    true,
			},
			Children: ServerConfig{}.Metadata(),
		},
		"database": {
			FieldMeta: editor.FieldMeta{
				Description: "Database connection configuration.",
			},
			Children: DatabaseConfig{}.Metadata(),
		},
		"logging": {
			FieldMeta: editor.FieldMeta{
				Description: "Application logging configuration.",
			},
			Children: LoggingConfig{}.Metadata(),
		},
		"deploy": {
			FieldMeta: editor.FieldMeta{
				Description: "Deployment configuration.",
			},
			Children: DeployConfig{}.Metadata(),
		},
		"network": {
			FieldMeta: editor.FieldMeta{
				Description: "Network configuration (exercises Formats, MinLength/MaxLength, NotOneOf).",
			},
			Children: NetworkConfig{}.Metadata(),
		},
		"security": {
			FieldMeta: editor.FieldMeta{
				Description: "Security configuration (exercises remaining built-in formats).",
			},
			Children: SecurityConfig{}.Metadata(),
		},
		"deploy-ext": {
			FieldMeta: editor.FieldMeta{
				Description: "Extended deploy configuration (exercises Multiline, Snippet, PreChecked).",
			},
			Children: DeployExtConfig{}.Metadata(),
		},
		"workers": {
			FieldMeta: editor.FieldMeta{
				Description: "Background worker definitions.",
			},
			Children: Worker{}.Metadata(),
		},
		"routes": {
			FieldMeta: editor.FieldMeta{
				Description: "HTTP route definitions.",
			},
			Children: Route{}.Metadata(),
		},
		"filters": {
			FieldMeta: editor.FieldMeta{
				Description: "File filters (recursive via any/all).",
			},
			Children: Filter{}.Metadata(),
		},
		"port-attrs": {
			FieldMeta: editor.FieldMeta{
				Description: "Per-port forwarding attributes (map[string]PortAttr).",
			},
			Children: PortAttr{}.Metadata(),
		},
		"edge-cases": {
			FieldMeta: editor.FieldMeta{
				Description: "Schema edge-case demonstrations.",
			},
			Children: SchemaEdgeCases{}.Metadata(),
		},
	}
}

func (TimeoutValue) Metadata() map[string]*metadata.Node {
	return map[string]*metadata.Node{
		"connect": {FieldMeta: editor.FieldMeta{
			Description: "Dial/connect timeout.",
			Formats:     []editor.Format{editor.FormatDuration},
			Example:     "connect: 5s",
		}},
		"read": {FieldMeta: editor.FieldMeta{
			Description: "Read timeout.",
			Formats:     []editor.Format{editor.FormatDuration},
			Example:     "read: 30s",
		}},
		"write": {FieldMeta: editor.FieldMeta{
			Description: "Write timeout.",
			Formats:     []editor.Format{editor.FormatDuration},
			Example:     "write: 30s",
		}},
	}
}

func (ServerConfig) Metadata() map[string]*metadata.Node {
	return map[string]*metadata.Node{
		"host": {FieldMeta: editor.FieldMeta{
			Description: "Address the server binds to.",
			Default:     "localhost",
			Example:     "host: 0.0.0.0",
			Snippet:     "  host: localhost\n",
			PreChecked:  true,
		}},
		"port": {FieldMeta: editor.FieldMeta{
			Description: "TCP port to listen on.",
			Default:     "8080",
			Example:     "port: 8080",
			Snippet:     "  port: 8080\n",
			PreChecked:  true,
		}},
		"tls": {FieldMeta: editor.FieldMeta{
			Description: "Enable TLS (HTTPS). Requires a certificate and key.",
			Default:     "false",
			Example:     "tls: true",
			Snippet:     "  tls: false\n",
		}},
		"allowed-ips": {FieldMeta: editor.FieldMeta{
			Description: "CIDR ranges or IPs allowed to connect. Empty means allow all.",
			Example:     "allowed-ips:\n  - 127.0.0.1\n  - 192.168.0.0/24\n  - 10.0.0.0/8",
			Snippet:     "  allowed-ips:\n    - 127.0.0.1\n",
		}},
		"headers": {FieldMeta: editor.FieldMeta{
			Description: "HTTP response headers added to every reply.",
			Example:     "headers:\n  X-Request-ID: \"\"\n  X-Forwarded-For: \"\"",
			Snippet:     "  headers:\n    X-Request-ID: \"\"\n",
		}},
	}
}

func (PoolConfig) Metadata() map[string]*metadata.Node {
	return map[string]*metadata.Node{
		"min-size": {FieldMeta: editor.FieldMeta{
			Description: "Minimum number of idle connections.",
			Default:     "2",
		}},
		"max-size": {FieldMeta: editor.FieldMeta{
			Description: "Maximum number of open connections.",
			Default:     "10",
		}},
		"timeout": {FieldMeta: editor.FieldMeta{
			Description: "Seconds to wait for an available connection.",
			Default:     "30",
		}},
	}
}

func (DatabaseConfig) Metadata() map[string]*metadata.Node {
	return map[string]*metadata.Node{
		"driver": {FieldMeta: editor.FieldMeta{
			OneOf:   []string{"postgres", "mysql", "sqlite"},
			Snippet: "  driver: postgres\n",
		}},
		"dsn": {FieldMeta: editor.FieldMeta{
			Snippet: "  dsn: \"postgres://localhost/mydb\"\n",
		}},
		"max-conns": {FieldMeta: editor.FieldMeta{
			Snippet: "  max-conns: 10\n",
		}},
		"pool": {
			FieldMeta: editor.FieldMeta{
				Snippet: "  pool:\n    min-size: 2\n    max-size: 10\n    timeout: 30\n",
			},
			Children: PoolConfig{}.Metadata(),
		},
	}
}

func (LoggingConfig) Metadata() map[string]*metadata.Node {
	return map[string]*metadata.Node{
		"level": {FieldMeta: editor.FieldMeta{
			Description: "Minimum severity to emit. Lower levels produce more output.",
			Required:    true,
			OneOf:       []string{"debug", "info", "warn", "error"},
			Default:     "info",
			Example:     "level: info",
			Snippet:     "  level: info\n",
			PreChecked:  true,
		}},
		"file": {FieldMeta: editor.FieldMeta{
			Description: "Path to the log file. Supports ~ for the home directory.",
			Example:     "file: /var/log/app.log",
			Snippet:     "  file: \"/var/log/app.log\"\n",
		}},
		"show-caller": {FieldMeta: editor.FieldMeta{
			Description: "Append source file and line number to each log entry.",
			Default:     "false",
			Example:     "show-caller: false",
			Snippet:     "  show-caller: false\n",
		}},
	}
}

func (DeployConfig) Metadata() map[string]*metadata.Node {
	return map[string]*metadata.Node{
		"strategy": {FieldMeta: editor.FieldMeta{
			OneOf:      []string{"rolling", "blue-green", "canary"},
			Snippet:    "  strategy: rolling\n",
			PreChecked: true,
		}},
		"replicas": {FieldMeta: editor.FieldMeta{
			Snippet:    "  replicas: 1\n",
			PreChecked: true,
		}},
		"enabled": {FieldMeta: editor.FieldMeta{
			Snippet: "  enabled: true\n",
		}},
		"auto-revert": {FieldMeta: editor.FieldMeta{
			Description: "Roll back automatically on a failed deploy.",
			Default:     "false",
		}},
	}
}

func (NetworkConfig) Metadata() map[string]*metadata.Node {
	return map[string]*metadata.Node{
		"endpoint": {FieldMeta: editor.FieldMeta{
			Description: "HTTP/HTTPS endpoint URL.",
			Formats:     []editor.Format{editor.FormatURL},
			Required:    true,
		}},
		"host": {FieldMeta: editor.FieldMeta{
			Description: "Hostname or IPv4 address (OR semantics: matches either).",
			Formats:     []editor.Format{editor.FormatHost, editor.FormatIPv4},
		}},
		"cidr": {FieldMeta: editor.FieldMeta{
			Description: "CIDR block (e.g. 10.0.0.0/8).",
			Formats:     []editor.Format{editor.FormatCIDR},
		}},
		"uuid": {FieldMeta: editor.FieldMeta{
			Formats: []editor.Format{editor.FormatUUID},
		}},
		"tag": {FieldMeta: editor.FieldMeta{
			Formats:   []editor.Format{editor.FormatSemver},
			MinLength: 5,
		}},
		"protocol": {FieldMeta: editor.FieldMeta{
			OneOf:    []string{"http", "https", "grpc", "ws"},
			NotOneOf: []string{"ftp", "telnet"},
		}},
		"note-name": {FieldMeta: editor.FieldMeta{
			MinLength: 3,
			MaxLength: 64,
		}},
		"any-ip": {FieldMeta: editor.FieldMeta{
			Description: "IPv4 or IPv6 address.",
			Formats:     []editor.Format{editor.FormatIP},
		}},
		"listen": {FieldMeta: editor.FieldMeta{
			Formats: []editor.Format{editor.FormatHostPort},
			Example: "listen: \"0.0.0.0:8080\"",
		}},
		"http-port": {FieldMeta: editor.FieldMeta{
			Formats: []editor.Format{editor.FormatPort},
		}},
		"timeout": {FieldMeta: editor.FieldMeta{
			Formats: []editor.Format{editor.FormatDuration},
			Example: "timeout: 30s",
		}},
		"expiry": {FieldMeta: editor.FieldMeta{
			Formats: []editor.Format{editor.FormatDate},
			Example: "expiry: 2026-12-31",
		}},
	}
}

func (SecurityConfig) Metadata() map[string]*metadata.Node {
	return map[string]*metadata.Node{
		"ipv6-addr": {FieldMeta: editor.FieldMeta{
			Formats: []editor.Format{editor.FormatIPv6},
			Example: "ipv6-addr: \"::1\"",
		}},
		"public-key": {FieldMeta: editor.FieldMeta{
			Multiline: true,
			Formats:   []editor.Format{editor.FormatPublicKey},
			Example:   "public-key: |\n  -----BEGIN PUBLIC KEY-----\n  ...\n  -----END PUBLIC KEY-----\n",
		}},
		"private-key": {FieldMeta: editor.FieldMeta{
			Multiline: true,
			Formats:   []editor.Format{editor.FormatPrivateKey},
			Example:   "private-key: |\n  -----BEGIN PRIVATE KEY-----\n  ...\n  -----END PRIVATE KEY-----\n",
		}},
		"fqdn": {FieldMeta: editor.FieldMeta{
			Formats: []editor.Format{editor.FormatFQDN},
			Example: "fqdn: api.example.com",
		}},
		"email": {FieldMeta: editor.FieldMeta{
			Formats: []editor.Format{editor.FormatEmail},
			Example: "email: admin@example.com",
		}},
	}
}

func (DeployExtConfig) Metadata() map[string]*metadata.Node {
	return map[string]*metadata.Node{
		"source": {FieldMeta: editor.FieldMeta{
			Description: "Terraform module source reference.",
			Formats:     []editor.Format{editor.FormatTerraformSource},
			Snippet:     "  source: git::https://github.com/company/module.git?ref=v1.0.0\n",
			PreChecked:  true,
		}},
		"script": {FieldMeta: editor.FieldMeta{
			Description: "Deploy shell script (Multiline=true, no Example - auto-generation exercised).",
			Multiline:   true,
			PreChecked:  true,
		}},
		"readme": {FieldMeta: editor.FieldMeta{
			Description: "Deployment notes (Multiline=true with explicit Example).",
			Multiline:   true,
			Example:     "readme: |\n  # My Deploy\n  Step 1: do X\n  Step 2: do Y\n",
		}},
		"git-ref": {FieldMeta: editor.FieldMeta{
			Formats: []editor.Format{editor.FormatGitRef},
			Example: "git-ref: v1.2.3",
		}},
		"dir-path": {FieldMeta: editor.FieldMeta{
			Description: "Cross-platform directory path (no existence check).",
			Formats:     []editor.Format{editor.FormatDirectoryPath},
			Example:     "dir-path: /opt/deploy",
		}},
	}
}

func (Worker) Metadata() map[string]*metadata.Node {
	return map[string]*metadata.Node{
		"name": {FieldMeta: editor.FieldMeta{
			Description: "Unique worker name.",
			Required:    true,
		}},
		"concurrency": {FieldMeta: editor.FieldMeta{
			Description: "Number of parallel goroutines.",
			Default:     "1",
		}},
		"queue": {FieldMeta: editor.FieldMeta{
			Description: "Queue name this worker drains.",
		}},
		"extensions": {FieldMeta: editor.FieldMeta{
			Description: "File extensions this worker handles (flow-style in seed).",
		}},
		"tags": {FieldMeta: editor.FieldMeta{
			Description: "Arbitrary string tags.",
		}},
	}
}

func (Route) Metadata() map[string]*metadata.Node {
	return map[string]*metadata.Node{
		"path": {FieldMeta: editor.FieldMeta{
			Description: "URL path pattern.",
			Required:    true,
			Example:     "path: /api/v1/users",
		}},
		"method": {FieldMeta: editor.FieldMeta{
			Description: "HTTP method.",
			Required:    true,
			OneOf:       []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		}},
		"handler": {FieldMeta: editor.FieldMeta{
			Description: "Handler function reference.",
			Required:    true,
		}},
		"auth": {FieldMeta: editor.FieldMeta{
			Description: "Require authentication for this route.",
			Default:     "false",
		}},
	}
}

func (Filter) Metadata() map[string]*metadata.Node {
	return map[string]*metadata.Node{
		"regex": {FieldMeta: editor.FieldMeta{
			Description: "Regular expression to match against file paths.",
		}},
		"glob": {FieldMeta: editor.FieldMeta{
			Description: "Glob pattern to match against file paths.",
			Example:     "glob: \"**/*.go\"",
		}},
		"include": {FieldMeta: editor.FieldMeta{
			Description: "Explicit paths to include.",
		}},
		"ignore": {FieldMeta: editor.FieldMeta{
			Description: "Paths to exclude even if matched by other rules.",
		}},
		"case-sensitive": {FieldMeta: editor.FieldMeta{
			Description: "Case-sensitive matching.",
			Default:     "false",
		}},
		"any": {FieldMeta: editor.FieldMeta{
			Description: "Sub-filters combined with OR semantics (self-referential).",
		}},
		"all": {FieldMeta: editor.FieldMeta{
			Description: "Sub-filters combined with AND semantics (self-referential).",
		}},
	}
}

func (PortAttr) Metadata() map[string]*metadata.Node {
	return map[string]*metadata.Node{
		"label": {FieldMeta: editor.FieldMeta{
			Description: "Human-readable label for the port.",
		}},
		"on-auto-forward": {FieldMeta: editor.FieldMeta{
			Description: "Action when the port is auto-forwarded.",
			OneOf:       []string{"notify", "openBrowser", "silent", "ignore"},
			Default:     "notify",
		}},
		"protocol": {FieldMeta: editor.FieldMeta{
			Description: "Network protocol for this port.",
			OneOf:       []string{"http", "https", "tcp"},
			Default:     "http",
		}},
	}
}

func (PortRule) Metadata() map[string]*metadata.Node {
	return map[string]*metadata.Node{
		"proto": {FieldMeta: editor.FieldMeta{
			Description: "IP protocol.",
			OneOf:       []string{"tcp", "udp", "icmp"},
		}},
		"allowed": {FieldMeta: editor.FieldMeta{
			Description: "Whether traffic matching this rule is allowed.",
			Default:     "false",
		}},
	}
}

func (SchemaEdgeCases) Metadata() map[string]*metadata.Node {
	return map[string]*metadata.Node{
		"replicas": {FieldMeta: editor.FieldMeta{
			Description: "Replica count (omitempty: omitted when 0).",
			Default:     "0",
		}},
		"ips": {FieldMeta: editor.FieldMeta{
			Description: "IP list serialized in flow style.",
			Example:     "ips: [10.0.0.1, 10.0.0.2]",
		}},
		"firewall-rules": {
			FieldMeta: editor.FieldMeta{
				Description: "Firewall rules indexed by priority (integer map key).",
			},
			Children: PortRule{}.Metadata(),
		},
		"background": {FieldMeta: editor.FieldMeta{
			Description: "Background color (#rrggbb, marshalled via MarshalYAML).",
			Example:     "background: \"#1e1e2e\"",
		}},
		"extras": {FieldMeta: editor.FieldMeta{
			Description: "Arbitrary value (interface{} / KindAny).",
		}},
	}
}
