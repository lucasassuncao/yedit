# Contributing to yedit

Thank you for your interest in contributing. This document covers how to report issues, propose changes, and submit pull requests.

yedit is a library: it has no binary of its own, so most reports come from a program that embeds it. [`cmd/demo`](cmd/demo/main.go) is the quickest way to reproduce something without your own app.

---

## Reporting issues

Open a GitHub issue and include:

- **What you expected** to happen
- **What actually happened** (paste the full error output)
- **Steps to reproduce**: the schema struct and `Metadata()` involved, the `editor.Config` you pass to `editor.Run`, and the YAML file being edited. A reduced version of `cmd/demo` that shows the problem is ideal.
- **Environment:** OS, architecture, terminal, Go version, and the yedit version from your `go.mod`

For a bug in the editor, set `Config.Trace.Dump: true` and attach the JSONL trace whose path comes back in `Result.DumpPath` (see [Session Tracing](docs/SESSION-TRACING.md)). It records every keystroke and action of the session, so check it for anything sensitive first.

Layout, list and tree widgets, dialogs and themes come from [bezel](https://github.com/lucasassuncao/bezel). If the problem is in how something is drawn rather than in the YAML, the schema or the editor's behavior, it may belong there.

For security vulnerabilities, do **not** open a public issue. Email the maintainer directly.

---

## Proposing changes

For non-trivial changes, open an issue or start a discussion before writing code. This avoids investing time in an implementation that conflicts with the project direction. That goes double for anything that changes the public API, since every app that embeds yedit inherits it.

Small fixes (typos, one-line corrections) can go straight to a pull request.

---

## Submitting a pull request

1. Fork the repository and create a branch from `main`.
2. Follow the [Development Guide](docs/dev/DEVELOPMENT.md) for setup and workflow, and read the [Architecture](docs/dev/ARCHITECTURE.md) before moving code between packages.
3. Run the full check suite before pushing:
   ```bash
   make all
   ```
   The pinned tools install into `./.gobin` on first use; nothing beyond `go` needs to be on your `PATH`.
4. Open a pull request against `main`.

If your change also needs a change in bezel, a local `go.work` with `use (. ../bezel)` builds against your checkout. It is gitignored, so the pull request still has to build against the bezel version pinned in `go.mod`.

### PR checklist

- [ ] `make all` passes locally (deps, fmt, docs, lint, security, tests, govulncheck, SBOM scan)
- [ ] New behavior is covered by tests
- [ ] Package READMEs regenerated with `make docs` and committed; CI fails on a stale diff
- [ ] Reference docs under `docs/` follow the change: a new `editor.Config` field in [Config Reference](docs/CONFIG-REFERENCE.md), a new built-in rule in [Validators](docs/VALIDATORS.md), a key binding in [Interaction Model](docs/INTERACTION.md)
- [ ] `spec` still imports neither `validate` nor `editor`, and `schema`, `metadata`, `validate` and `document` still import no terminal library
- [ ] [Architecture](docs/dev/ARCHITECTURE.md) updated if a new package, pattern or decision was introduced
- [ ] `docs/demo.gif` re-recorded if the change alters what the README demo shows
- [ ] Commit messages follow the project style (see below)

### Commit message style

[Conventional Commits](https://www.conventionalcommits.org/), with the package as the scope when the change stays inside one. Short subject in imperative mood, lowercase, no trailing period. Add a body only when context is genuinely needed, written as a few lines of prose that say why, not what the diff already shows.

```
fix(editor): drop carriage returns from the preset preview

A CR left in a preset from a CRLF file sends the terminal cursor to
column 0, so the preview drew over the panel beside it. Applying a
preset already normalized the endings; the preview did not.
```

Mark a breaking change to the public API with `!` after the type or scope, and describe the migration in a `BREAKING CHANGE:` footer.

---

## Code of conduct

Be direct and constructive. Critique code, not people. Assume good intent.
