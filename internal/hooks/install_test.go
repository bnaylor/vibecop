package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallClaudeHooks(t *testing.T) {
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	if err := InstallHooks(HarnessClaude); err != nil {
		t.Fatal(err)
	}

	// Verify the settings file was created.
	path := filepath.Join(tmpHome, ".claude", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var cfg claudeSettings
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}

	if cfg.Hooks == nil || len(cfg.Hooks.PreToolUse) == 0 {
		t.Fatal("expected PreToolUse entries")
	}

	entry := cfg.Hooks.PreToolUse[0]
	if entry.Matcher != "" {
		t.Errorf("expected empty matcher, got %q", entry.Matcher)
	}
	if len(entry.Hooks) == 0 || entry.Hooks[0].Command != "vibecop hook" {
		t.Errorf("expected vibecop hook command, got %+v", entry.Hooks)
	}
}

func TestInstallClaudeHooksIdempotent(t *testing.T) {
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	if err := InstallHooks(HarnessClaude); err != nil {
		t.Fatal(err)
	}
	if err := InstallHooks(HarnessClaude); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(tmpHome, ".claude", "settings.json")
	data, _ := os.ReadFile(path)

	var cfg claudeSettings
	json.Unmarshal(data, &cfg)

	if len(cfg.Hooks.PreToolUse) != 1 {
		t.Errorf("expected 1 entry after two installs, got %d", len(cfg.Hooks.PreToolUse))
	}
}

func TestInstallGeminiHooks(t *testing.T) {
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	if err := InstallHooks(HarnessGemini); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(tmpHome, ".gemini", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var cfg geminiSettings
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}

	if cfg.Hooks == nil || cfg.Hooks.BeforeTool != "vibecop hook" {
		t.Errorf("expected before_tool 'vibecop hook', got %q", cfg.Hooks.BeforeTool)
	}
}

func TestInstallUnsupportedHarness(t *testing.T) {
	err := InstallHooks("deepseek")
	if err == nil {
		t.Fatal("expected error for unsupported harness")
	}
}

func TestUninstallClaudeHooks(t *testing.T) {
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	// Install first.
	InstallHooks(HarnessClaude)

	// Then uninstall.
	if err := UninstallHooks(HarnessClaude); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(tmpHome, ".claude", "settings.json")

	// Read back — should have empty hooks or no hooks key.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]any
	json.Unmarshal(data, &raw)
	if _, ok := raw["hooks"]; ok {
		var cfg claudeSettings
		json.Unmarshal(data, &cfg)
		if cfg.Hooks != nil && len(cfg.Hooks.PreToolUse) > 0 {
			t.Error("expected no PreToolUse entries after uninstall")
		}
	}
}

func TestUninstallGeminiHooks(t *testing.T) {
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	InstallHooks(HarnessGemini)
	if err := UninstallHooks(HarnessGemini); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(tmpHome, ".gemini", "settings.json")
	data, _ := os.ReadFile(path)

	var raw map[string]any
	json.Unmarshal(data, &raw)
	if _, ok := raw["hooks"]; ok {
		var cfg geminiSettings
		json.Unmarshal(data, &cfg)
		if cfg.Hooks != nil && cfg.Hooks.BeforeTool != "" {
			t.Error("expected empty before_tool after uninstall")
		}
	}
}

func TestUninstallWhenNotInstalled(t *testing.T) {
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	// Uninstall without installing first — should be a no-op.
	if err := UninstallHooks(HarnessClaude); err != nil {
		t.Fatal(err)
	}
}

func TestInstallPreservesExistingSettings(t *testing.T) {
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	// Create Claude settings with pre-existing content.
	claudeDir := filepath.Join(tmpHome, ".claude")
	os.MkdirAll(claudeDir, 0755)

	existing := map[string]any{
		"theme": "dark",
		"hooks": map[string]any{
			"PreToolUse": []map[string]any{
				{
					"matcher": "Read",
					"hooks": []map[string]any{
						{"type": "command", "command": "some-other-tool"},
					},
				},
			},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0644)

	// Install vibecop hooks.
	if err := InstallHooks(HarnessClaude); err != nil {
		t.Fatal(err)
	}

	// Verify both hooks exist.
	path := filepath.Join(claudeDir, "settings.json")
	result, _ := os.ReadFile(path)

	var cfg claudeSettings
	json.Unmarshal(result, &cfg)

	if len(cfg.Hooks.PreToolUse) != 2 {
		t.Errorf("expected 2 PreToolUse entries, got %d", len(cfg.Hooks.PreToolUse))
	}
}

func TestInstallClaudePreservesExtraKeys(t *testing.T) {
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	claudeDir := filepath.Join(tmpHome, ".claude")
	os.MkdirAll(claudeDir, 0755)

	// Write a settings file with keys our struct doesn't know about.
	existing := `{"theme":"dark","model":"claude-opus-4-5","preferredLanguage":"en"}`
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(existing), 0644)

	if err := InstallHooks(HarnessClaude); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["theme"] != "dark" {
		t.Errorf("theme was lost: got %v", raw["theme"])
	}
	if raw["model"] != "claude-opus-4-5" {
		t.Errorf("model was lost: got %v", raw["model"])
	}
	if raw["preferredLanguage"] != "en" {
		t.Errorf("preferredLanguage was lost: got %v", raw["preferredLanguage"])
	}
}

func TestUninstallClaudePreservesExtraKeys(t *testing.T) {
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	claudeDir := filepath.Join(tmpHome, ".claude")
	os.MkdirAll(claudeDir, 0755)

	// Install first.
	os.WriteFile(filepath.Join(claudeDir, "settings.json"),
		[]byte(`{"theme":"dark"}`), 0644)
	InstallHooks(HarnessClaude)

	// Now uninstall.
	if err := UninstallHooks(HarnessClaude); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	var raw map[string]any
	json.Unmarshal(data, &raw)
	if raw["theme"] != "dark" {
		t.Errorf("theme was lost after uninstall: got %v", raw["theme"])
	}
}

func TestInstallGeminiPreservesExtraKeys(t *testing.T) {
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	geminiDir := filepath.Join(tmpHome, ".gemini")
	os.MkdirAll(geminiDir, 0755)

	existing := `{"theme":"dark","timeout":30}`
	os.WriteFile(filepath.Join(geminiDir, "settings.json"), []byte(existing), 0644)

	if err := InstallHooks(HarnessGemini); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(geminiDir, "settings.json"))
	var raw map[string]any
	json.Unmarshal(data, &raw)
	if raw["theme"] != "dark" {
		t.Errorf("theme was lost: got %v", raw["theme"])
	}
	if raw["timeout"] != float64(30) {
		t.Errorf("timeout was lost: got %v", raw["timeout"])
	}
}
