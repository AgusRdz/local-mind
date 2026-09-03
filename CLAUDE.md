# local-mind — Project Instructions

## Tooling runs inside Docker — never on the host

The host machine has no Go runtime. All Go tooling runs inside the Docker container:

```bash
docker compose run --rm dev go build ./...
docker compose run --rm dev go test ./...
docker compose run --rm dev go vet ./...
docker compose run --rm dev go mod tidy
```

Never run `go` directly on the host — it will fail.

## What this is

A deterministic retrieval bridge: a `UserPromptSubmit` hook that runs an FTS5 keyword
match over the Obsidian vault + Claude Code memory dirs before the prompt reaches the
model, and injects the relevant note(s) with confidence-banded, budget-capped output.
Retrieval is LLM-free by design. See `docs/PLAN.md` and `ROADMAP.md`.
