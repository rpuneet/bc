# Agent Instructions

## Project Overview

This project is **mycel** - a CLI-first AI agent orchestration platform with an embedded React web UI. It coordinates multiple AI agents (Claude, Gemini, Cursor, etc.) working in isolated tmux sessions or Docker containers, each with its own git worktree.

## Quick Start

Initialize and run mycel:

```bash
mycel up                      # Start server + web UI on localhost:9374 (bootstraps the workspace)
mycel agent create <name> \
  --role engineer \
  --tool claude              # Spawn an agent
mycel status                  # See what's running
```

## Quick Reference

### Managing Agents

```bash
mycel agent list              # List all agents
mycel agent create <name>     # Create new agent
mycel agent attach <agent>    # Attach to agent session
mycel agent peek <agent>      # Watch agent output
mycel agent stop <agent>      # Stop agent
mycel down                    # Stop all agents
```

### Development

```bash
make build                    # Build everything
make test                     # Run all tests
make lint                     # Run linters
make check                    # Full quality gate
```

## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create GitHub issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds

   ```bash
   make test                     # Run tests
   make lint                     # Run linters
   ```

3. **Update issue status** - Close finished GitHub issues, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:

   ```bash
   git pull --rebase
   git push
   git status  # MUST show "up to date with origin"
   ```

5. **Clean up** - Clear stashes, prune remote branches

   ```bash
   git stash clear
   git remote prune origin
   ```

6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds

## Architecture

mycel uses a provider-based architecture where different AI agent CLIs (Claude, Gemini, Cursor, etc.) are registered as providers. Each provider implements a common interface for:

- Starting agents with provider-specific commands
- Session resumption (for providers that support it)
- Version detection

Providers declare additional capabilities via optional interfaces (`ActivitySource`, `CostSource`, `ModelLister`, `ResumableSessionDetector`). Agent state (idle, working, done, error) is not detected by providers: it is derived in `pkg/agent` from ingested hook/transcript events; providers only declare how their activity is sourced via `ActivitySource.ActivityMode()`.

See `pkg/provider/` for provider implementations.
