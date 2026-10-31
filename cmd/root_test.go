package cmd

import "testing"

func TestFirstRunSkipList(t *testing.T) {
	// These commands must never trigger the interactive setup wizard —
	// they are called non-interactively (hook by the harness, stop/status
	// by scripts, etc.) and blocking on stdin would break the caller.
	skipCommands := []string{"hook", "setup", "help", "completion"}
	for _, name := range skipCommands {
		if shouldTriggerSetup(name) {
			t.Errorf("command %q should not trigger setup wizard", name)
		}
	}

	// These commands are interactive and should trigger setup when config is absent.
	interactiveCommands := []string{"start", "tui", "init", "install"}
	for _, name := range interactiveCommands {
		if !shouldTriggerSetup(name) {
			t.Errorf("command %q should trigger setup wizard when config is absent", name)
		}
	}
}

func TestInstallAllIncludesGeminiAndAntigravity(t *testing.T) {
	origAll := installAll
	origHarness := installHarness
	t.Cleanup(func() {
		installAll = origAll
		installHarness = origHarness
	})
	installHarness = ""
	installAll = true

	targets := resolveInstallTargets()
	seen := make(map[string]bool)
	for _, h := range targets {
		seen[h] = true
	}
	for _, want := range []string{"claude", "antigravity", "gemini", "codex", "copilot"} {
		if !seen[want] {
			t.Errorf("install --all targets %v missing %q", targets, want)
		}
	}
}
