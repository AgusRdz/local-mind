package index

import (
	"testing"
	"time"
)

func TestParseNote_FullFrontmatter(t *testing.T) {
	src := []byte(`---
name: worktree-runner
description: Thin Go CLI wrapping git worktrees
aliases: [parallel branches, concurrent worktrees]
private: true
date: 2026-07-01
---
# Overview
- Planned as ~100 lines, one command per parallel branch
`)
	n := ParseNote("/notes/worktree.md", src, time.Now())

	if n.Name != "worktree-runner" {
		t.Errorf("Name = %q", n.Name)
	}
	if n.Description != "Thin Go CLI wrapping git worktrees" {
		t.Errorf("Description = %q", n.Description)
	}
	if n.Aliases != "parallel branches concurrent worktrees" {
		t.Errorf("Aliases = %q", n.Aliases)
	}
	if !n.Private {
		t.Error("Private = false, want true")
	}
	if n.Modified.Format("2006-01-02") != "2026-07-01" {
		t.Errorf("Modified = %v, want date from frontmatter", n.Modified)
	}
	if n.Headings != "Overview" {
		t.Errorf("Headings = %q", n.Headings)
	}
}

func TestParseNote_NoFrontmatter(t *testing.T) {
	mtime := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	n := ParseNote("/notes/plain.md", []byte("just some body text\nsecond line"), mtime)

	if n.Name != "plain" {
		t.Errorf("Name = %q, want filename fallback", n.Name)
	}
	if n.Description != "just some body text" {
		t.Errorf("Description = %q, want first prose line", n.Description)
	}
	if !n.Modified.Equal(mtime) {
		t.Errorf("Modified = %v, want file mtime fallback", n.Modified)
	}
	if n.Private {
		t.Error("Private = true, want false")
	}
}

func TestParseNote_AliasesAsCommaString(t *testing.T) {
	src := []byte("---\nname: x\naliases: foo, bar baz\n---\nbody")
	n := ParseNote("/notes/x.md", src, time.Now())
	if n.Aliases != "foo bar baz" {
		t.Errorf("Aliases = %q, want tolerant comma-string parse", n.Aliases)
	}
}

func TestBuildMatch(t *testing.T) {
	got := buildMatch("why did we move the retry logic out of the client SDK")
	// stopwords (why/did/out/the) dropped, terms quoted and OR-joined
	want := `"move" OR "retry" OR "logic" OR "client" OR "sdk"`
	if got != want {
		t.Errorf("buildMatch = %q\nwant %q", got, want)
	}
	if buildMatch("a of to") != "" {
		t.Error("all-stopword query should yield empty match")
	}
}
