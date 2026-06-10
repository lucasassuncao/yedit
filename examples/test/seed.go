package main

// ── Seed YAML ─────────────────────────────────────────────────────────────────

const seedYAML = `import: shared.yaml

app-name: "my-app"
debug: false
version: "0.1.0"
port: 8080
ratio: 1.5
build-timeout: 30s
tags:
  - go
  - tui
settings:
  cache: true
  max-upload: 10mb
server:
  host: localhost
  port: 8080
  tls: false
  allowed-ips:
    - 127.0.0.1
    - 10.0.0.0/8
  headers:
    X-Request-ID: ""
    X-Service-Name: "my-app"
logging:
  level: info
  show-caller: false
workers:
  - name: "default"
    concurrency: 2
    queue: "main"
    extensions: ["go", "yaml"]
    tags:
      - critical
  - name: "background"
    concurrency: 1
    queue: "low"
    tags: []
  - name: "heavy"
    concurrency: 4
    queue: "batch"
port-attrs:
  "3000":
    label: "frontend"
    on-auto-forward: openBrowser
    protocol: http
  "8080":
    label: "api"
    on-auto-forward: notify
    protocol: http
filters:
  - glob: "*.go"
    case-sensitive: true
  - regex: ".*_test\\.go$"
    ignore:
      - vendor
edge-cases:
  created-by: "alice"
  version-tag: "v1.0"
  team: "platform"
  contact: "platform@example.com"
  replicas: 3
  ips: [10.0.0.1, 10.0.0.2]
  firewall-rules:
    8080:
      proto: tcp
      allowed: true
    443:
      proto: tcp
      allowed: true
  background: "#1e1e2e"
  extras: "free-form string"
unknown-key: "flagged by ctrl+l validate"
`
