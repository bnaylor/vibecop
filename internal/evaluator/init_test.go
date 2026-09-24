package evaluator

import (
	"sort"
	"strings"
	"testing"

	"github.com/bnaylor/vibecop/internal/hooks"
)

func TestInitializationPrompt(t *testing.T) {
	if InitializationPrompt == "" {
		t.Fatal("initialization prompt should not be empty")
	}
	if !strings.Contains(InitializationPrompt, "You are generating") {
		t.Error("should start with the expected opener")
	}
	if !strings.Contains(InitializationPrompt, "VibeCop") {
		t.Error("should mention VibeCop")
	}
	if !strings.Contains(InitializationPrompt, "go.mod") {
		t.Error("should mention go.mod as a project marker")
	}
}

func TestInitializationPromptAllowsToolchain(t *testing.T) {
	// The prompt should explicitly permit the project's build toolchain.
	if !strings.Contains(InitializationPrompt, "go") &&
		!strings.Contains(InitializationPrompt, "build toolchain") {
		t.Error("prompt should instruct the agent to allow the project's build toolchain")
	}
}

func TestGeneratePromptUnsupportedHarness(t *testing.T) {
	_, err := GeneratePrompt("unsupported", "")
	if err == nil {
		t.Fatal("expected error for unsupported harness")
	}
}

func TestRefineContext(t *testing.T) {
	current := "You are VibeCop, guardian of this project."
	activity := `{"tool":"Bash","verdict":"approve"}
{"tool":"Read","verdict":"approve"}`

	ctx := RefineContext(current, activity)
	if !strings.Contains(ctx, current) {
		t.Error("refine context should include current prompt")
	}
	if !strings.Contains(ctx, "Recent activity") {
		t.Error("refine context should include activity section")
	}
	if !strings.Contains(ctx, "approve") {
		t.Error("refine context should include activity data")
	}
}

func TestRefineContextNoActivity(t *testing.T) {
	ctx := RefineContext("You are VibeCop.", "")
	if !strings.Contains(ctx, "no recent activity") {
		t.Error("should indicate no activity when empty")
	}
}

func TestInitializationPromptStartsWithCorrectDirective(t *testing.T) {
	lines := strings.Split(InitializationPrompt, "\n")
	if len(lines) > 0 {
		first := strings.TrimSpace(lines[0])
		if !strings.HasPrefix(first, "You are generating") {
			t.Errorf("expected first line to start with 'You are generating', got %q", first)
		}
	}
}

func TestRefineContextFormat(t *testing.T) {
	// Verify the refine context structure has the right sections.
	ctx := RefineContext("prompt text", "activity data")
	sections := []string{
		"Current system prompt:",
		"prompt text",
		"Recent activity",
		"activity data",
		"improve the system prompt",
	}
	for _, s := range sections {
		if !strings.Contains(ctx, s) {
			t.Errorf("refine context should contain %q", s)
		}
	}
}

// TestAgentInvocationHonorsHarnessValue pins the fix for --harness being
// decorative: the passed value must decide the binary. Previously every
// non-claude value funnelled into a PATH probe that ran antigravity if it
// happened to be installed, whatever the caller asked for.
func TestAgentInvocationHonorsHarnessValue(t *testing.T) {
	for harness, inv := range agentInvocations {
		if len(inv.binaries) == 0 {
			t.Errorf("%s: no candidate binaries", harness)
			continue
		}
		if inv.binaries[0] != harness {
			t.Errorf("%s: first candidate is %q, want the harness's own binary", harness, inv.binaries[0])
		}
		if inv.args == nil {
			t.Errorf("%s: no args builder", harness)
			continue
		}
		args := inv.args("PROMPT")
		var found bool
		for _, a := range args {
			if a == "PROMPT" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: prompt not passed to the CLI: %v", harness, args)
		}
	}
}

// TestSupportedHarnessesCoversHookHarnesses guards the reported mismatch
// between the harnesses hooks can be installed for and the harnesses a
// Guardian prompt can be generated for.
func TestSupportedHarnessesCoversHookHarnesses(t *testing.T) {
	supported := make(map[string]bool)
	for _, h := range SupportedHarnesses() {
		supported[h] = true
	}
	for _, h := range []string{
		hooks.HarnessClaude,
		hooks.HarnessGemini,
		hooks.HarnessCodex,
		hooks.HarnessCopilot,
		hooks.HarnessAntigravity,
		hooks.HarnessAgy,
	} {
		if !supported[h] {
			t.Errorf("harness %q supports hooks but init cannot generate a prompt for it", h)
		}
	}
}

func TestSupportedHarnessesSorted(t *testing.T) {
	got := SupportedHarnesses()
	if !sort.StringsAreSorted(got) {
		t.Errorf("SupportedHarnesses should be sorted for stable help text: %v", got)
	}
	if len(got) != len(agentInvocations) {
		t.Errorf("got %d harnesses, want %d", len(got), len(agentInvocations))
	}
}

func TestUnsupportedHarnessErrorListsSupported(t *testing.T) {
	_, err := GeneratePrompt("cursor", "")
	if err == nil {
		t.Fatal("expected error for unsupported harness")
	}
	if !strings.Contains(err.Error(), "claude") {
		t.Errorf("error should name the supported values, got: %v", err)
	}
}

func TestResolveBinaryMissing(t *testing.T) {
	_, err := resolveBinary([]string{"vibecop-no-such-agent-cli"})
	if err == nil {
		t.Fatal("expected error when no candidate is on PATH")
	}
	if !strings.Contains(err.Error(), "PATH") {
		t.Errorf("error should mention PATH, got: %v", err)
	}
}
