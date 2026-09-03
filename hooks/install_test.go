package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestInstallWithCommand_FreshFile(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "settings.json")
	cmd := `"/usr/local/bin/local-mind" hook`
	if err := installWithCommand(sp, cmd); err != nil {
		t.Fatal(err)
	}

	settings := readJSON(t, sp)
	entries := userPromptSubmit(settings)
	if len(entries) != 1 || !hasLocalMindHook(entries[0]) {
		t.Fatalf("hook not registered: %+v", settings)
	}
}

func TestInstallWithCommand_Idempotent(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "settings.json")
	_ = installWithCommand(sp, `"/old/local-mind" hook`)
	_ = installWithCommand(sp, `"/new/local-mind" hook`)

	entries := userPromptSubmit(readJSON(t, sp))
	count := 0
	for _, e := range entries {
		arr, _ := e["hooks"].([]any)
		for _, h := range arr {
			if hm, ok := h.(map[string]any); ok && isLocalMindHook(hm) {
				count++
				if hm["command"] != `"/new/local-mind" hook` {
					t.Errorf("command = %v, want updated path", hm["command"])
				}
			}
		}
	}
	if count != 1 {
		t.Errorf("local-mind hook count = %d, want 1 (updated in place, not duplicated)", count)
	}
}

func TestInstallPreservesForeignHooks(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "settings.json")
	seed := `{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"other-tool run"}]}]}}`
	if err := os.WriteFile(sp, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := installWithCommand(sp, `"/bin/local-mind" hook`); err != nil {
		t.Fatal(err)
	}
	// Uninstall should remove only ours, leaving the foreign hook intact.
	if err := uninstallFrom(sp); err != nil {
		t.Fatal(err)
	}
	settings := readJSON(t, sp)
	entries := userPromptSubmit(settings)
	foundForeign := false
	for _, e := range entries {
		arr, _ := e["hooks"].([]any)
		for _, h := range arr {
			hm, _ := h.(map[string]any)
			if hm["command"] == "other-tool run" {
				foundForeign = true
			}
			if isLocalMindHook(hm) {
				t.Error("local-mind hook should have been removed")
			}
		}
	}
	if !foundForeign {
		t.Error("foreign hook was clobbered")
	}
}

func TestUninstall_EmptyIsNoop(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "settings.json")
	if err := uninstallFrom(sp); err != nil {
		t.Errorf("uninstall on missing file should be a no-op, got %v", err)
	}
}
