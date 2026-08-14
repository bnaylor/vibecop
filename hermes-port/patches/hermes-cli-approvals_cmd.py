"""
hermes approvals — Guardian mode management.

Guardian mode extends Hermes' smart approvals with project-aware context and
an editable prompt file. Subcommands:

  hermes approvals [status]   Show guardian config and prompt status
  hermes approvals init       Generate a project-specific guardian prompt
  hermes approvals refine     Regenerate the guardian prompt using recent activity

Storage: ``<project-root>/.hermes/guardian-prompt.md`` or
``~/.hermes/guardian-prompt.md`` or the path configured in
``approvals.guardian.prompt_path``.
"""
from __future__ import annotations

import logging
import os
from pathlib import Path

logger = logging.getLogger("hermes.approvals_cmd")


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _get_project_root() -> Path | None:
    """Detect the project root using coding_context."""
    try:
        from agent.coding_context import _git_root, _marker_root

        cwd = Path(os.getcwd()).resolve()
        return _git_root(cwd) or _marker_root(cwd)
    except Exception:
        return None


def _default_prompt_path() -> Path | None:
    """Return the default guardian prompt path for the current project, or None."""
    root = _get_project_root()
    if root is None:
        return None
    path = root / ".hermes" / "guardian-prompt.md"
    path.parent.mkdir(parents=True, exist_ok=True)
    return path


def _get_guardian_prompt_path() -> Path | None:
    """Resolve the effective guardian prompt path from config or project root."""
    try:
        from tools.approval import _get_guardian_config

        cfg = _get_guardian_config()
        explicit = cfg.get("prompt_path", "") or ""
        if explicit:
            p = Path(explicit).expanduser()
            if p.parent.exists():
                return p
    except Exception:
        pass
    return _default_prompt_path()


def _gather_project_info() -> str:
    """Gather project facts for the init prompt generator.
    
    Returns a formatted block with workspace info, manifests, and context files.
    """
    parts = []

    # Workspace snapshot
    try:
        from agent.coding_context import build_coding_workspace_block

        block = build_coding_workspace_block()
        if block:
            parts.append(block)
    except Exception:
        pass

    # Read context files for richer analysis
    root = _get_project_root()
    if root:
        for ctx_file in ("AGENTS.md", "CLAUDE.md", ".cursorrules", "README.md"):
            path = root / ctx_file
            if path.is_file() and path.stat().st_size <= 50 * 1024:
                try:
                    content = path.read_text(encoding="utf-8", errors="replace")
                    parts.append(f"\n--- {ctx_file} (first 2KB) ---\n{content[:2048]}")
                except Exception:
                    pass

    return "\n\n".join(parts)


# ---------------------------------------------------------------------------
# Subcommand handlers
# ---------------------------------------------------------------------------

def cmd_approvals_status(args) -> None:  # noqa: ARG001
    """Show guardian config and prompt status."""
    from tools.approval import _get_approval_config, _get_guardian_config, _is_guardian_enabled

    approvals_cfg = _get_approval_config()
    mode = approvals_cfg.get("mode", "manual")
    guardian_cfg = _get_guardian_config()

    print()
    print("  Guardian Mode")
    print("  ─────────────")
    print(f"  Enabled:          {'✓ yes' if _is_guardian_enabled() else '○ no'}")
    print(f"  Approval mode:    {mode}")
    
    prompt_path = _get_guardian_prompt_path()
    if prompt_path and prompt_path.exists():
        size = prompt_path.stat().st_size
        print(f"  Prompt file:      {prompt_path}  ({size} bytes)")
        preview = prompt_path.read_text(encoding="utf-8", errors="replace")[:300]
        print(f"  Preview:          {preview[:80].strip()!r}...")
    elif prompt_path:
        print(f"  Prompt file:      {prompt_path}  (not yet created)")
    else:
        print("  Prompt file:      (no project root detected — create one manually)")

    activity_window = guardian_cfg.get("activity_window", 10)
    print(f"  Activity window:  {activity_window}")

    # Show auxiliary model config
    try:
        from agent.auxiliary_client import _resolve_task_provider_model
        provider, model, *_ = _resolve_task_provider_model("approval")
        print(f"  Approval LLM:     {model or '(auto)'}  (via {provider})")
    except Exception:
        pass

    print()


