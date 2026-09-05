package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestObsidianVaults_ParsesRegistry(t *testing.T) {
	home := t.TempDir()
	vault := filepath.Join(home, "MyVault")
	if err := os.MkdirAll(vault, 0o700); err != nil {
		t.Fatal(err)
	}
	cfgDir := filepath.Join(home, ".config", "obsidian")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	registry := `{"vaults":{"abc123":{"path":"` + vault + `","ts":1,"open":true}}}`
	if err := os.WriteFile(filepath.Join(cfgDir, "obsidian.json"), []byte(registry), 0o600); err != nil {
		t.Fatal(err)
	}

	got := obsidianVaults(home)
	if len(got) != 1 || got[0] != vault {
		t.Fatalf("obsidianVaults() = %v, want [%s]", got, vault)
	}
}

func TestObsidianVaults_NoRegistry(t *testing.T) {
	home := t.TempDir()
	if got := obsidianVaults(home); got != nil {
		t.Fatalf("obsidianVaults() = %v, want nil", got)
	}
}

func TestObsidianVaults_MalformedRegistry(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "obsidian")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "obsidian.json"), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := obsidianVaults(home); got != nil {
		t.Fatalf("obsidianVaults() = %v, want nil on malformed JSON", got)
	}
}

func TestDetectCandidates_SkipsNonexistentAndDedupes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	memDir := filepath.Join(home, ".claude", "memory")
	if err := os.MkdirAll(memDir, 0o700); err != nil {
		t.Fatal(err)
	}

	got := DetectCandidates()
	if len(got) != 1 {
		t.Fatalf("DetectCandidates() = %+v, want exactly the one existing dir", got)
	}
	if got[0].Path != memDir || got[0].Label != "Claude Code memory" {
		t.Errorf("candidate = %+v, want {%s, Claude Code memory}", got[0], memDir)
	}
}
