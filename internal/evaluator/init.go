package evaluator

import (
	"bytes"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// InitializationPrompt is passed to the agent to generate a Guardian prompt.
const InitializationPrompt = `You are generating a system prompt for VibeCop, a lightweight AI that reviews
tool-use requests from a coding agent in real time and decides whether to approve,
deny, or escalate them to a human.

Your task: analyze this project and produce a VibeCop system prompt that will
help it make accurate, conservative decisions about what tool use is normal and
expected here.

To do this:
1. Read the project root: look for README.md, CLAUDE.md, AGENTS.md, package.json,
   Package.swift, Cargo.toml, pyproject.toml, go.mod, or any other top-level
   config that reveals the tech stack and project purpose.
2. Note the language(s), build tools, test frameworks, and any external services
   or APIs the project uses.
3. Note any existing agent configuration (hooks, permissions) that hints at what
   kinds of tool use are already expected.

CRITICAL: Always allow the project's own build toolchain (e.g., go, swift,
cargo, npm) even in early/empty project states — the first thing after init is
building the project itself.

Then write a system prompt for VibeCop. The prompt must:
- Explain VibeCop's role: second-opinion AI for tool-use approvals, no shared
  context with the primary agent, conservative by design.
- Describe this specific project: what it is, its tech stack, its build/test
  workflow, and what kinds of commands are routine.
- Give examples of what should be approved automatically for this project.
- Give examples of what should trigger escalation or denial.
- Specify the response format VibeCop must always use:
    { "verdict": "approve" | "deny" | "escalate", "reason": "..." }
- Instruct VibeCop: when in doubt, escalate rather than deny; never approve
  operations that touch files or network resources clearly outside the project.
- Instruct VibeCop on the deny/escalate boundary: deny is terminal and prompts
  no human, so reserve it for actions no user instruction should clear
  (credential theft, exfiltration, destroying data outside the project).
  Everything else worrying — home-directory and agent-config writes, global
  installs, irreversible-looking commands — goes to escalate, because the user
  may have asked for it. A deny the user wanted just moves the action to a
  plain shell where VibeCop sees nothing.

CRITICAL: Your output will be saved verbatim as the VibeCop system prompt file.
Start with "You are VibeCop" as the very first line. Do NOT include any
introductory text, commentary, meta-commentary (e.g. "Now I have enough context"),
markdown fences, or closing remarks. Output ONLY the system prompt text,
beginning immediately with its first line and ending after its last.
No preamble of any kind.`

// agentInvocation describes how to drive a harness CLI headlessly for
// prompt generation. binaries are tried in order and the first one on PATH
// wins — the list only ever holds alias names for the *same* CLI, never a
// different vendor's. Picking whatever agent happens to be installed would
// silently ignore the --harness value the caller passed.
type agentInvocation struct {
	binaries []string
	args     func(prompt string) []string
}

var agentInvocations = map[string]agentInvocation{
	HarnessClaude: {
		binaries: []string{"claude"},
		args:     func(p string) []string { return []string{"-p", p, "--output-format", "text"} },
	},
	HarnessGemini: {
		binaries: []string{"gemini"},
		args:     func(p string) []string { return []string{"-p", p} },
	},
	// Antigravity installs under either name depending on how it was
	// fetched; both keys resolve to the same CLI, preferring their own
	// spelling.
	HarnessAntigravity: {
		binaries: []string{"antigravity", "agy"},
		args:     func(p string) []string { return []string{"-p", p} },
	},
	HarnessAgy: {
		binaries: []string{"agy", "antigravity"},
		args:     func(p string) []string { return []string{"-p", p} },
	},
	// codex/copilot use each vendor's documented headless invocation but
	// have not been exercised end-to-end here (issue #28). init prints the
	// generated prompt for review before saving, so a wrong flag or a
	// session preamble in the output shows up rather than being written
	// silently to system-prompt.md.
	HarnessCodex: {
		binaries: []string{"codex"},
		args:     func(p string) []string { return []string{"exec", p} },
	},
	HarnessCopilot: {
		binaries: []string{"copilot"},
		args:     func(p string) []string { return []string{"-p", p, "--allow-all-tools"} },
	},
}

// SupportedHarnesses returns the harness values GeneratePrompt accepts,
// sorted. Single source of truth for the --harness help text and the
// unsupported-value error, so the two can't drift from the real set.
func SupportedHarnesses() []string {
	out := make([]string, 0, len(agentInvocations))
	for h := range agentInvocations {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

// GeneratePrompt runs the specified agent to generate a Guardian prompt.
// extraContext is appended to the initialization prompt (used by refine).
func GeneratePrompt(harness, extraContext string) (string, error) {
	prompt := InitializationPrompt
	if extraContext != "" {
		prompt += "\n\n" + extraContext
	}

	inv, ok := agentInvocations[harness]
	if !ok {
		return "", fmt.Errorf("unsupported harness: %s (want one of: %s)",
			harness, strings.Join(SupportedHarnesses(), ", "))
	}

	bin, err := resolveBinary(inv.binaries)
	if err != nil {
		return "", fmt.Errorf("harness %s: %w", harness, err)
	}
	return runAgent(bin, inv.args(prompt))
}

// resolveBinary returns the first candidate found on PATH.
func resolveBinary(candidates []string) (string, error) {
	for _, c := range candidates {
		if _, err := exec.LookPath(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("not on PATH (looked for: %s)", strings.Join(candidates, ", "))
}

func runAgent(bin string, args []string) (string, error) {
	cmd := exec.Command(bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %w\n%s", bin, err, strings.TrimSpace(stderr.String()))
	}

	out := strings.TrimSpace(stdout.String())
	if out == "" {
		return "", fmt.Errorf("%s produced no output", bin)
	}
	return out, nil
}

// RefineContext builds extra context for the refine flow from the current
// system prompt and recent activity entries.
func RefineContext(currentPrompt, activityData string) string {
	var b strings.Builder
	b.WriteString("Current system prompt:\n\n")
	b.WriteString(currentPrompt)
	b.WriteString("\n\n")
	b.WriteString("Recent activity (tool-use verdicts from this project):\n\n")
	if activityData == "" {
		b.WriteString("(no recent activity)\n")
	} else {
		b.WriteString(activityData)
	}
	b.WriteString("\n\n")
	b.WriteString("Please review and improve the system prompt above based on this ")
	b.WriteString("activity. Keep what works, fix what doesn't, and output the ")
	b.WriteString("entire revised prompt following the same format rules as before.")
	return b.String()
}
