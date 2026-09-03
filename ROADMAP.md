# local-mind — Roadmap

Milestones are ordered by dependency and value. Each ships independently and leaves the binary in a working state. "Done when" = the acceptance bar; nothing is checked off until it passes in Docker.

Status legend: ☐ todo · ◐ in progress · ☑ done

---

## M0 — Scaffold & build infra ☑
Mirror the `chop` toolchain so this is a sibling, not a stranger.

- Go module `github.com/AgusRdz/local-mind`, `go 1.24`, deps: `modernc.org/sqlite`, `gopkg.in/yaml.v3`.
- `Dockerfile` (`golang:1.24-alpine`) + `docker-compose.yml` (dev service, go caches).
- `Makefile`: `build`, `test`, `coverage`, `cross`, `install`, `clean`, `changelog`, `release[-patch|-minor|-major]`.
- `.gitignore`, `LICENSE` (MIT), `cliff.toml`, `CLAUDE.md` (Docker-only tooling note).
- `main.go` with command dispatch + `version` (ldflags `-X main.version`).

**Done when:** `make build` produces `bin/local-mind` in Docker and `local-mind version` prints the ldflags version.

---

## M1 — Index core (rebuild + grep) ☑
The deterministic retrieval engine. No hook yet — provable in isolation.

- `config` pkg: load/scaffold `~/.local-mind/config.yml` (sources, thresholds, budget, ignore globs).
- `index` pkg: FTS5 schema (Porter stemmer, weighted columns), frontmatter parser (`name`/`description`/`aliases`/`private`), structural extraction (headings, first-line-of-bullet).
- `rebuild` command: full + `--incremental` (mtime/git-diff); honors ignore globs; `private: true` still indexed (for grep) but flagged.
- `grep "<query>" [--scope current|all]`: bm25-ranked results with score + inject-band label.

**Done when:** on a fixture vault, `grep "concurrent worktrees"` returns the worktree note via its `aliases`, ranked above an incidental body hit, with correct band labels. Unit tests cover frontmatter parsing, private-flag handling, and ranking.

---

## M2 — The hook (retrieval → injection) ☑
The actual product: context appears automatically, safely, visibly.

- `hooks` pkg: `UserPromptSubmit` entrypoint — read stdin JSON (`prompt`, `cwd`), run the M1 matcher scoped to current repo + global vault.
- Two-tier confidence bands (very-high → body, high → desc+path, low → candidate trace) under a hard token budget (≤N notes / ≤M tokens).
- **Privacy gate:** `private: true` notes are never injected, regardless of score.
- stderr trace with note age + score; stdout carries only the injected context.
- `init [--global|--status]`, `uninstall`: settings.json wiring for `UserPromptSubmit` (adapted from chop's install.go; no `Bash` matcher).

**Done when:** piping a real prompt into `local-mind hook` injects the right note within budget, prints the age/score trace to stderr, injects nothing on no-match, and never injects a `private` note. `init --status` reports installed/not-installed. Latency < 50 ms on the live vault.

---

## M3 — Observability & feedback ☐
Turn live use into the quality data Stage 0 would have gathered.

- `trace.log` append on every hook invocation (query, matches, bands, scores, latency).
- `stats [--since]`: injection counts by band, score histogram (p50/p90), latency, likely-miss rate.
- `bad`: mark the last injection unhelpful (hard signal); folded into the miss rate.
- Soft re-ask heuristic surfaced in `stats` but never auto-tuning thresholds.

**Done when:** `stats --since 1d` reports counts, histogram, and miss rate from a seeded trace log; `bad` flips the last entry and moves the number.

---

## M4 — Authoring aids ☐
Kill the alias-curation rot and keep the index fresh with zero thought.

- `suggest-aliases <path>`: offline model call (via `claude -p` or API) proposing `aliases` + a tight `description`; prints a diff, applies on confirm. Read-time path stays LLM-free.
- Auto-boost structural fields already covered in M1; here add a `rebuild --stale` that only touches files changed since last index (git-aware).
- Optional git `post-commit` snippet (documented, not force-installed) to reindex changed notes automatically.

**Done when:** `suggest-aliases` on an alias-less note proposes sensible aliases and, on confirm, writes valid frontmatter that M1 then indexes.

---

## M5 — Distribution ◐
Ship it like chop: signed, checksummed.

- ☑ `install.ps1` + `install.sh` — anonymous download (chop-style), SHA256 verify, optional Ed25519 signature verify, PATH wiring.
- ☑ `.github/workflows`: `ci.yml` (test on push/PR) + `release.yml` (cross-build 5 targets, checksums, optional sign, GitHub Release + generated notes, provenance attestation) on `v*` tags.
- ☑ `README.md` + `docs/RELEASING.md` (signing setup, release + install steps).
- ☐ Push the workflows (blocked on `gh auth refresh -s workflow`).
- ☐ Cut `v0.1.0` and verify install from the release.
- ☐ (deferred) `update` self-update command; Homebrew tap (now that the repo is public).

**Done when:** a `v0.1.0` tag produces a GitHub Release with cross-platform binaries + checksums, and `install.sh`/`install.ps1` install and verify a binary from it.

---

## Deferred (evidence-gated)
- Semantic/embedding rerank as an optional second stage after FTS5.
- Formal eval harness with a labeled corpus.
- MCP tool interface for explicit model-reasoned queries.
- Cross-machine sync beyond git.

---

## Build order for this session
M0 → M1 → M2 gets a **working, installable retrieval hook** — the core value. M3–M5 are fast follows. This session targets **M0–M2**; M3–M5 tracked here for continuation.
