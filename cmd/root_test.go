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
