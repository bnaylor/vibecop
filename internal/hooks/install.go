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

	// Load or create settings.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	entry := claudePreToolEntry{
		Matcher: "",
		Hooks: []claudeHook{{
			Type:    "command",
			Command: "vibecop hook",
		}},
	}

	cfg := claudeSettings{}
	existing := readJSONFile(path, &cfg)

	if cfg.Hooks == nil {
		cfg.Hooks = &claudeHooks{}
	}

	// Check if already installed.
	for _, e := range cfg.Hooks.PreToolUse {
		if len(e.Hooks) > 0 && e.Hooks[0].Command == "vibecop hook" {
			return nil // already installed, idempotent
		}
	}

	cfg.Hooks.PreToolUse = append(cfg.Hooks.PreToolUse, entry)
	return writeJSONFile(path, cfg, existing)
}

func installGeminiHooks() error {
	path, err := geminiSettingsPath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	cfg := geminiSettings{}
	existing := readJSONFile(path, &cfg)

	if cfg.Hooks == nil {
		cfg.Hooks = &geminiHooks{}
	}

	if cfg.Hooks.BeforeTool == "vibecop hook" {
		return nil // already installed
	}

	cfg.Hooks.BeforeTool = "vibecop hook"
	return writeJSONFile(path, cfg, existing)
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
		return nil // nothing to uninstall
	}

	cfg := claudeSettings{}
	existing := readJSONFile(path, &cfg)

	if cfg.Hooks == nil {
		return nil
	}

	// Filter out vibecop hook entries.
	filtered := slices.DeleteFunc(cfg.Hooks.PreToolUse, func(e claudePreToolEntry) bool {
		return len(e.Hooks) > 0 && e.Hooks[0].Command == "vibecop hook"
	})

	if len(filtered) == len(cfg.Hooks.PreToolUse) {
		return nil // nothing to remove
	}

	if len(filtered) == 0 {
		cfg.Hooks.PreToolUse = nil
		cfg.Hooks = nil
	} else {
		cfg.Hooks.PreToolUse = filtered
	}

	return writeJSONFile(path, cfg, existing)
}

func uninstallGeminiHooks() error {
	path, err := geminiSettingsPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	cfg := geminiSettings{}
	existing := readJSONFile(path, &cfg)

	if cfg.Hooks == nil || cfg.Hooks.BeforeTool != "vibecop hook" {
		return nil
	}

	cfg.Hooks.BeforeTool = ""

	// Clean up empty hooks.
	if cfg.Hooks.BeforeTool == "" {
		cfg.Hooks = nil
	}

	return writeJSONFile(path, cfg, existing)
}

// readJSONFile reads a JSON file into dst. If the file doesn't exist,
// dst is left unchanged and existingOk is false.
func readJSONFile(path string, dst any) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(dst); err != nil {
		return false
	}
	return true
}

// writeJSONFile writes cfg to path with standard formatting.
func writeJSONFile(path string, cfg any, _ bool) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
