package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AgusRdz/local-mind/config"
)

func TestCollectNotes_MissingVsForceVsIgnore(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("has.md", "---\nname: has\naliases: [x]\n---\nbody")
	write("missing.md", "---\nname: missing\n---\nbody")
	write("templates/tpl.md", "---\nname: tpl\n---\nbody") // ignored

	cfg := config.Config{Sources: []string{root}, Ignore: []string{"**/templates/**"}}

	notes, skipped := collectNotes(cfg, false)
	if len(notes) != 1 || filepath.Base(notes[0]) != "missing.md" {
		t.Fatalf("default: expected only missing.md, got %v", notes)
	}
	if skipped != 1 {
		t.Errorf("expected 1 skipped (has aliases), got %d", skipped)
	}

	forced, _ := collectNotes(cfg, true)
	if len(forced) != 2 {
		t.Errorf("--force: expected both non-ignored notes, got %d (%v)", len(forced), forced)
	}
	for _, p := range forced {
		if strings.Contains(filepath.ToSlash(p), "/templates/") {
			t.Error("ignored templates note should never be collected")
		}
	}
}

func TestUpsertFrontmatter_ReplaceInlineAliases(t *testing.T) {
	src := []byte("---\nname: wt\ndescription: git worktrees\naliases: [old one]\n---\nbody here\n")
	out, err := upsertFrontmatter(src, "wt.md", []string{"parallel branches", "concurrent"}, "")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "old one") {
		t.Error("old alias not removed")
	}
	if !strings.Contains(s, `aliases: ["parallel branches", "concurrent"]`) {
		t.Errorf("new aliases missing:\n%s", s)
	}
	if !strings.Contains(s, "name: wt") || !strings.Contains(s, "description: git worktrees") {
		t.Error("other frontmatter keys not preserved")
	}
	if !strings.Contains(s, "body here") {
		t.Error("body not preserved")
	}
}

func TestUpsertFrontmatter_ReplaceBlockAliases(t *testing.T) {
	src := []byte("---\nname: wt\naliases:\n  - old a\n  - old b\ntags: [x]\n---\nbody\n")
	out, err := upsertFrontmatter(src, "wt.md", []string{"new one"}, "")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "old a") || strings.Contains(s, "old b") {
		t.Errorf("block aliases not removed:\n%s", s)
	}
	if !strings.Contains(s, "tags: [x]") {
		t.Error("tags key (after block) not preserved")
	}
	if !strings.Contains(s, `aliases: ["new one"]`) {
		t.Errorf("new aliases missing:\n%s", s)
	}
}

func TestUpsertFrontmatter_AppendWhenAbsent(t *testing.T) {
	src := []byte("---\nname: wt\ndescription: d\n---\nbody\n")
	out, err := upsertFrontmatter(src, "wt.md", []string{"a", "b"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `aliases: ["a", "b"]`) {
		t.Errorf("aliases not appended:\n%s", out)
	}
}

func TestUpsertFrontmatter_SetDescriptionOnlyWhenMissing(t *testing.T) {
	src := []byte("---\nname: wt\n---\nbody\n")
	out, _ := upsertFrontmatter(src, "wt.md", []string{"a"}, "a fresh description")
	if !strings.Contains(string(out), "description: a fresh description") {
		t.Errorf("description not set when missing:\n%s", out)
	}
}

func TestUpsertFrontmatter_NoFrontmatterCreatesOne(t *testing.T) {
	src := []byte("just body text\nmore body\n")
	out, err := upsertFrontmatter(src, "notes/wt.md", []string{"a"}, "desc")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.HasPrefix(s, "---\n") {
		t.Errorf("frontmatter not created:\n%s", s)
	}
	if !strings.Contains(s, "name: wt") || !strings.Contains(s, `aliases: ["a"]`) || !strings.Contains(s, "description: desc") {
		t.Errorf("created frontmatter incomplete:\n%s", s)
	}
	if !strings.Contains(s, "just body text") {
		t.Error("original body lost")
	}
}

func TestParseSuggestion_ExtractsJSONFromProse(t *testing.T) {
	out := []byte("Sure! Here are the aliases:\n{\"aliases\": [\"foo\", \" bar \", \"\"], \"description\": \" a desc \"}\nHope that helps.")
	s, err := parseSuggestion(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Aliases) != 2 || s.Aliases[0] != "foo" || s.Aliases[1] != "bar" {
		t.Errorf("aliases not cleaned/trimmed: %#v", s.Aliases)
	}
	if s.Description != "a desc" {
		t.Errorf("description = %q", s.Description)
	}
}