def cmd_approvals_init(args) -> None:  # noqa: ARG001
    """Generate a project-specific guardian prompt using the auxiliary LLM."""
    from agent.auxiliary_client import call_llm

    root = _get_project_root()
    if root is None:
        print("  ✗ No project root detected. Run this from inside a git repo or")
        print("    a directory with a project manifest (go.mod, package.json, etc.).")
        raise SystemExit(1)

    prompt_path = _get_guardian_prompt_path()
    if prompt_path is None:
        print("  ✗ Could not determine prompt file path.")
        raise SystemExit(1)

    if prompt_path.exists():
        print(f"  ○ Guardian prompt already exists at {prompt_path}")
        resp = input("  Overwrite? [y/N] ").strip().lower()
        if resp != "y":
            print("  Cancelled.")
            return

    print("  Analyzing project...")
    project_info = _gather_project_info()

    system_prompt = (
        "You are generating a system prompt for an AI approval reviewer (Guardian mode).\n"
        "The reviewer will evaluate terminal commands that an AI coding agent wants to run\n"
        "and decide whether to approve, deny, or escalate them to the human.\n\n"
        "Analyze the project information below and produce a Guardian system prompt that:\n"
        "1. Explains the reviewer's role (second-opinion AI, no shared context with the agent)\n"
        "2. Describes this specific project — what it is, its tech stack, its build/test workflow\n"
        "3. Gives examples of what should be APPROVED automatically (routine for this project)\n"
        "4. Gives examples of what should trigger ESCALATION or DENIAL\n\n"
        "Rules for the prompt:\n"
        "- Be specific to this project: \"go test ./... is routine\" not \"tests are routine\"\n"
        "- When in doubt, escalate rather than deny\n"
        "- Never approve operations that touch files clearly outside the project\n"
        "- Keep it concise — no more than 60 lines\n\n"
        "Start with \"You are a Guardian approval reviewer\" as the first line.\n"
        "Output ONLY the prompt text — no preamble, no commentary, no markdown fences."
    )

    user_message = f"Project information:\n\n{project_info}\n\nGenerate the Guardian prompt."

    print("  Generating prompt (this may take a moment)...")
    try:
        response = call_llm(
            task="approval",
            messages=[
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": user_message},
            ],
            temperature=0.3,
            max_tokens=1024,
        )
        prompt_text = response.choices[0].message.content or ""
    except Exception as e:
        print(f"  ✗ LLM call failed: {e}")
        print("  You can create the prompt file manually. See the Guardian Mode docs.")
        raise SystemExit(1) from e

    if not prompt_text.strip():
        print("  ✗ LLM returned empty response.")
        raise SystemExit(1)

    # Strip markdown fences if the LLM included them
    prompt_text = prompt_text.strip()
    if prompt_text.startswith("```"):
        prompt_text = prompt_text.split("\n", 1)[-1]
    if prompt_text.endswith("```"):
        prompt_text = prompt_text.rsplit("```", 1)[0]
    prompt_text = prompt_text.strip()

    print(f"\n  Generated prompt ({len(prompt_text)} chars):")
    print("  ───────────────────────────────────────────────")
    for line in prompt_text.splitlines():
        print(f"  {line}")
    print("  ───────────────────────────────────────────────")

    try:
        resp = input("\n  Save this prompt? [Y/n] ").strip().lower()
    except (EOFError, OSError):
        resp = "y"  # non-TTY: auto-save
    if resp == "n":
        print("  Cancelled.")
        return

    prompt_path.write_text(prompt_text, encoding="utf-8")
    print(f"  ✓ Saved to {prompt_path}")

    # Enable guardian if not already
    _enable_guardian_if_needed()

    print("  Guardian mode is active. Your next session will use this prompt.")


