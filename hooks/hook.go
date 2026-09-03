// Package hooks implements the UserPromptSubmit entrypoint and its
// registration into Claude Code's settings.json.
package hooks

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/AgusRdz/local-mind/config"
	"github.com/AgusRdz/local-mind/index"
)

// hookInput is the JSON payload Claude Code sends a UserPromptSubmit hook.
type hookInput struct {
	Prompt        string `json:"prompt"`
	Cwd           string `json:"cwd"`
	HookEventName string `json:"hook_event_name"`
}

// hookOutput is the structured response that injects additional context.
type hookOutput struct {
	HookSpecificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

// RunHook reads a prompt from stdin, runs deterministic retrieval, and emits
// confidence-banded context on stdout with a trace on stderr. It never fails
// the prompt: on any error it emits nothing and exits 0.
func RunHook() {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return
	}
	var in hookInput
	if err := json.Unmarshal(raw, &in); err != nil || strings.TrimSpace(in.Prompt) == "" {
		return
	}

	cfg, err := config.Load()
	if err != nil {
		return
	}
	idx, err := index.Open()
	if err != nil {
		return
	}
	defer idx.Close()

	// Fetch a few extra so privacy/low-band filtering still leaves candidates.
	results, err := idx.Search(in.Prompt, cfg.Bands, cfg.Budget.MaxNotes+5)
	if err != nil {
		return
	}

	ctx, injected, candidates := build(results, cfg.Budget)

	// Trace every invocation (stderr + trace.log).
	trace(in.Prompt, injected, candidates)

	if ctx == "" {
		return
	}
	out := hookOutput{}
	out.HookSpecificOutput.HookEventName = "UserPromptSubmit"
	out.HookSpecificOutput.AdditionalContext = ctx
	data, err := json.Marshal(out)
	if err != nil {
		return
	}
	fmt.Fprintln(os.Stdout, string(data))
}

// build assembles the injected context under budget, honoring the privacy gate
// and confidence bands. Returns the context, the injected results, and any
// low-band candidates (for the trace only).
func build(results []index.Result, budget config.Budget) (ctx string, injected, candidates []index.Result) {
	var sb strings.Builder
	tokens := 0
	for _, r := range results {
		if r.Private {
			continue // privacy gate: never inject
		}
		if r.Band == index.BandLow {
			candidates = append(candidates, r)
			continue
		}
		if len(injected) >= budget.MaxNotes {
			continue
		}
		var block string
		switch r.Band {
		case index.BandBody:
			block = fmt.Sprintf("[from %s, updated %s]\n%s\n", rel(r.Path), age(r.Modified), r.Body)
		case index.BandDesc:
			block = fmt.Sprintf("[see %s, updated %s] %s\n", rel(r.Path), age(r.Modified), r.Description)
		}
		cost := estTokens(block)
		if tokens+cost > budget.MaxTokens && len(injected) > 0 {
			continue // would blow the budget; keep what we have
		}
		sb.WriteString(block)
		sb.WriteString("\n")
		tokens += cost
		injected = append(injected, r)
	}
	if len(injected) == 0 {
		return "", nil, candidates
	}
	return "Relevant notes from your second brain (via local-mind):\n\n" + sb.String(), injected, candidates
}

func trace(prompt string, injected, candidates []index.Result) {
	var lines []string
	for _, r := range injected {
		lines = append(lines, fmt.Sprintf("[local-mind: injected %s (age %s), band %s, score %.2f]",
			rel(r.Path), age(r.Modified), r.Band, r.Conf))
	}
	if len(injected) == 0 && len(candidates) > 0 {
		var cs []string
		for _, c := range candidates {
			cs = append(cs, fmt.Sprintf("%s (%.2f)", rel(c.Path), c.Conf))
		}
		lines = append(lines, "[local-mind: no confident match — candidates: "+strings.Join(cs, ", ")+"]")
	}
	if len(lines) == 0 {
		return
	}
	msg := strings.Join(lines, "\n")
	fmt.Fprintln(os.Stderr, msg)

	if tp, err := config.TracePath(); err == nil {
		if f, err := os.OpenFile(tp, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			defer f.Close()
			ts := time.Now().Format(time.RFC3339)
			for _, r := range injected {
				fmt.Fprintf(f, "%s\tinject\t%s\t%s\t%.4f\t%s\n", ts, r.Band, rel(r.Path), r.Conf, oneLine(prompt))
			}
			if len(injected) == 0 && len(candidates) > 0 {
				fmt.Fprintf(f, "%s\tmiss\t-\t-\t0\t%s\n", ts, oneLine(prompt))
			}
		}
	}
}

func estTokens(s string) int { return len(s)/4 + 1 }

func rel(path string) string {
	// Show the last two path segments for readability.
	p := strings.ReplaceAll(path, "\\", "/")
	parts := strings.Split(p, "/")
	if len(parts) <= 2 {
		return p
	}
	return strings.Join(parts[len(parts)-2:], "/")
}

func age(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := time.Since(t)
	switch {
	case d < 48*time.Hour:
		return "1d"
	case d < 14*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 60*24*time.Hour:
		return fmt.Sprintf("%dwk", int(d.Hours()/24/7))
	case d < 730*24*time.Hour:
		return fmt.Sprintf("%dmo", int(d.Hours()/24/30))
	default:
		return fmt.Sprintf("%dy", int(d.Hours()/24/365))
	}
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}
