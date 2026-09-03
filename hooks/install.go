package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const binaryName = "local-mind"

// Install registers the UserPromptSubmit hook in ~/.claude/settings.json.
func Install() error {
	settingsPath, err := settingsPath()
	if err != nil {
		return err
	}
	cmd, err := ExpectedHookCommand()
	if err != nil {
		return err
	}
	if err := installWithCommand(settingsPath, cmd); err != nil {
		return err
	}
	fmt.Printf("local-mind hook installed in %s\n", settingsPath)
	return nil
}

// Uninstall removes the hook from settings.json.
func Uninstall() error {
	settingsPath, err := settingsPath()
	if err != nil {
		return err
	}
	if err := uninstallFrom(settingsPath); err != nil {
		return err
	}
	fmt.Printf("local-mind hook removed from %s\n", settingsPath)
	return nil
}

// IsInstalled reports whether the hook is registered, and where.
func IsInstalled() (bool, string) {
	sp, err := settingsPath()
	if err != nil {
		return false, ""
	}
	settings, err := readSettings(sp)
	if err != nil {
		return false, sp
	}
	for _, entry := range userPromptSubmit(settings) {
		if hasLocalMindHook(entry) {
			return true, sp
		}
	}
	return false, sp
}

func settingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

// ExpectedHookCommand returns the hook command for the current binary.
func ExpectedHookCommand() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("get executable: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		resolved = exe
	}
	return fmt.Sprintf(`"%s" hook`, strings.ReplaceAll(resolved, "\\", "/")), nil
}

func readSettings(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]any{}, nil
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return settings, nil
}

func writeSettings(path string, settings map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

// userPromptSubmit returns the UserPromptSubmit entry list from settings.
func userPromptSubmit(settings map[string]any) []map[string]any {
	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := hooksMap["UserPromptSubmit"].([]any)
	if !ok {
		return nil
	}
	var out []map[string]any
	for _, e := range raw {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func hasLocalMindHook(entry map[string]any) bool {
	arr, ok := entry["hooks"].([]any)
	if !ok {
		return false
	}
	for _, h := range arr {
		if hm, ok := h.(map[string]any); ok && isLocalMindHook(hm) {
			return true
		}
	}
	return false
}

func isLocalMindHook(h map[string]any) bool {
	cmd, ok := h["command"].(string)
	if !ok {
		return false
	}
	return strings.Contains(cmd, binaryName) && strings.HasSuffix(cmd, " hook")
}

func installWithCommand(settingsPath, cmd string) error {
	settings, err := readSettings(settingsPath)
	if err != nil {
		return err
	}

	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		hooksMap = map[string]any{}
		settings["hooks"] = hooksMap
	}

	upsRaw, _ := hooksMap["UserPromptSubmit"].([]any)
	entry := map[string]any{"type": "command", "command": cmd}

	// Update in place if a local-mind hook already exists.
	for _, e := range upsRaw {
		em, ok := e.(map[string]any)
		if !ok {
			continue
		}
		arr, ok := em["hooks"].([]any)
		if !ok {
			continue
		}
		for i, h := range arr {
			if hm, ok := h.(map[string]any); ok && isLocalMindHook(hm) {
				arr[i] = entry
				em["hooks"] = arr
				hooksMap["UserPromptSubmit"] = upsRaw
				return writeSettings(settingsPath, settings)
			}
		}
	}

	// Otherwise append a new entry.
	upsRaw = append(upsRaw, map[string]any{
		"hooks": []any{entry},
	})
	hooksMap["UserPromptSubmit"] = upsRaw
	return writeSettings(settingsPath, settings)
}

func uninstallFrom(settingsPath string) error {
	settings, err := readSettings(settingsPath)
	if err != nil {
		return err
	}
	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		return nil
	}
	upsRaw, ok := hooksMap["UserPromptSubmit"].([]any)
	if !ok {
		return nil
	}

	newUPS := make([]any, 0, len(upsRaw))
	for _, e := range upsRaw {
		em, ok := e.(map[string]any)
		if !ok {
			newUPS = append(newUPS, e)
			continue
		}
		arr, ok := em["hooks"].([]any)
		if !ok {
			newUPS = append(newUPS, e)
			continue
		}
		newHooks := make([]any, 0, len(arr))
		for _, h := range arr {
			if hm, ok := h.(map[string]any); ok && isLocalMindHook(hm) {
				continue
			}
			newHooks = append(newHooks, h)
		}
		if len(newHooks) > 0 {
			em["hooks"] = newHooks
			newUPS = append(newUPS, em)
		}
	}

	if len(newUPS) > 0 {
		hooksMap["UserPromptSubmit"] = newUPS
	} else {
		delete(hooksMap, "UserPromptSubmit")
	}
	if len(hooksMap) == 0 {
		delete(settings, "hooks")
	} else {
		settings["hooks"] = hooksMap
	}
	return writeSettings(settingsPath, settings)
}
