package hooks

import (
	"strings"
	"testing"

	"github.com/AgusRdz/local-mind/config"
	"github.com/AgusRdz/local-mind/index"
)

// Regression: a single note whose body exceeds the token budget must be
// truncated, never dumped whole (previously the first note bypassed the cap).
func TestBuild_TruncatesOversizedBody(t *testing.T) {
	huge := strings.Repeat("alpha beta gamma ", 4000) // ~68 KB
	r := index.Result{Name: "big", Path: "/vault/big.md", Body: huge, Band: index.BandBody, Conf: 0.9}
	budget := config.Budget{MaxNotes: 3, MaxTokens: 50}

	ctx, injected, _ := build([]index.Result{r}, budget)

	if len(injected) != 1 {
		t.Fatalf("want 1 injected note, got %d", len(injected))
	}
	if !strings.Contains(ctx, "truncated") {
		t.Errorf("expected truncation marker in output:\n%s", ctx)
	}
	if len(ctx) >= len(huge) {
		t.Errorf("body was not truncated: ctx %d bytes vs body %d bytes", len(ctx), len(huge))
	}
	// Injected content must stay within budget (+ small slack for the fixed
	// prefix line, which is not counted against the note budget).
	if got := estTokens(ctx); got > budget.MaxTokens+40 {
		t.Errorf("output ~%d tokens exceeds budget %d (+slack)", got, budget.MaxTokens)
	}
}

// A normal note under budget is injected verbatim, without a truncation marker.
func TestBuild_SmallBodyNotTruncated(t *testing.T) {
	r := index.Result{Name: "s", Path: "/vault/s.md", Body: "short body content", Band: index.BandBody, Conf: 0.9}
	ctx, injected, _ := build([]index.Result{r}, config.Budget{MaxNotes: 3, MaxTokens: 1200})
	if len(injected) != 1 {
		t.Fatalf("want 1 injected, got %d", len(injected))
	}
	if strings.Contains(ctx, "truncated") {
		t.Error("small body should not be truncated")
	}
	if !strings.Contains(ctx, "short body content") {
		t.Error("body content missing")
	}
}

// Budget cap holds across multiple notes: total never exceeds MaxTokens.
func TestBuild_MultiNoteBudgetHolds(t *testing.T) {
	big := strings.Repeat("x ", 2000)
	results := []index.Result{
		{Name: "a", Path: "/v/a.md", Body: big, Band: index.BandBody, Conf: 0.9},
		{Name: "b", Path: "/v/b.md", Body: big, Band: index.BandBody, Conf: 0.8},
	}
	budget := config.Budget{MaxNotes: 3, MaxTokens: 60}
	ctx, _, _ := build(results, budget)
	if got := estTokens(ctx); got > budget.MaxTokens+40 {
		t.Errorf("output ~%d tokens exceeds budget %d (+slack)", got, budget.MaxTokens)
	}
}

// Private notes are never injected regardless of band.
func TestBuild_PrivateNeverInjected(t *testing.T) {
	r := index.Result{Name: "sec", Path: "/v/sec.md", Body: "secret", Band: index.BandBody, Private: true, Conf: 0.9}
	ctx, injected, _ := build([]index.Result{r}, config.Budget{MaxNotes: 3, MaxTokens: 1200})
	if len(injected) != 0 || ctx != "" {
		t.Errorf("private note was injected: %q", ctx)
	}
}
