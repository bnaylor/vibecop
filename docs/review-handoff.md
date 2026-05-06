# Code Review Handoff

This document summarizes the code review performed after the initial implementation,
the bugs found, and the fixes applied. It is intended to bring the next session up
to speed before the first end-to-end test run.

---

## What was reviewed

Full source tree against `docs/spec.md`. All 47 original tests passed; `go vet` was
clean; the binary built. Review was performed before any manual testing.

---

## Bugs found and fixed (shipped)

### Critical

**1. `writeJSONFile` silently deleted unknown keys from settings.json**  
File: `internal/hooks/install.go`  
The install/uninstall functions read `~/.claude/settings.json` and
`~/.gemini/settings.json` into narrow typed structs (`claudeSettings`,
`geminiSettings`), then wrote them back — destroying any keys those structs
didn't know about (e.g. `theme`, `model`, `permissions`, `env` in Claude Code's
settings).

Fix: All four functions (`installClaudeHooks`, `installGeminiHooks`,
`uninstallClaudeHooks`, `uninstallGeminiHooks`) now read the existing file as
`map[string]any` via a new `readRawJSON` helper, manipulate only the `hooks`
subtree, and write the merged map back via `writeRawJSON`. The old `readJSONFile`
/ `writeJSONFile` pair was removed. Four new tests verify key preservation for
both harnesses on both install and uninstall.

**2. `vibecop hook` would trigger the interactive setup wizard**  
File: `cmd/root.go`  
`PersistentPreRunE` launched the interactive setup wizard for any command not
named `setup`, `help`, or `completion` when no config file was present. `hook` was
not excluded. Because `hook` is invoked non-interactively by the coding harness,
this would block on stdin with no terminal attached.

Fix: Extracted a `shouldTriggerSetup(cmdName string) bool` helper. `hook`, `stop`,
and `status` are now excluded. One new test covers the skip list.

### High

**3. No consecutive-failure pass-through**  
File: `cmd/start.go`  
The spec requires: after 3 consecutive evaluator failures, VibeCop suspends for the
session and returns `approve` (fail-open) for all subsequent requests until the
daemon is restarted or `vibecop test` succeeds.

Fix: `makePermissionHandler` now tracks `consecutiveFailures` and `suspended` in the
closure (protected by a mutex for concurrent requests). On the 3rd consecutive
failure it sets `suspended = true` and emits a warning TUI event. While suspended,
all requests return `{verdict: "approve"}` immediately. The counter resets on any
successful evaluation. This required introducing an `evalClient` interface so the
handler could be unit-tested with a fake evaluator. Two new tests cover suspension
and counter reset.

### Medium

**4. `activity.go` Save used a fragile hand-rolled `filepath.Dir`**  
File: `internal/audit/activity.go`  
The directory creation before writing `activity.jsonl` manually subtracted
`"/activity.jsonl"` from the path as a string slice, with a secondary loop-based
fallback. Both replaced with `filepath.Dir(path)`.

**5. `think: false` injected into all OpenAI-format requests**  
File: `internal/evaluator/evaluator.go`  
The spec says `think: false` is for Ollama CoT models only. It was being injected
for every OpenAI-format request, including cloud providers that don't support it.

Fix: Added `ollamaCoT bool` to `Client` and a `isOllamaEndpoint(endpoint string)
bool` helper (checks for `localhost` / `127.0.0.1`). `think: false` is only
injected when `ollamaCoT` is true. Two new tests verify the behavior via
`buildOpenAIRequest` directly.

**6. `go.mod` declared `go 1.25.5` (a non-existent version)**  
Changed to `go 1.22` per the spec.

### Low

- **`min` function in `cmd/status.go`** shadowed the Go 1.21+ builtin. Removed;
  now uses the builtin directly.
- **`postSetup` called `testCmd.RunE(nil, nil)`** — passing a nil `*cobra.Command`
  is fragile. Changed to `testCmd.RunE(testCmd, nil)`.
- **`hooks.go` lines 100 and 106** had multiple statements on one line (legacy from
  the LLM's aggressive sed edits). Reformatted to standard Go style.
- **`daemon.go` PID parse** used `data[:len(data)-1]` to strip a trailing newline.
  Replaced with `strings.TrimSpace(string(data))`.
- **TUI log panel** cleared the entire view when it exceeded `maxLogLines`. Now
  trims to the last N lines using `strings.Split`.
- **`App.Close()`** was never called after `tui.Run()` returned; the daemon
  connection leaked until GC. Now called explicitly after `runUI()` returns.
- **deepseek harness references** in CLI flag help text (e.g. `--harness
  claude|gemini|deepseek`). No deepseek CLI harness was ever implemented and none
  is needed — the project uses `~/bin/claude-ds`, a claude-compatible wrapper that
  already works with `--harness claude`. Hint text updated to say "or any
  claude-compatible wrapper".

---

## Known gaps not fixed (architectural or deferred to first-run testing)

**Audit pending completion path is incomplete**  
`audit.Logger` has `WritePending` / `CompletePending` / `FlushPending` which are
internally correct. But `CompletePending` is never called — there is no signal path
from the harness back to the daemon carrying the human's approve/block decision.
Pending records flush with `"blocked"` on daemon shutdown. This is an architectural
gap: the hook's exit-code path only communicates direction, not the human's
resolution. Deferred pending first-run experience and a design decision on how to
capture human decisions.

**TUI: several spec features stubbed but not wired**  
- `enter` key expand/collapse reason: keyboard handler only handles `q` and `r`;
  `enter` binding shown in status bar is not connected.
- `r` refresh config: `refreshConfig()` sets a placeholder string; config is never
  re-fetched from the daemon.
- Config panel: `UpdateConfig()` exists and is correct but is never called; panel
  stays at "waiting for data..." forever.
- Header: shows event count only; spec calls for active project path and
  Guardian/Baseline mode indicator.

These are all plausible first-run punch-list items — visible immediately on `vibecop
tui` and good candidates for the next session.

---

## Test count

| Before review | After fixes |
|---|---|
| 47 tests | 71 tests |

All 71 pass. `go vet` clean. Binary builds.
