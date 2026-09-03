// Command local-mind is a deterministic retrieval bridge between a plain-text
// second brain and Claude Code. See docs/PLAN.md and ROADMAP.md.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/AgusRdz/local-mind/config"
	"github.com/AgusRdz/local-mind/hooks"
	"github.com/AgusRdz/local-mind/index"
)

// version is set via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "hook":
		hooks.RunHook()
	case "rebuild":
		cmdRebuild(os.Args[2:])
	case "grep":
		cmdGrep(os.Args[2:])
	case "init":
		cmdInit(os.Args[2:])
	case "uninstall":
		fail(hooks.Uninstall())
	case "stats":
		cmdStats(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Printf("local-mind %s\n", version)
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func cmdRebuild(args []string) {
	incremental := hasFlag(args, "--incremental")
	cfg, err := config.Load()
	fail(err)
	if len(cfg.Sources) == 0 {
		cp, _ := config.Path()
		fmt.Fprintf(os.Stderr, "no sources configured. Edit %s and add note roots under `sources:`.\n", cp)
		os.Exit(1)
	}
	idx, err := index.Open()
	fail(err)
	defer idx.Close()

	start := time.Now()
	indexed, skipped, err := idx.Rebuild(cfg, incremental)
	fail(err)
	fmt.Printf("indexed %d note(s)", indexed)
	if incremental {
		fmt.Printf(", skipped %d unchanged", skipped)
	}
	fmt.Printf(" from %d source(s) in %s\n", len(cfg.Sources), time.Since(start).Round(time.Millisecond))
}

func cmdGrep(args []string) {
	query := strings.Join(positional(args), " ")
	if strings.TrimSpace(query) == "" {
		fmt.Fprintln(os.Stderr, "usage: local-mind grep \"<query>\"")
		os.Exit(1)
	}
	cfg, err := config.Load()
	fail(err)
	idx, err := index.Open()
	fail(err)
	defer idx.Close()

	results, err := idx.Search(query, cfg.Bands, 10)
	fail(err)
	if len(results) == 0 {
		fmt.Println("no match")
		return
	}
	for _, r := range results {
		fmt.Printf("%-6s  %.2f  %s\n", "["+r.Band+"]", r.Conf, r.Path)
		if r.Description != "" {
			fmt.Printf("        %s\n", truncate(r.Description, 100))
		}
	}
}

func cmdInit(args []string) {
	if hasFlag(args, "--status") {
		installed, path := hooks.IsInstalled()
		if installed {
			fmt.Printf("installed (UserPromptSubmit hook in %s)\n", path)
		} else {
			fmt.Printf("not installed (%s)\n", path)
		}
		return
	}
	fail(hooks.Install())
	fmt.Println("\nNext: run `local-mind rebuild` to build the index, then start a new Claude Code session.")
}

func cmdStats(args []string) {
	tp, err := config.TracePath()
	fail(err)
	f, err := os.Open(tp)
	if err != nil {
		fmt.Println("no trace data yet")
		return
	}
	defer f.Close()

	var injects, misses int
	bands := map[string]int{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Split(sc.Text(), "\t")
		if len(fields) < 2 {
			continue
		}
		switch fields[1] {
		case "inject":
			injects++
			if len(fields) >= 3 {
				bands[fields[2]]++
			}
		case "miss":
			misses++
		}
	}
	total := injects + misses
	fmt.Printf("%d injection(s) [body %d / desc %d], %d miss(es)\n", injects, bands["body"], bands["desc"], misses)
	if total > 0 {
		fmt.Printf("estimated miss rate: %.0f%%\n", 100*float64(misses)/float64(total))
	}
}

// --- helpers ---

func usage() {
	fmt.Print(`local-mind — deterministic retrieval bridge for Claude Code

usage:
  local-mind rebuild [--incremental]   build the FTS5 index from configured sources
  local-mind grep "<query>"            manual query (same matching as the hook)
  local-mind init [--status]           install the UserPromptSubmit hook (or show status)
  local-mind uninstall                 remove the hook
  local-mind stats                     injection/miss summary from the trace log
  local-mind hook                      hook entrypoint (reads stdin JSON)
  local-mind version                   print version

config: ~/.local-mind/config.yml   index: ~/.local-mind/index.db
`)
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func positional(args []string) []string {
	var out []string
	for _, a := range args {
		if !strings.HasPrefix(a, "--") {
			out = append(out, a)
		}
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func fail(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "local-mind: %v\n", err)
		os.Exit(1)
	}
}
