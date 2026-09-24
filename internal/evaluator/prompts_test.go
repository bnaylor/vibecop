package evaluator

import (
	"os"
	"strings"
	"testing"
)

// denySection / escalateSection slice the baseline prompt into its two
// classification lists so tests can assert which list a pattern lives in.
func denySection(t *testing.T) string {
	t.Helper()
	return sectionBetween(t, "DENY immediately", "ESCALATE (")
}

func escalateSection(t *testing.T) string {
	t.Helper()
	return sectionBetween(t, "ESCALATE (", "APPROVE everything else")
}

func sectionBetween(t *testing.T, start, end string) string {
	t.Helper()
	i := strings.Index(BaselinePrompt, start)
	if i < 0 {
		t.Fatalf("baseline prompt has no %q section", start)
	}
	j := strings.Index(BaselinePrompt[i:], end)
	if j < 0 {
		t.Fatalf("baseline prompt has no %q after %q", end, start)
	}
	return BaselinePrompt[i : i+j]
}

// TestBaselineEscalatesHomeDirWrites pins the deny/escalate boundary: a
// hard deny on home-dir and shell-config writes has no approval path, so a
// human who genuinely wanted the action re-runs it in a plain shell where
// vibecop sees nothing. These belong in escalate.
func TestBaselineEscalatesHomeDirWrites(t *testing.T) {
	deny := denySection(t)
	escalate := escalateSection(t)

	for _, pattern := range []string{
		".bashrc",
		"~/.claude/",
		"Package installs",
		"kill, pkill",
	} {
		if strings.Contains(deny, pattern) {
			t.Errorf("%q is in the DENY list; a human may legitimately request it, so it belongs in ESCALATE", pattern)
		}
		if !strings.Contains(escalate, pattern) {
			t.Errorf("%q should be enumerated in the ESCALATE list", pattern)
		}
	}
}

// TestBaselineKeepsHardBoundaries guards against over-loosening: the
// categories no user instruction should clear stay terminal.
func TestBaselineKeepsHardBoundaries(t *testing.T) {
	deny := denySection(t)
	for _, pattern := range []string{
		"Recursive deletion of paths at or above the home directory",
		"~/.ssh",
		"~/.gnupg",
		"keychain",
		"exfiltration",
	} {
		if !strings.Contains(deny, pattern) {
			t.Errorf("%q must stay in the DENY list", pattern)
		}
	}
}

// TestBaselineExplainsDenyVsEscalate keeps the rationale in the prompt.
// Enumerated rules alone produce inconsistent verdicts on patterns the
// lists do not name; the model needs the principle to generalize from.
func TestBaselineExplainsDenyVsEscalate(t *testing.T) {
	if !strings.Contains(BaselinePrompt, "Deny is terminal") {
		t.Error("baseline prompt should state why deny is reserved for hard boundaries")
	}
}

func TestInitializationPromptCarriesDenyEscalateBoundary(t *testing.T) {
	if !strings.Contains(InitializationPrompt, "deny is terminal") {
		t.Error("generated Guardian prompts should inherit the deny/escalate boundary guidance")
	}
}

// TestSpecMatchesPrompts catches doc drift: docs/spec.md embeds both
// prompts verbatim, and they had silently diverged from the source before.
func TestSpecMatchesPrompts(t *testing.T) {
	data, err := os.ReadFile("../../docs/spec.md")
	if err != nil {
		t.Skipf("spec not readable: %v", err)
	}
	spec := string(data)
	if !strings.Contains(spec, BaselinePrompt) {
		t.Error("docs/spec.md baseline prompt is out of sync with BaselinePrompt")
	}
	if !strings.Contains(spec, InitializationPrompt) {
		t.Error("docs/spec.md initialization prompt is out of sync with InitializationPrompt")
	}
}
