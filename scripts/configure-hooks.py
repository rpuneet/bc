#!/usr/bin/env python3
"""
Configure Claude Code hooks for bc agents.

All 22 hooks POST rich JSON to bcd's /api/agents/{name}/hook endpoint.
Hook input comes via stdin as JSON from Claude Code — we extract tool names,
commands, subagent info, errors, etc. and forward to bcd.

This is the SINGLE source of truth for agent status. No polling, no file IPC.

Usage:
    python3 scripts/configure-hooks.py --workspace /path --runtime tmux
    python3 scripts/configure-hooks.py --workspace /path --runtime docker
    python3 scripts/configure-hooks.py --dry-run --runtime docker
"""

import argparse
import json
import os
import sys

# Hook definitions: event name → state mapping + what to extract from stdin JSON
# Claude Code passes JSON via stdin with fields like tool_name, tool_input, etc.
HOOKS = {
    # ── Session lifecycle ──
    "SessionStart": {
        "state": "idle",
        "extract": """'{"event":"SessionStart","state":"idle","task":"Session started"}'""",
    },
    "SessionEnd": {
        "state": "stopped",
        "extract": """'{"event":"SessionEnd","state":"stopped","task":"Session ended"}'""",
    },

    # ── User interaction ──
    "UserPromptSubmit": {
        "state": "working",
        "extract": """'{"event":"UserPromptSubmit","state":"working","task":"Processing prompt..."}'""",
    },

    # ── Tool usage (stdin has tool_name, tool_input, tool_response) ──
    "PreToolUse": {
        "state": "working",
        # Extract tool_name and command from stdin JSON
        "extract": """$(echo "$HOOK_INPUT" | jq -c '{event:"PreToolUse",state:"working",tool_name:.tool_name,task:("Running: "+.tool_name),command:.tool_input.command,tool_input:.tool_input}')""",
    },
    "PostToolUse": {
        "state": "idle",
        "extract": """$(echo "$HOOK_INPUT" | jq -c '{event:"PostToolUse",state:"idle",tool_name:.tool_name,task:("Done: "+.tool_name)}')""",
    },
    "PostToolUseFailure": {
        "state": "working",
        "extract": """$(echo "$HOOK_INPUT" | jq -c '{event:"PostToolUseFailure",state:"working",tool_name:.tool_name,task:("Failed: "+.tool_name),error:.error}')""",
    },

    # ── Permissions ──
    "PermissionRequest": {
        "state": "stuck",
        "extract": """$(echo "$HOOK_INPUT" | jq -c '{event:"PermissionRequest",state:"stuck",tool_name:.tool_name,task:("Permission: "+.tool_name)}')""",
    },

    # ── Response lifecycle ──
    "Stop": {
        "state": "idle",
        "extract": """'{"event":"Stop","state":"idle","task":"Turn complete"}'""",
    },
    # StopFailure is NOT a valid Claude Code hook key — omitted intentionally.
    # See invalidHookKeys in pkg/agent/hooks.go.

    # ── Notifications ──
    "Notification": {
        "state": "",
        "extract": """$(echo "$HOOK_INPUT" | jq -c '{event:"Notification",message:.message}')""",
    },

    # ── Subagents ──
    "SubagentStart": {
        "state": "working",
        "extract": """$(echo "$HOOK_INPUT" | jq -c '{event:"SubagentStart",state:"working",task:("Subagent: "+(.agent_type // "unknown")),subagent_id:.agent_id,subagent_type:.agent_type}')""",
    },
    "SubagentStop": {
        "state": "working",
        "extract": """$(echo "$HOOK_INPUT" | jq -c '{event:"SubagentStop",state:"working",task:"Subagent completed",subagent_id:.agent_id,subagent_type:.agent_type}')""",
    },

    # ── Tasks ──
    "TaskCompleted": {
        "state": "done",
        "extract": """$(echo "$HOOK_INPUT" | jq -c '{event:"TaskCompleted",state:"done",task:("Task done: "+(.task_description // ""))}')""",
    },

    # ── Teammate ──
    "TeammateIdle": {
        "state": "",
        "extract": """$(echo "$HOOK_INPUT" | jq -c '{event:"TeammateIdle",teammate:.teammate}')""",
    },

    # ── Instructions ──
    "InstructionsLoaded": {
        "state": "",
        "extract": """$(echo "$HOOK_INPUT" | jq -c '{event:"InstructionsLoaded",file:.file_path}')""",
    },

    # ── Config ──
    "ConfigChange": {
        "state": "",
        "extract": """$(echo "$HOOK_INPUT" | jq -c '{event:"ConfigChange",file:.changed_file_path}')""",
    },

    # ── Worktree ──
    "WorktreeCreate": {
        "state": "starting",
        "extract": """$(echo "$HOOK_INPUT" | jq -c '{event:"WorktreeCreate",state:"starting",task:"Creating worktree"}')""",
    },
    "WorktreeRemove": {
        "state": "",
        "extract": """$(echo "$HOOK_INPUT" | jq -c '{event:"WorktreeRemove",task:"Removing worktree"}')""",
    },

    # ── Context compaction ──
    "PreCompact": {
        "state": "working",
        "extract": """'{"event":"PreCompact","state":"working","task":"Compacting context..."}'""",
    },
    "PostCompact": {
        "state": "working",
        "extract": """'{"event":"PostCompact","state":"working","task":"Context compacted"}'""",
    },

    # ── MCP elicitation ──
    "Elicitation": {
        "state": "stuck",
        "extract": """$(echo "$HOOK_INPUT" | jq -c '{event:"Elicitation",state:"stuck",task:"MCP input needed",server:.server_name}')""",
    },
    "ElicitationResult": {
        "state": "working",
        "extract": """'{"event":"ElicitationResult","state":"working","task":"MCP input received"}'""",
    },
}

