package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// cmdSuggestAliases proposes aliases (and a description if missing) for a note
// using the `claude` CLI at authoring time, then writes them into frontmatter
// on confirmation. Retrieval stays LLM-free — this touches the write path only.
func cmdSuggestAliases(args []string) {
	dryRun := hasFlag(args, "--dry-run")
	yes := hasFlag(args, "--yes")
	model := flagValue(args, "--model")

	path := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--model" { // consumes the next token as its value
			i++
			continue
		}
		if strings.HasPrefix(a, "--") {
			continue
		}
		path = a
		break
	}
	if path == "" {
		fmt.Fprintln(os.Stderr, "usage: local-mind suggest-aliases <path> [--dry-run] [--yes] [--model <name>]")
		os.Exit(1)
	}

	raw, err := os.ReadFile(path)
	fail(err)

	name, desc, aliases, body := parseForSuggest(raw, path)

	suggestion, err := askModel(name, desc, aliases, body, model)
	fail(err)
	if len(suggestion.Aliases) == 0 {
		fmt.Println("model proposed no aliases; nothing to do")
		return
	}

	setDesc := strings.TrimSpace(desc) == "" && strings.TrimSpace(suggestion.Description) != ""

	fmt.Printf("note: %s\n", path)
	fmt.Printf("  current aliases:  %s\n", orNone(aliases))
	fmt.Printf("  proposed aliases: %s\n", strings.Join(suggestion.Aliases, ", "))
	if setDesc {
		fmt.Printf("  proposed description (was empty): %s\n", suggestion.Description)
	}

	if dryRun {
		return
	}
	if !yes && !confirm("apply to frontmatter?") {
		fmt.Println("aborted")
		return
	}

	descToWrite := ""
	if setDesc {
		descToWrite = suggestion.Description
	}
	updated, err := upsertFrontmatter(raw, path, suggestion.Aliases, descToWrite)
	fail(err)
	fail(os.WriteFile(path, updated, 0o644))
	fmt.Println("updated frontmatter — run `local-mind rebuild --incremental` to index it")
}

type suggestion struct {
	Aliases     []string `json:"aliases"`
	Description string   `json:"description"`
}

// askModel invokes the `claude` CLI headlessly to propose aliases.
func askModel(name, desc, aliases, body, model string) (suggestion, error) {
	claude, err := exec.LookPath("claude")
	if err != nil {
		return suggestion{}, fmt.Errorf("`claude` CLI not found on PATH — needed to generate aliases")
	}
	if len(body) > 1500 {
		body = body[:1500]
	}
	prompt := fmt.Sprintf(`You are building a keyword search index for a personal notes vault.
Propose 3-7 SHORT alias phrases: alternate search terms or synonyms someone might
type to find this note, that are NOT already obvious from its title. Favor the
vocabulary a searcher would use; do not restate the title. Also propose a concise
one-line description (max 120 chars) if the current one is weak or missing.

Title: %s
Current description: %s
Current aliases: %s
Body (truncated):
%s

Output ONLY a JSON object, no prose and no code fence:
{"aliases": ["phrase one", "phrase two"], "description": "..."}`,
		name, orNone(desc), orNone(aliases), body)

	cmdArgs := []string{"-p"}
	if model != "" {
		cmdArgs = append(cmdArgs, "--model", model)
	}
	cmdArgs = append(cmdArgs, prompt)

	out, err := exec.Command(claude, cmdArgs...).Output()
	if err != nil {
		return suggestion{}, fmt.Errorf("claude CLI failed: %w", err)
	}
	return parseSuggestion(out)
}

// parseSuggestion extracts the first {...} JSON object from model output.
func parseSuggestion(out []byte) (suggestion, error) {
	s := string(out)
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end < start {
		return suggestion{}, fmt.Errorf("no JSON object in model output")
	}
	var sug suggestion
	if err := json.Unmarshal([]byte(s[start:end+1]), &sug); err != nil {
		return suggestion{}, fmt.Errorf("parse model JSON: %w", err)
	}
	// trim + drop empties
	var clean []string
	for _, a := range sug.Aliases {
		if t := strings.TrimSpace(a); t != "" {
			clean = append(clean, t)
		}
	}
	sug.Aliases = clean
	sug.Description = strings.TrimSpace(sug.Description)
	return sug, nil
}

