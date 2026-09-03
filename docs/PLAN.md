# local-mind — Project Plan (v2)

A deterministic retrieval bridge between a plain-text "second brain" (Obsidian vault + Claude Code memory dirs) and Claude Code, so relevant past context is injected automatically — before the model sees the prompt — with zero token cost when nothing matches and no round-trip cost when something does.

> **v2 changes** (vs. the original Downloads draft): implementation locked to **Go + Docker + `modernc.org/sqlite`** (mirrors the `chop` toolchain); retrieval widened to **index the existing Claude Code memory dirs alongside the vault** (the real gap); **two-tier confidence-banded injection** replaces blind full-file injection; **write-time alias generation** removes the manual-curation rot; plus a **privacy gate**, **age-in-trace**, and an **explicit feedback signal**. Rationale for each is inline below.

---

## 1. Problem

Working across many repos daily, Claude Code has no durable recall of prior decisions, context, or notes unless:
- it's manually pasted into the conversation each time, or
- Claude discovers it via ad hoc file reads/greps (a paid round-trip per lookup), or
- it's stuffed into `CLAUDE.md` / `MEMORY.md`, which grow unbounded and get re-read in full every session.

The memory + `/brain` vault system already covers *authoring* and *high-frequency* recall. What it does **not** solve: ad hoc cross-repo lookups still cost round-trips, and `MEMORY.md` doesn't scale to vault-sized recall — it's loaded whole, every session, whether relevant or not.

## 2. Goal

Automatically surface relevant notes into Claude Code's context, with:
- **Zero added cost when nothing is relevant.**
- **No hidden round-trip cost when something is** — retrieval runs in a hook, before the model.
- **Full visibility** into what was injected and why.
- **No new storage format** — files stay human-readable, `grep`-able, git-native.
- **Scales past MEMORY.md** — the hook surfaces the relevant subset on demand, so the always-loaded index can shrink.

---

## 3. Design Principles

1. **Files, not a database.** Notes are plain Markdown + frontmatter. The index is a disposable, rebuildable cache.
2. **Deterministic before probabilistic.** Retrieval is a plain FTS5 keyword/structural match, run before the prompt reaches the model — never a decision the model reasons about or pays for.
3. **Visible by default.** Every injection prints an inline trace to stderr, in the same terminal session.
4. **No dependency until proven necessary.** No embeddings, no vector index, no ML runtime, no daemon — until keyword search demonstrably fails.
5. **Git is the source of truth.** Sync, backup, and history come from git, not a custom service.
6. **The zero-cost rule applies to the *retrieval* path, not the *authoring* path.** Using the model offline at write time (to generate aliases/summaries) is free at read time and does not violate principle 2. *(v2)*

---

## 4. Architecture

### Toolchain (locked — matches `chop`)
- **Go 1.24**, single static binary, `CGO_ENABLED=0`.
- **`modernc.org/sqlite`** — pure-Go SQLite with FTS5 compiled in. No cgo, no external SQLite.
- **`gopkg.in/yaml.v3`** — frontmatter + config parsing.
- **Built in Docker** (`golang:1.24-alpine`, `docker compose run --rm dev go ...`). The host has no Go runtime by design; never build on the host.

### Sources indexed *(v2 — the value multiplier)*
One FTS5 store over **all** note sources, resolved from config:
- The Obsidian vault (`second-brain/`).
- Claude Code global memory (`~/.claude/memory/`).
- Per-project memory (`<repo>/memory/`) for the current repo.

This is what lets `MEMORY.md` stop being loaded whole every session — the hook surfaces only the relevant slice.

### Storage layout
```
~/.local-mind/
├── config.yml     # sources, thresholds, budgets, ignore globs
├── index.db       # FTS5 cache, rebuildable, never hand-edited
└── trace.log      # append-only injection log (feeds `stats`)
```

Each note (unchanged format — no migration):
```markdown
---
name: worktree-runner
description: Thin Go CLI wrapping git worktrees + claude -p
aliases: [parallel branches, concurrent worktrees, multi-agent runner]
private: false            # v2: true -> indexed for local grep, NEVER auto-injected
---
- Planned as ~100 lines, one command per parallel branch
```

### Index
- SQLite + FTS5, **Porter stemmer** (catches `run`/`running`/`runner`).
- Indexes `name`, `description`, `aliases`, headings, first-line-of-bullet, and body — as weighted columns (bm25 column weights). Structural fields (name/description/aliases/headings) are boosted so a title hit outranks an incidental body hit.
- Rebuilt from files; `rebuild` supports incremental (mtime/git-diff) so it stays fresh cheaply.

