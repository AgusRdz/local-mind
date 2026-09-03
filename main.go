// Command local-mind is a deterministic retrieval bridge between a plain-text
// second brain and Claude Code. See docs/PLAN.md and ROADMAP.md.
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
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
	case "bad":
		cmdBad()
	case "doctor":
		cmdDoctor()
	case "config":
		cmdConfig(os.Args[2:])
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

// --- stats ---

type traceRec struct {
	ts     time.Time
	kind   string // inject | miss | bad
	band   string
	path   string
	conf   float64
	prompt string
}

func cmdStats(args []string) {
	since := time.Duration(0)
	for i := 0; i < len(args); i++ {
		if args[i] == "--since" && i+1 < len(args) {
			d, err := parseSince(args[i+1])
			fail(err)
			since = d
		}
	}
	recs, err := readTrace()
	if err != nil {
		fmt.Println("no trace data yet — the hook logs here once it runs on prompts")
		return
	}
	cutoff := time.Time{}
	if since > 0 {
		cutoff = time.Now().Add(-since)
	}

	var injects, misses, bads int
	bands := map[string]int{}
	var confs []float64
	for _, r := range recs {
		if !cutoff.IsZero() && r.ts.Before(cutoff) {
			continue
		}
		switch r.kind {
		case "inject":
			injects++
			bands[r.band]++
			confs = append(confs, r.conf)
		case "miss":
			misses++
		case "bad":
			bads++
		}
	}

	window := "all time"
	if since > 0 {
		window = "last " + args[indexOf(args, "--since")+1]
	}
	fmt.Printf("local-mind stats (%s)\n", window)
	fmt.Printf("  injections: %d  (body %d / desc %d)\n", injects, bands[index.BandBody], bands[index.BandDesc])
	fmt.Printf("  misses:     %d  (no confident match)\n", misses)
	fmt.Printf("  flagged bad: %d  (marked unhelpful via `local-mind bad`)\n", bads)

	total := injects + misses
	if total > 0 {
		effMiss := float64(misses+bads) / float64(total) * 100
		fmt.Printf("  miss rate:  %.0f%%  (misses + flagged / total)\n", effMiss)
	}
	if len(confs) > 0 {
		sort.Float64s(confs)
		fmt.Printf("  confidence: p50 %.2f  p90 %.2f  min %.2f  max %.2f\n",
			percentile(confs, 0.5), percentile(confs, 0.9), confs[0], confs[len(confs)-1])
		fmt.Println("  histogram:")
		printHistogram(confs)
	}
}

func cmdBad() {
	recs, err := readTrace()
	if err != nil || len(recs) == 0 {
		fmt.Println("no injections to flag yet")
		return
	}
	var last *traceRec
	for i := len(recs) - 1; i >= 0; i-- {
		if recs[i].kind == "inject" {
			last = &recs[i]
			break
		}
	}
	if last == nil {
		fmt.Println("no injection found to flag")
		return
	}
	tp, err := config.TracePath()
	fail(err)
	f, err := os.OpenFile(tp, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	fail(err)
	defer f.Close()
	fmt.Fprintf(f, "%s\tbad\t-\t%s\t0\t%s\n", time.Now().Format(time.RFC3339), last.path, last.prompt)
	fmt.Printf("flagged last injection as unhelpful: %s\n", last.path)
}

// --- doctor ---

func cmdDoctor() {
	failed := false
	check := func(ok bool, warn bool, label, detail string) {
		mark := "PASS"
		if !ok {
			if warn {
				mark = "WARN"
			} else {
				mark = "FAIL"
				failed = true
			}
		}
		if detail != "" {
			fmt.Printf("  [%s] %s — %s\n", mark, label, detail)
		} else {
			fmt.Printf("  [%s] %s\n", mark, label)
		}
	}

	fmt.Printf("local-mind doctor (v%s)\n", version)

	exe, _ := os.Executable()
	check(exe != "", true, "binary", exe)

	installed, sp := hooks.IsInstalled()
	check(installed, false, "hook registered", sp)

	cfg, cfgErr := config.Load()
	cp, _ := config.Path()
	check(cfgErr == nil, false, "config parses", cp)

	check(len(cfg.Sources) > 0, false, "sources configured", fmt.Sprintf("%d", len(cfg.Sources)))
	for _, s := range cfg.Sources {
		fi, err := os.Stat(s)
		check(err == nil && fi.IsDir(), true, "source", s)
	}

	dbPath, _ := config.DBPath()
	if fi, err := os.Stat(dbPath); err == nil {
		idx, ierr := index.Open()
		if ierr == nil {
			defer idx.Close()
			n, _ := idx.Count()
			p, _ := idx.PrivateCount()
			age := time.Since(fi.ModTime()).Round(time.Minute)
			check(n > 0, n == 0, "index built", fmt.Sprintf("%d notes (%d private), rebuilt %s ago", n, p, age))
		} else {
			check(false, false, "index readable", ierr.Error())
		}
	} else {
		check(false, false, "index built", "run `local-mind rebuild`")
	}

	fmt.Println()
	if failed {
		fmt.Println("doctor: problems found (see FAIL above)")
		os.Exit(1)
	}
	fmt.Println("doctor: all good")
}

// --- config ---

func cmdConfig(args []string) {
	sub := "show"
	if len(args) > 0 {
		sub = args[0]
	}
	cp, _ := config.Path()
	switch sub {
	case "show", "":
		cfg, err := config.Load()
		fail(err)
		fmt.Printf("# %s\n", cp)
		printConfig(cfg)
	case "path":
		fmt.Println(cp)
	case "edit":
		if _, err := config.Load(); err != nil { // ensure it exists/valid
			fail(err)
		}
		fail(openEditor(cp))
	case "set":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: local-mind config set <very_high|high|max_notes|max_tokens> <value>")
			os.Exit(1)
		}
		cfg, err := config.Load()
		fail(err)
		fail(cfg.Set(args[1], args[2]))
		fail(config.Save(cfg))
		fmt.Printf("set %s = %s\n", args[1], args[2])
	case "add-source":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: local-mind config add-source <path>")
			os.Exit(1)
		}
		cfg, err := config.Load()
		fail(err)
		if cfg.AddSource(args[1]) {
			fail(config.Save(cfg))
			fmt.Printf("added source: %s\n(run `local-mind rebuild` to index it)\n", args[1])
		} else {
			fmt.Println("source already present")
		}
	case "add-ignore":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: local-mind config add-ignore <glob>")
			os.Exit(1)
		}
		cfg, err := config.Load()
		fail(err)
		if cfg.AddIgnore(args[1]) {
			fail(config.Save(cfg))
			fmt.Printf("added ignore: %s\n(run `local-mind rebuild` to apply)\n", args[1])
		} else {
			fmt.Println("ignore glob already present")
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown config subcommand: %s\n", sub)
		fmt.Fprintln(os.Stderr, "  show | path | edit | set <key> <val> | add-source <path> | add-ignore <glob>")
		os.Exit(1)
	}
}

