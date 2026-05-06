package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// --- Claude Code settings.json types ---

type claudeSettings struct {
	Hooks *claudeHooks `json:"hooks,omitempty"`
}

type claudeHooks struct {
	PreToolUse []claudePreToolEntry `json:"PreToolUse,omitempty"`
}

type claudePreToolEntry struct {
	Matcher string       `json:"matcher"`
	Hooks   []claudeHook `json:"hooks"`
}

type claudeHook struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// --- Gemini CLI settings.json types ---

type geminiSettings struct {
	Hooks *geminiHooks `json:"hooks,omitempty"`
}

type geminiHooks struct {
	BeforeTool string `json:"before_tool,omitempty"`
}

// Default paths.
func claudeSettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

func geminiSettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gemini", "settings.json"), nil
}

// InstallHooks adds vibecop hooks to the specified harness's settings.
func InstallHooks(harness string) error {
	switch harness {
	case HarnessClaude:
		return installClaudeHooks()
	case HarnessGemini:
		return installGeminiHooks()
	default:
		return fmt.Errorf("unsupported harness: %s", harness)
	}
}

func installClaudeHooks() error {
	path, err := claudeSettingsPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}

	raw := readRawJSON(path)

	// Parse existing hooks from the raw map.
	var hooks claudeHooks
	if raw["hooks"] != nil {
		if b, err := json.Marshal(raw["hooks"]); err == nil {
			json.Unmarshal(b, &hooks) //nolint:errcheck
		}
	}

	// Check if already installed.
	for _, e := range hooks.PreToolUse {
		if len(e.Hooks) > 0 && e.Hooks[0].Command == "vibecop hook" {
			return nil
		}
	}

	hooks.PreToolUse = append(hooks.PreToolUse, claudePreToolEntry{
		Matcher: "",
		Hooks:   []claudeHook{{Type: "command", Command: "vibecop hook"}},
	})
	raw["hooks"] = hooks
	return writeRawJSON(path, raw)
}

func installGeminiHooks() error {
	path, err := geminiSettingsPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}

	raw := readRawJSON(path)

	var hooks geminiHooks
	if raw["hooks"] != nil {
		if b, err := json.Marshal(raw["hooks"]); err == nil {
			json.Unmarshal(b, &hooks) //nolint:errcheck
		}
	}

	if hooks.BeforeTool == "vibecop hook" {
		return nil
	}

	hooks.BeforeTool = "vibecop hook"
	raw["hooks"] = hooks
	return writeRawJSON(path, raw)
}

// UninstallHooks removes vibecop hooks from the specified harness's settings.
func UninstallHooks(harness string) error {
	switch harness {
	case HarnessClaude:
		return uninstallClaudeHooks()
	case HarnessGemini:
		return uninstallGeminiHooks()
	default:
		return fmt.Errorf("unsupported harness: %s", harness)
	}
}

func uninstallClaudeHooks() error {
	path, err := claudeSettingsPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	raw := readRawJSON(path)

	var hooks claudeHooks
	if raw["hooks"] != nil {
		if b, err := json.Marshal(raw["hooks"]); err == nil {
			json.Unmarshal(b, &hooks) //nolint:errcheck
		}
	}

	filtered := slices.DeleteFunc(hooks.PreToolUse, func(e claudePreToolEntry) bool {
		return len(e.Hooks) > 0 && e.Hooks[0].Command == "vibecop hook"
	})

	if len(filtered) == len(hooks.PreToolUse) {
		return nil
	}

	if len(filtered) == 0 {
		delete(raw, "hooks")
	} else {
		hooks.PreToolUse = filtered
		raw["hooks"] = hooks
	}

	return writeRawJSON(path, raw)
}

func uninstallGeminiHooks() error {
	path, err := geminiSettingsPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	raw := readRawJSON(path)

	var hooks geminiHooks
	if raw["hooks"] != nil {
		if b, err := json.Marshal(raw["hooks"]); err == nil {
			json.Unmarshal(b, &hooks) //nolint:errcheck
		}
	}

	if hooks.BeforeTool != "vibecop hook" {
		return nil
	}

	delete(raw, "hooks")
	return writeRawJSON(path, raw)
}

// readRawJSON reads a JSON file as a raw map, preserving all keys.
// Returns an empty map if the file doesn't exist or is malformed.
func readRawJSON(path string) map[string]any {
	raw := map[string]any{}
	data, err := os.ReadFile(path)
	if err != nil {
		return raw
	}
	json.Unmarshal(data, &raw) //nolint:errcheck
	return raw
}

// writeRawJSON writes v to path with standard indentation.
func writeRawJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
