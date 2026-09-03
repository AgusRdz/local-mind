# local-mind

A deterministic retrieval bridge between a plain-text "second brain" (Obsidian vault + Claude Code memory dirs) and Claude Code.

A `UserPromptSubmit` hook runs an FTS5 keyword match over your notes **before the prompt reaches the model** and injects the relevant note(s) as context — with confidence-banded, budget-capped output. Retrieval is LLM-free: zero token cost when nothing matches, no round-trip cost when something does.

> **Why:** `MEMORY.md` / `CLAUDE.md` get loaded whole every session and don't scale to a vault; ad hoc lookups otherwise cost a paid round-trip. local-mind surfaces only the relevant slice, deterministically, in the hook.

## How it works

1. `rebuild` indexes every `.md` under your configured sources into a local SQLite FTS5 database (Porter stemmer; structural fields — `name`, `description`, `aliases`, headings — weighted above body).
2. On each prompt, the hook runs the same keyword match and injects results by **confidence band**:
   - **very-high** → the note body (budget-capped)
   - **high** → `description` + path only (Claude reads the file itself if it wants more)
   - **low** → nothing injected; candidates printed to the trace
3. Every injection prints an inline trace to stderr, with note age and score.

A note with `private: true` in its frontmatter is indexed for local `grep` but **never** auto-injected.

## Install

Built as a single static Go binary (no cgo). Requires Docker for building (the host needs no Go runtime).

```bash
make install          # build in Docker + copy to your local bin
local-mind init       # register the UserPromptSubmit hook in ~/.claude/settings.json
local-mind rebuild    # build the index
```

Then start a new Claude Code session.

## Configuration

`~/.local-mind/config.yml` (scaffolded on first run):

```yaml
sources:
  - /home/you/.claude/memory
  - /home/you/dev/second-brain
ignore:
  - "**/templates/**"
bands:
  very_high: 0.60   # >= this -> inject body
  high: 0.35        # >= this -> inject description
budget:
  max_notes: 3
  max_tokens: 1200
```

Tune the bands from what `local-mind stats` shows. Index: `~/.local-mind/index.db`. Trace log: `~/.local-mind/trace.log`.

## Commands

```
local-mind rebuild [--incremental]   build the index (incremental = only changed files)
local-mind grep "<query>"            manual query, same matching as the hook
local-mind init [--status]           install the hook / show status
local-mind uninstall                 remove the hook
local-mind stats                     injection / miss summary from the trace log
local-mind version
```

## Design

See [`docs/PLAN.md`](docs/PLAN.md) and [`ROADMAP.md`](ROADMAP.md). Toolchain mirrors [`chop`](https://github.com/AgusRdz/chop): Go + Docker + `modernc.org/sqlite`, git-cliff changelog, signed cross-platform releases.
