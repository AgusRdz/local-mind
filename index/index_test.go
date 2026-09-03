package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AgusRdz/local-mind/config"
)

func writeFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func newTestIndex(t *testing.T) (*Index, config.Config, string) {
	t.Helper()
	notes := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.db")
	idx, err := OpenAt(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { idx.Close() })
	cfg := config.Config{
		Sources: []string{notes},
		Bands:   config.Bands{VeryHigh: 0.50, High: 0.30},
		Budget:  config.Budget{MaxNotes: 3, MaxTokens: 1200},
	}
	return idx, cfg, notes
}

func TestRebuildAndSearch_AliasBeatsIncidentalBodyHit(t *testing.T) {
	idx, cfg, notes := newTestIndex(t)

	writeFixture(t, notes, "worktree.md", `---
name: worktree-runner
description: Thin Go CLI wrapping git worktrees
aliases: [parallel branches, concurrent worktrees]
---
Planned as one command per parallel branch.
`)
	// An unrelated note that merely mentions "worktrees" once in the body.
	writeFixture(t, notes, "misc.md", `---
name: misc-notes
description: Assorted stuff
---
We briefly discussed worktrees at standup but moved on.
`)

	indexed, _, err := idx.Rebuild(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if indexed != 2 {
		t.Fatalf("indexed = %d, want 2", indexed)
	}

	results, err := idx.Search("concurrent worktrees", cfg.Bands, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("no results")
	}
	if filepath.Base(results[0].Path) != "worktree.md" {
		t.Errorf("top result = %s, want worktree.md (alias match should outrank incidental body hit)", results[0].Path)
	}
}

func TestSearch_PrivateFlagPreserved(t *testing.T) {
	idx, cfg, notes := newTestIndex(t)
	writeFixture(t, notes, "secret.md", `---
name: secret-note
description: sensitive client retry details
private: true
---
retry retry retry
`)
	if _, _, err := idx.Rebuild(cfg, false); err != nil {
		t.Fatal(err)
	}
	results, err := idx.Search("retry", cfg.Bands, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !results[0].Private {
		t.Fatalf("expected 1 private result, got %+v", results)
	}
}

func TestRebuildIncremental_SkipsUnchanged(t *testing.T) {
	idx, cfg, notes := newTestIndex(t)
	writeFixture(t, notes, "a.md", "---\nname: a\n---\nalpha content")
	if _, _, err := idx.Rebuild(cfg, false); err != nil {
		t.Fatal(err)
	}
	_, skipped, err := idx.Rebuild(cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1 (unchanged file)", skipped)
	}
}

// Regression: a query that matches on a single term must not reach the body
// band (which would dump a full note on one common word).
func TestSearch_SingleTermCapsAtDesc(t *testing.T) {
	idx, cfg, notes := newTestIndex(t)
	writeFixture(t, notes, "wt.md", `---
name: worktree-runner
description: git worktrees CLI
aliases: [worktree]
---
some body
`)
	if _, _, err := idx.Rebuild(cfg, false); err != nil {
		t.Fatal(err)
	}
	res, err := idx.Search("worktree", cfg.Bands, 5) // single significant term
	if err != nil {
		t.Fatal(err)
	}
	if len(res) == 0 {
		t.Fatal("no results")
	}
	if res[0].Band == BandBody {
		t.Errorf("single-term match reached body band (conf %.2f); want desc or lower", res[0].Conf)
	}
}

// A genuinely multi-term match still reaches the body band.
func TestSearch_MultiTermReachesBody(t *testing.T) {
	idx, cfg, notes := newTestIndex(t)
	writeFixture(t, notes, "wt.md", `---
name: worktree-runner
description: git worktrees CLI
aliases: [concurrent worktrees, parallel branches]
---
body
`)
	if _, _, err := idx.Rebuild(cfg, false); err != nil {
		t.Fatal(err)
	}
	res, err := idx.Search("concurrent worktrees", cfg.Bands, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) == 0 || res[0].Band != BandBody {
		t.Fatalf("multi-term structural match should reach body band, got %+v", res)
	}
}

func TestClassify(t *testing.T) {
	b := config.Bands{VeryHigh: 0.50, High: 0.30}
	cases := []struct {
		conf float64
		want string
	}{
		{0.80, BandBody},
		{0.50, BandBody},
		{0.40, BandDesc},
		{0.30, BandDesc},
		{0.10, BandLow},
	}
	for _, c := range cases {
		if got := classify(c.conf, b); got != c.want {
			t.Errorf("classify(%.2f) = %s, want %s", c.conf, got, c.want)
		}
	}
}

func TestIgnoreGlob(t *testing.T) {
	if !ignored("/x/templates/note.md", []string{"**/templates/**"}) {
		t.Error("templates path should be ignored")
	}
	if ignored("/x/areas/note.md", []string{"**/templates/**"}) {
		t.Error("areas path should not be ignored")
	}
}
