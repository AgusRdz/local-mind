# local-mind — Project Instructions

## Tooling runs inside Docker — never on the host

The host has no Go runtime. All Go tooling runs inside the container via the
Makefile or `docker compose run --rm dev <cmd>`:

```bash
make build            # or: docker compose run --rm dev go build ./...
make test             # or: docker compose run --rm dev go test ./...
docker compose run --rm dev go vet ./...
docker compose run --rm dev go mod tidy
```

Never run `go` directly on the host — it will fail.

## What this is

A deterministic retrieval bridge: a `UserPromptSubmit` hook that runs an FTS5
keyword match over the Obsidian vault + Claude Code memory dirs *before* the
prompt reaches the model, injecting the relevant note(s) by term-coverage
confidence band (body / desc / candidate) under a token budget, with a privacy
gate (`private: true` notes are never injected). Retrieval is LLM-free.

Layout: `main.go` (command dispatch) · `config/` (config.yml) · `index/` (FTS5
schema, frontmatter parsing, ranked search) · `hooks/` (UserPromptSubmit
entrypoint + settings.json wiring). See `docs/PLAN.md` and `ROADMAP.md`.

## Conventions

- Conventional Commits (drives git-cliff changelog + `make release`).
- This repo lives under `dev/`, whose git `includeIf` sets the correct authorship
  identity automatically — do not override `user.email` on commits here.
