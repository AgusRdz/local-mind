package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAddSource_Valid(t *testing.T) {
	dir := t.TempDir()
	c := &Config{}
	added, err := c.AddSource(dir)
	if err != nil || !added {
		t.Fatalf("AddSource(%s) = (%v, %v), want (true, nil)", dir, added, err)
	}
	if len(c.Sources) != 1 || c.Sources[0] != dir {
		t.Errorf("Sources = %v, want [%s]", c.Sources, dir)
	}
}

func TestAddSource_AlreadyPresent(t *testing.T) {
	dir := t.TempDir()
	c := &Config{Sources: []string{dir}}
	added, err := c.AddSource(dir)
	if err != nil || added {
		t.Fatalf("AddSource(%s) = (%v, %v), want (false, nil)", dir, added, err)
	}
}

func TestAddSource_NonexistentPath(t *testing.T) {
	c := &Config{}
	added, err := c.AddSource(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil || added {
		t.Fatalf("AddSource(missing) = (%v, %v), want (false, error)", added, err)
	}
}

func TestAddSource_PathIsFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "note.md")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &Config{}
	added, err := c.AddSource(file)
	if err == nil || added {
		t.Fatalf("AddSource(file) = (%v, %v), want (false, error)", added, err)
	}
}