// parseForSuggest returns (name, description, aliasesCSV, body) for prompting.
func parseForSuggest(raw []byte, path string) (name, desc, aliases, body string) {
	fm, b := splitFM(raw)
	body = strings.TrimSpace(string(b))
	for _, line := range strings.Split(fm, "\n") {
		l := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(l, "name:"):
			name = strings.TrimSpace(strings.TrimPrefix(l, "name:"))
		case strings.HasPrefix(l, "description:"):
			desc = strings.TrimSpace(strings.TrimPrefix(l, "description:"))
		case strings.HasPrefix(l, "aliases:"):
			aliases = strings.TrimSpace(strings.TrimPrefix(l, "aliases:"))
			aliases = strings.Trim(aliases, "[]")
		}
	}
	if name == "" {
		base := filepath.Base(path)
		name = strings.TrimSuffix(base, filepath.Ext(base))
	}
	return name, desc, aliases, body
}

// upsertFrontmatter rewrites a note's frontmatter, replacing any existing
// aliases with the given ones (inline flow list), and setting description only
// when descToWrite is non-empty. Body and other frontmatter keys are preserved.
func upsertFrontmatter(raw []byte, path string, aliases []string, descToWrite string) ([]byte, error) {
	aliasLine := "aliases: [" + strings.Join(quoteAll(aliases), ", ") + "]"

	fm, body := splitFM(raw)
	if fm == "" {
		// No frontmatter — create one.
		base := filepath.Base(path)
		nameVal := strings.TrimSuffix(base, filepath.Ext(base))
		var b strings.Builder
		b.WriteString("---\n")
		b.WriteString("name: " + nameVal + "\n")
		if descToWrite != "" {
			b.WriteString("description: " + descToWrite + "\n")
		}
		b.WriteString(aliasLine + "\n")
		b.WriteString("---\n\n")
		b.Write(raw)
		return []byte(b.String()), nil
	}

	lines := strings.Split(fm, "\n")
	var kept []string
	skipBlock := false
	haveDesc := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Drop an existing aliases key (inline or block form).
		if strings.HasPrefix(trimmed, "aliases:") {
			// If block form (nothing after the colon), skip following list items.
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "aliases:"))
			skipBlock = rest == ""
			continue
		}
		if skipBlock {
			if strings.HasPrefix(trimmed, "- ") || (line != "" && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t"))) {
				continue // still inside the block list
			}
			skipBlock = false
		}
		if strings.HasPrefix(trimmed, "description:") {
			haveDesc = true
		}
		kept = append(kept, line)
	}

	// Drop trailing blank lines in the frontmatter block.
	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}
	if descToWrite != "" && !haveDesc {
		kept = append(kept, "description: "+descToWrite)
	}
	kept = append(kept, aliasLine)

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(strings.Join(kept, "\n"))
	b.WriteString("\n---\n")
	b.WriteString(body)
	return []byte(b.String()), nil
}

// splitFM returns (frontmatterInner, body). frontmatterInner excludes the
// delimiter lines; body includes everything after the closing delimiter.
func splitFM(raw []byte) (string, string) {
	s := strings.TrimLeft(string(raw), "\uFEFF \t\r\n")
	if !strings.HasPrefix(s, "---") {
		return "", string(raw)
	}
	// position after first delimiter line
	nl := strings.IndexByte(s, '\n')
	if nl < 0 {
		return "", string(raw)
	}
	rest := s[nl+1:]
	// find closing delimiter line
	lines := strings.Split(rest, "\n")
	for i, line := range lines {
		if strings.TrimRight(line, "\r") == "---" {
			fm := strings.Join(lines[:i], "\n")
			body := strings.Join(lines[i+1:], "\n")
			return fm, body
		}
	}
	return "", string(raw)
}

func quoteAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = `"` + strings.ReplaceAll(s, `"`, `'`) + `"`
	}
	return out
}

func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(none)"
	}
	return s
}

func confirm(question string) bool {
	fmt.Printf("%s [y/N] ", question)
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

func flagValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