// --- helpers ---

func readTrace() ([]traceRec, error) {
	tp, err := config.TracePath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(tp)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var recs []traceRec
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Split(sc.Text(), "\t")
		if len(fields) < 2 {
			continue
		}
		r := traceRec{kind: fields[1]}
		r.ts, _ = time.Parse(time.RFC3339, fields[0])
		if len(fields) >= 3 {
			r.band = fields[2]
		}
		if len(fields) >= 4 {
			r.path = fields[3]
		}
		if len(fields) >= 5 {
			r.conf, _ = strconv.ParseFloat(fields[4], 64)
		}
		if len(fields) >= 6 {
			r.prompt = fields[5]
		}
		recs = append(recs, r)
	}
	return recs, sc.Err()
}

func parseSince(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	unit := s[len(s)-1]
	if unit == 'd' || unit == 'w' {
		n, err := strconv.Atoi(s[:len(s)-1])
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		mult := time.Hour * 24
		if unit == 'w' {
			mult *= 7
		}
		return time.Duration(n) * mult, nil
	}
	return time.ParseDuration(s)
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p * float64(len(sorted)-1))
	return sorted[idx]
}

func printHistogram(confs []float64) {
	// Five fixed buckets across 0..1.
	buckets := make([]int, 5)
	labels := []string{"0.0-0.2", "0.2-0.4", "0.4-0.6", "0.6-0.8", "0.8-1.0"}
	max := 0
	for _, c := range confs {
		b := int(c * 5)
		if b > 4 {
			b = 4
		}
		buckets[b]++
		if buckets[b] > max {
			max = buckets[b]
		}
	}
	for i, n := range buckets {
		bar := ""
		if max > 0 {
			bar = strings.Repeat("█", n*20/max)
		}
		fmt.Printf("    %s  %2d %s\n", labels[i], n, bar)
	}
}

func printConfig(cfg config.Config) {
	fmt.Println("sources:")
	for _, s := range cfg.Sources {
		fmt.Printf("  - %s\n", s)
	}
	fmt.Println("ignore:")
	for _, g := range cfg.Ignore {
		fmt.Printf("  - %s\n", g)
	}
	fmt.Printf("bands:   very_high=%.2f  high=%.2f\n", cfg.Bands.VeryHigh, cfg.Bands.High)
	fmt.Printf("budget:  max_notes=%d  max_tokens=%d\n", cfg.Budget.MaxNotes, cfg.Budget.MaxTokens)
}

func openEditor(path string) error {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		if runtime.GOOS == "windows" {
			editor = "notepad"
		} else {
			editor = "vi"
		}
	}
	cmd := exec.Command(editor, path)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func usage() {
	fmt.Print(`local-mind — deterministic retrieval bridge for Claude Code

usage:
  local-mind rebuild [--incremental]   build the FTS5 index from configured sources
  local-mind grep "<query>"            manual query (same matching as the hook)
  local-mind init [--status]           install the UserPromptSubmit hook (or show status)
  local-mind uninstall                 remove the hook
  local-mind doctor                    health check (hook, sources, index, config)
  local-mind stats [--since 7d]        injection/miss summary + confidence histogram
  local-mind bad                       flag the last injection as unhelpful
  local-mind config <cmd>              show | path | edit | set <k> <v> | add-source | add-ignore
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

func indexOf(args []string, s string) int {
	for i, a := range args {
		if a == s {
			return i
		}
	}
	return -1
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