# ── bc-internal events (not Claude Code hooks, triggered by bcd itself) ──
# These are POSTed to /api/agents/{name}/hook by bcd's Go code, not by the
# Python script. Defined here for documentation and endpoint validation.
BC_INTERNAL_EVENTS = {
    "ChannelMessage": {
        "state": "",  # no state change
        "fields": ["channel", "sender", "message", "mentions"],
        "description": "Agent received a channel message",
    },
    "ChannelSent": {
        "state": "",  # no state change
        "fields": ["channel", "message"],
        "description": "Agent sent a message to a channel",
    },
    "AgentMessage": {
        "state": "",  # no state change
        "fields": ["from_agent", "message"],
        "description": "Agent received a direct message from another agent",
    },
    "CostUpdate": {
        "state": "",  # no state change
        "fields": ["input_tokens", "output_tokens", "cost_usd", "model"],
        "description": "Cost data imported for this agent",
    },
}


def build_hook_command(event_name: str, hook_config: dict, bcd_addr: str) -> str:
    """
    Build a bash command that:
    1. Reads JSON from stdin (Claude Code's hook input)
    2. Extracts relevant fields via jq
    3. POSTs to bcd /api/agents/{name}/hook
    """
    extract = hook_config["extract"]

    # Wrap in a bash script that:
    # - Captures stdin (hook input JSON)
    # - Extracts fields with jq
    # - POSTs to bcd
    cmd = (
        f'bash -c \''
        f'HOOK_INPUT=$(cat); '
        f'PAYLOAD={extract}; '
        f'curl -sX POST {bcd_addr}/api/agents/${{MYCEL_AGENT_ID}}/hook '
        f'-H "Content-Type: application/json" '
        f'-d "$PAYLOAD" 2>/dev/null || true'
        f'\''
    )
    return cmd


def generate_settings(bcd_addr: str) -> dict:
    """Generate the .claude/settings.json hooks section."""
    hooks = {}
    for event_name, config in HOOKS.items():
        cmd = build_hook_command(event_name, config, bcd_addr)
        hooks[event_name] = [
            {
                "hooks": [
                    {
                        "type": "command",
                        "command": cmd,
                    }
                ]
            }
        ]
    return {"hooks": hooks}


def merge_settings(existing: dict, new_hooks: dict) -> dict:
    """Merge new hooks into existing settings without clobbering user config."""
    if "hooks" not in existing:
        existing["hooks"] = {}

    for event_name, matchers in new_hooks["hooks"].items():
        # Always replace bc hooks (identified by /api/agents/ in command)
        if event_name in existing["hooks"]:
            # Remove old bc hooks, keep user hooks
            user_hooks = []
            for matcher in existing["hooks"][event_name]:
                for hook in matcher.get("hooks", []):
                    if "/api/agents/" not in hook.get("command", ""):
                        user_hooks.append(matcher)
                        break
            existing["hooks"][event_name] = user_hooks + matchers
        else:
            existing["hooks"][event_name] = matchers

    return existing


def configure(workspace: str, bcd_addr: str) -> None:
    """Write hooks to .claude/settings.json in the workspace."""
    claude_dir = os.path.join(workspace, ".claude")
    os.makedirs(claude_dir, exist_ok=True)

    settings_path = os.path.join(claude_dir, "settings.json")
    new_hooks = generate_settings(bcd_addr)

    existing = {}
    if os.path.exists(settings_path):
        try:
            with open(settings_path) as f:
                existing = json.load(f)
        except (json.JSONDecodeError, IOError):
            existing = {}

    merged = merge_settings(existing, new_hooks)

    with open(settings_path, "w") as f:
        json.dump(merged, f, indent=2)
        f.write("\n")

    print(f"Configured {len(HOOKS)} hooks in {settings_path}")
    print(f"  bcd: {bcd_addr}")
    print(f"  Events: {', '.join(HOOKS.keys())}")


def main():
    parser = argparse.ArgumentParser(description="Configure Claude Code hooks for bc agents")
    parser.add_argument("--workspace", default=".", help="Workspace root (default: .)")
    parser.add_argument("--runtime", choices=["tmux", "docker"], default="tmux", help="Runtime (default: tmux)")
    parser.add_argument("--addr", default="", help="Custom bcd address")
    parser.add_argument("--dry-run", action="store_true", help="Print JSON without writing")
    args = parser.parse_args()

    if args.addr:
        bcd_addr = args.addr
    elif args.runtime == "docker":
        bcd_addr = "http://host.docker.internal:9374"
    else:
        bcd_addr = "http://127.0.0.1:9374"

    if args.dry_run:
        settings = generate_settings(bcd_addr)
        print(json.dumps(settings, indent=2))
        return

    workspace = os.path.abspath(args.workspace)
    if not os.path.isdir(workspace):
        print(f"Error: {workspace} not found", file=sys.stderr)
        sys.exit(1)

    configure(workspace, bcd_addr)


if __name__ == "__main__":
    main()