def cmd_approvals_refine(args) -> None:  # noqa: ARG001
    """Regenerate the guardian prompt using the current prompt + recent activity."""
    from agent.auxiliary_client import call_llm

    prompt_path = _get_guardian_prompt_path()
    if prompt_path is None or not prompt_path.exists():
        print("  ✗ No guardian prompt found. Run 'hermes approvals init' first.")
        raise SystemExit(1)

    current_prompt = prompt_path.read_text(encoding="utf-8", errors="replace")

    # Gather recent activity from the approval module
    activity_data = ""
    try:
        from tools.approval import _session_activity, get_current_session_key

        session_key = get_current_session_key()
        activities = _session_activity.get(session_key, [])
        if activities:
            lines = ["Recent approval decisions:"]
            for entry in activities[-20:]:  # last 20
                inp = (entry.get("input") or "")[:100]
                lines.append(f"  [{entry.get('verdict', '?')}] {entry.get('tool', '?')}: {inp}")
            activity_data = "\n".join(lines)
    except Exception:
        pass

    system_prompt = (
        "You are refining a Guardian approval prompt. A Guardian prompt is used by\n"
        "a second-opinion AI that reviews tool commands from a coding agent and decides\n"
        "whether to approve, deny, or escalate them.\n\n"
        "Below is the current prompt and recent session activity showing what was\n"
        "approved, denied, or escalated. Use the activity to improve the prompt —\n"
        "add rules for patterns that keep getting escalated (they should be auto-approved\n"
        "or denied), and adjust project-specific guidance.\n\n"
        "Output ONLY the refined prompt — no preamble, no commentary."
    )

    user_parts = [f"Current prompt:\n{current_prompt}"]
    if activity_data:
        user_parts.append(activity_data)
    user_parts.append("Generate the refined Guardian prompt.")
    user_message = "\n\n".join(user_parts)

    print("  Refining prompt using recent activity...")
    try:
        response = call_llm(
            task="approval",
            messages=[
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": user_message},
            ],
            temperature=0.3,
            max_tokens=1024,
        )
        new_prompt = response.choices[0].message.content or ""
    except Exception as e:
        print(f"  ✗ LLM call failed: {e}")
        raise SystemExit(1) from e

    new_prompt = new_prompt.strip()
    # Strip fences
    if new_prompt.startswith("```"):
        new_prompt = new_prompt.split("\n", 1)[-1]
    if new_prompt.endswith("```"):
        new_prompt = new_prompt.rsplit("```", 1)[0]
    new_prompt = new_prompt.strip()

    if not new_prompt:
        print("  ✗ LLM returned empty response.")
        raise SystemExit(1)

    print(f"\n  Refined prompt ({len(new_prompt)} chars):")
    print("  ───────────────────────────────────────────────")
    for line in new_prompt.splitlines():
        print(f"  {line}")
    print("  ───────────────────────────────────────────────")

    try:
        resp = input("\n  Save this refined prompt? [Y/n] ").strip().lower()
    except (EOFError, OSError):
        resp = "y"  # non-TTY: auto-save
    if resp == "n":
        print("  Cancelled.")
        return

    # Backup current
    backup_path = prompt_path.with_suffix(".md.bak")
    prompt_path.rename(backup_path)
    prompt_path.write_text(new_prompt, encoding="utf-8")
    print(f"  ✓ Saved to {prompt_path}")
    print(f"  ✓ Backup at {backup_path}")


def _enable_guardian_if_needed() -> None:
    """Enable guardian mode in config if it's not already on."""
    try:
        from hermes_cli.config import load_config, save_config

        config = load_config()
        approvals = config.setdefault("approvals", {})
        if not approvals.get("guardian", {}).get("enabled"):
            guardian = approvals.setdefault("guardian", {})
            guardian["enabled"] = True
            if not approvals.get("mode") or approvals["mode"] == "manual":
                approvals["mode"] = "smart"
            save_config(config)
            print("  ✓ Guardian mode enabled in config.yaml")
    except Exception as e:
        print(f"  ⚠ Could not auto-enable guardian in config: {e}")
        print("  To enable manually, add to config.yaml:")
        print("    approvals:")
        print("      mode: smart")
        print("      guardian:")
        print("        enabled: true")


# ---------------------------------------------------------------------------
# Dispatch
# ---------------------------------------------------------------------------

def cmd_approvals(args) -> None:
    """Top-level dispatcher for ``hermes approvals [subcommand]``."""
    sub = getattr(args, "approvals_command", None)
    if sub in {None, "", "status", "show", "info"}:
        cmd_approvals_status(args)
    elif sub == "init":
        cmd_approvals_init(args)
    elif sub == "refine":
        cmd_approvals_refine(args)
    else:
        print(f"Unknown approvals subcommand: {sub}")
        print("Use one of: status, init, refine")
        raise SystemExit(2)
