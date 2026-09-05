package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
)

// Candidate is a note directory found on disk, worth suggesting as a source.
type Candidate struct {
	Path  string
	Label string
}

// DetectCandidates probes for note directories beyond the narrow, silent set
// detectSources uses for Defaults(). It is never called from Load()/Defaults()
// — discovering a personal vault (which may hold private content) should
// always be surfaced to a human for confirmation, never auto-added silently.
func DetectCandidates() []Candidate {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	var out []Candidate
	seen := map[string]bool{}
	add := func(path, label string) {
		if path == "" || seen[path] {
			return
		}
		if fi, err := os.Stat(path); err == nil && fi.IsDir() {
			seen[path] = true
			out = append(out, Candidate{Path: path, Label: label})
		}
	}

	add(filepath.Join(home, ".claude", "memory"), "Claude Code memory")
	add(filepath.Join(home, "dev", "second-brain"), "known folder")

	for _, v := range obsidianVaults(home) {
		add(v, "Obsidian vault")
	}

	for _, c := range []string{
		filepath.Join(home, "Documents", "Obsidian"),
		filepath.Join(home, "Obsidian"),
		filepath.Join(home, "logseq"),
		filepath.Join(home, "Documents", "Notes"),
		filepath.Join(home, "notes"),
		filepath.Join(home, "Library", "Mobile Documents", "iCloud~md~obsidian", "Documents"),
	} {
		add(c, "known folder")
	}

	return out
}

// obsidianVaults reads Obsidian's own vault registry rather than guessing
// folder names, returning every vault it knows about.
func obsidianVaults(home string) []string {
	var cfgPath string
	switch runtime.GOOS {
	case "darwin":
		cfgPath = filepath.Join(home, "Library", "Application Support", "obsidian", "obsidian.json")
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return nil
		}
		cfgPath = filepath.Join(appData, "obsidian", "obsidian.json")
	default:
		cfgPath = filepath.Join(home, ".config", "obsidian", "obsidian.json")
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil
	}
	var parsed struct {
		Vaults map[string]struct {
			Path string `json:"path"`
		} `json:"vaults"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil
	}
	var paths []string
	for _, v := range parsed.Vaults {
		if v.Path != "" {
			paths = append(paths, v.Path)
		}
	}
	sort.Strings(paths)
	return paths
}