### Retrieval trigger — the hook
- A Claude Code **`UserPromptSubmit`** hook runs before the prompt reaches the model.
- Reads the prompt from stdin, runs a plain FTS5 match — **no LLM call in the decision**.
- Emits matched context on **stdout** (added to the model's context) and a trace on **stderr**.
- No match → emits nothing. Zero added tokens, zero round-trip.

### Two-tier confidence-banded injection *(v2 — biggest risk fix)*
Blind full-file injection is the original plan's weakest point: a keyword collision dumps the wrong note's whole body into context. Instead, banded by a **term-coverage confidence** (0..1) against configurable thresholds, under a **hard token budget** (≤N notes, ≤M total tokens). Confidence measures how many query terms hit a note's *structural* fields (name/aliases/description/headings, strong) vs. only its body (weak) — corpus-independent, unlike raw bm25 magnitude, which is used only for ordering:

| Band | Action |
|------|--------|
| **very-high** | Inject the note body (still budget-capped). |
| **high** | Inject `description` + path only. Claude reads the full file itself if it wants it — now a cheap, *targeted* read of a file it knows exists. |
| **low** | Inject nothing into context; print one candidate line to the trace: `candidates: areas/x.md (0.62), …`. |

A false positive now costs one description line, not a full note.

### Visibility
Every injection prints an inline trace to stderr, with **note age** *(v2 — staleness cue, matches the "verify stale refs" memory rule)*:
```
[local-mind: injected areas/worktree-runner.md (age 2mo), matched "concurrent worktrees", score 0.88]
```

### CLI surface
```
local-mind rebuild [--incremental]      # (re)build FTS5 index from configured sources
local-mind grep "<query>" [--scope ...] # manual query, identical matching to the hook
local-mind hook                         # UserPromptSubmit entrypoint (stdin JSON -> stdout context)
local-mind init [--global|--status]     # register/inspect the hook in settings.json
local-mind uninstall                    # remove the hook
local-mind stats [--since 7d]           # injections, score histogram, likely-miss rate
local-mind bad                          # v2: mark the last injection unhelpful (hard feedback signal)
local-mind suggest-aliases <path>       # v2: offline model call, proposes aliases/description
local-mind version
```
No MCP server, no tool schema exposed to the model, no daemon. The CLI + hook are the entire interface.

> **Writes stay with `/brain`.** *(v2)* local-mind is retrieval-only — no `write` command. The `/brain` skill already owns note authoring and format; duplicating it here invites format drift for no gain.

### Default scope *(v2)*
The hook defaults to **current repo's `memory/` + the global vault** — not all repos — to prevent cross-project context bleed. `grep --scope all` overrides for manual queries.

### Feedback signals *(v2)*
- **Explicit (hard):** `local-mind bad` marks the last injection unhelpful. One keystroke of ground truth.
- **Inferred (soft):** an immediate re-ask after an injection is flagged a likely miss. Kept as a smell, but **not** used to auto-tune thresholds — too noisy on its own.

### Latency budget
The hook runs on **every** prompt. Target **< 50 ms** end-to-end. A Go static binary + a small local FTS5 query clears this comfortably; if it ever doesn't, that's a signal, logged in `stats`.

---

## 5. Example Day-to-Day Usage

```bash
$ claude "why did we move the retry logic out of the client SDK last month"
[local-mind: injected areas/sdk-retry-migration.md (age 3wk), matched "retry logic", score 0.91]
Claude: [answers directly, using the injected context]

$ local-mind grep "consumer lag"
# no match — nothing written yet

$ local-mind grep "retry" --scope all
areas/sdk-retry-migration.md   score 0.91  [inject: body]
areas/auth-proxy-retry.md      score 0.63  [inject: desc]

$ local-mind stats --since 1d
11 injections | 8 body / 2 desc / 1 candidate-only
score p50 0.71 p90 0.90 | 1 immediate re-ask, 0 explicit `bad` (est. miss rate ~9%)
```

---

## 6. Deferred until evidence justifies it
- Semantic/embedding-based reranking (a second stage *after* FTS5, never replacing it).
- A formal eval harness with a labeled corpus.
- An MCP tool interface for explicit, model-reasoned queries.
- Cross-machine sync beyond plain git.

## 7. Stage 0 note
The original plan gated the build behind a two-week manual-friction baseline. That gate is **intentionally skipped** at the owner's direction — the tool is being built now. Stage 0's *measurement* still happens, in reverse: `stats` + the `bad` signal produce the real quality data the baseline would have, but from live use instead of guesswork.

## 8. Open items (carried, now decidable from live use)
1. Threshold values for the very-high / high / low bands — seed with defaults, tune from the `stats` score histogram.
2. Token budget (N notes / M tokens) — seed conservative, widen if `stats` shows headroom.
3. Whether `suggest-aliases` runs via `claude -p` or a direct API call — decide when M4 lands.
