# vibecop → Hermes port

A port of vibecop's second-opinion approval idea into
[Hermes Agent](https://hermes-agent.nousresearch.com) as **Guardian mode**:
project-aware smart approvals, where a context-free LLM decides
`APPROVE` / `DENY` / `ESCALATE` on every flagged terminal command using an
editable, project-specific prompt plus recent-activity context.

> **This is a port, not a Hermes feature.** Guardian mode is installed by
> patching Hermes source (the `.diff` files under `patches/`) and adding one
> CLI module. Hermes ships `approvals.mode: smart` with a fixed generic prompt;
> Guardian replaces that prompt with one that knows *your* project. Do not
> assume `hermes approvals` exists on a stock install.

## What it does

With `approvals.mode: smart` and `approvals.guardian.enabled: true`:

1. Every pattern-flagged command is scored by an approval LLM.
2. The LLM receives a **project workspace snapshot** (branch, dirty status,
   manifests) and the session's **recent approval activity**.
3. The LLM reads an **editable Guardian prompt** (resolved from config path →
   `<project-root>/.hermes/guardian-prompt.md` → `~/.hermes/guardian-prompt.md`)
   instead of the generic default.
4. Verdicts: `APPROVE` (silent, session-scoped), `DENY` (blocked),
   `ESCALATE` (falls through to the human).

A `hermes approvals` CLI manages it: `status`, `init` (generate a
project-specific prompt from the codebase), `refine` (improve it from recent
activity).

## Install

```bash
cd ~/.hermes/hermes-agent
git apply "$VIBECOP/hermes-port/patches/hermes-tools-approval.py.diff"
git apply "$VIBECOP/hermes-port/patches/hermes-cli-main.py.diff"
cp "$VIBECOP/hermes-port/patches/hermes-cli-approvals_cmd.py" hermes_cli/approvals_cmd.py
```

Add the config from `config.reference.yaml` to `~/.hermes/config.yaml`, then
restart the gateway and run `hermes approvals init` in your project root.

> The diffs target a specific Hermes revision and may not apply cleanly if
> Hermes has diverged. Apply manually — the diffs carry `# === Guardian mode`
> markers, and `approvals_cmd.py` is a standalone file.

## Approval backend

Any fast LLM. `gemini-2.5-flash` (~1.8s) is the recommended default for GCP /
work; a local Ollama model (~1.1s tuned) is an option for air-gapped hosts.
The local stack (tuned model + thinkless proxy + systemd unit) is
deployment-specific and lives with the homelab tooling, not here.

## Guardian prompt

See `prompts/guardian-prompt.example.md`. A plain markdown file with
`APPROVE`, `ESCALATE`, and `DENY` sections, read fresh on every approval — so
edits take effect immediately.

## Pitfalls

- The approval LLM does **not** reliably generalize `10.x.x.x` to a literal
  `10.3.x`. State ranges in CIDR (`10.0.0.0/8`) *and* the concrete subnet.
- `docker run --rm` is often misread as a delete — note that `--rm` is a
  container-lifecycle flag, not `rm -rf`.
- For `gh pr review`, use `--body-file` instead of inline `--body` to dodge
  tirith's Unicode/emoji scanner.
- When a local approval proxy (Ollama) backs Guardian, it is a separate systemd
  unit and will be down after a gateway restart — verify it before testing.

## Relationship to vibecop

Behavior is equivalent to the Go daemon's Guardian mode; the daemon remains
the reference implementation for non-Hermes hosts. This port is maintained
alongside it so Hermes agents get the same protection without running a second
binary.
