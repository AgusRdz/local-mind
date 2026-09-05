package index

import (
	"path/filepath"
	"testing"
)

func TestDanglingLinks_ResolvesFrontmatterName(t *testing.T) {
	idx, cfg, notes := newTestIndex(t)
	writeFixture(t, notes, "a.md", "---\nname: note-a\n---\nsee [[note-b]]")
	writeFixture(t, notes, "b.md", "---\nname: note-b\n---\nbody")
	if _, _, err := idx.Rebuild(cfg, false); err != nil {
		t.Fatal(err)
	}
	dangling, err := idx.DanglingLinks()
	if err != nil {
		t.Fatal(err)
	}
	if len(dangling) != 0 {
		t.Fatalf("dangling = %+v, want none (link resolves)", dangling)
	}
}

func TestDanglingLinks_ResolvesFilenameStem(t *testing.T) {
	idx, cfg, notes := newTestIndex(t)
	writeFixture(t, notes, "a.md", "---\nname: note-a\n---\nsee [[b]]")
	writeFixture(t, notes, "b.md", "no frontmatter, name falls back to filename stem")
	if _, _, err := idx.Rebuild(cfg, false); err != nil {
		t.Fatal(err)
	}
	dangling, err := idx.DanglingLinks()
	if err != nil {
		t.Fatal(err)
	}
	if len(dangling) != 0 {
		t.Fatalf("dangling = %+v, want none (link resolves via filename stem)", dangling)
	}
}

func TestDanglingLinks_ReportsUnresolved(t *testing.T) {
	idx, cfg, notes := newTestIndex(t)
	writeFixture(t, notes, "a.md", "---\nname: note-a\n---\nsee [[missing-note]] and [[missing-note]] again")
	if _, _, err := idx.Rebuild(cfg, false); err != nil {
		t.Fatal(err)
	}
	dangling, err := idx.DanglingLinks()
	if err != nil {
		t.Fatal(err)
	}
	if len(dangling) != 1 {
		t.Fatalf("dangling = %+v, want exactly one note reported", dangling)
	}
	for path, targets := range dangling {
		if filepath.Base(path) != "a.md" {
			t.Errorf("reported path = %s, want a.md", path)
		}
		if len(targets) != 1 || targets[0] != "missing-note" {
			t.Errorf("targets = %v, want [missing-note] (deduped)", targets)
		}
	}
}

func TestDanglingLinks_SelfReferenceIgnored(t *testing.T) {
	idx, cfg, notes := newTestIndex(t)
	writeFixture(t, notes, "a.md", "---\nname: note-a\n---\nsee [[note-a]]")
	if _, _, err := idx.Rebuild(cfg, false); err != nil {
		t.Fatal(err)
	}
	dangling, err := idx.DanglingLinks()
	if err != nil {
		t.Fatal(err)
	}
	if len(dangling) != 0 {
		t.Fatalf("dangling = %+v, want none (self-reference ignored)", dangling)
	}
}

func TestDanglingLinks_CaseAnchorAndDisplayText(t *testing.T) {
	idx, cfg, notes := newTestIndex(t)
	writeFixture(t, notes, "a.md", "---\nname: note-a\n---\nsee [[Note-B#Some Heading|the other note]]")
	writeFixture(t, notes, "b.md", "---\nname: note-b\n---\nbody")
	if _, _, err := idx.Rebuild(cfg, false); err != nil {
		t.Fatal(err)
	}
	dangling, err := idx.DanglingLinks()
	if err != nil {
		t.Fatal(err)
	}
	if len(dangling) != 0 {
		t.Fatalf("dangling = %+v, want none (case, anchor, and display text normalized)", dangling)
	}
}
