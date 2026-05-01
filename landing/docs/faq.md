# mycel FAQ - Frequently Asked Questions

## Installation & Setup

### Q: What are the system requirements for mycel?
**A:** mycel requires:
- **Go 1.25.1+** - Download from [golang.org](https://go.dev/dl)
- **tmux** - Available via `brew install tmux` (macOS) or `apt-get install tmux` (Linux)
- **git 2.30+** - Version control system
- **Claude Code** (or compatible AI tool like Cursor)

Optional: Node.js 18+ if using mycel with JavaScript/TypeScript projects.

### Q: Can I use mycel on Windows?
**A:** Yes, using WSL2 (Windows Subsystem for Linux 2):
1. Install WSL2: `wsl --install -d Ubuntu-22.04`
2. Inside WSL2, install Go and tmux
3. Install mycel using Linux instructions
4. Run bc commands from WSL2 terminal

Native Windows support is not available yet, but WSL2 works well.

### Q: How do I uninstall mycel?
**A:**
```bash
# Remove binary
rm $(which mycel)

# Optional: Remove workspace
rm -rf .bc/

# Optional: Clone directory
rm -rf ~/Projects/bc
```

### Q: What if `make build` fails?
**A:** Common causes:
- **Go not in PATH**: Run `go version` to verify
- **Missing dependencies**: Try `go mod download`
- **Old Go version**: Update to Go 1.25.1+
- **Permission denied**: Try `sudo make install`

Run `make clean` then `make build` again.

---

## Core Concepts

### Q: What is a worktree?
**A:** A worktree is an isolated copy of your git repository where an agent works. bc creates one worktree per agent at `.bc/worktrees/<agent-id>/`. This allows multiple agents to work simultaneously without merge conflicts.

Benefits:
- Each agent has independent git state
- No conflicts even when editing same files
- Easy to review each agent's changes separately
- Can merge cleanly to main

### Q: What is persistent memory?
**A:** All mycel state is stored in git-tracked files in `.bc/`:
- `.bc/agents/agents.json` - Agent state
- `.bc/queue.json` - Work queue
- `.bc/events.jsonl` - Event log

This means:
- State survives crashes and restarts
- Full audit trail of all work
- Can rollback to previous states
- Complete debugging history

### Q: How do roles and capabilities work?
**A:** mycel uses role-based access control:

| Role | Can Create | Can Assign | Can Execute |
|------|-----------|-----------|------------|
| ProductManager | Managers | Work | No |
| Manager | Engineers, QA | Work | No |
| Engineer | Nothing | Nothing | Yes |
| QA | Nothing | Nothing | Yes |

Example:
```bash
bc spawn pm-01 --role product-manager  # Can spawn managers
bc spawn eng-01 --role engineer        # Can only execute work
```

### Q: What is the work queue?
**A:** The work queue tracks tasks through their lifecycle:
```
pending → assigned → working → done
                               ↓ (conflict)
                              stuck
```

Each task has:
- Unique ID (work-0001)
- Status (pending, assigned, working, done, stuck, failed)
- Assigned agent
- Merge state

View with: `bc queue list`

---

## Workflow Questions

### Q: Can multiple agents work on the same file?
**A:** Yes! mycel uses git worktrees to prevent conflicts:
- Agent A edits `src/app.js` in their worktree
- Agent B edits same file in their worktree
- Both changes merge cleanly to main (no conflicts)

This is mycel's core innovation - true parallel development.

### Q: How do I handle merge conflicts?
**A:**
1. **Prevention**: Most conflicts prevented by worktree isolation
2. **Detection**: `bc merge list` shows conflicts
3. **Resolution**:
   ```bash
   # Manual resolution
   git checkout --theirs conflicted-file
   git add conflicted-file
   git commit -m "resolve conflict"
   ```
4. **Retry**: `bc merge process` to continue

### Q: What happens if an agent crashes?
**A:** No data loss due to git-backed persistence:
1. Agent state saved in `.bc/agents/agents.json`
2. Work queue saved in `.bc/queue.json`
3. Kill tmux session: `tmux kill-session -t bc-<agent>`
4. Restart agent: `bc spawn eng-01 --role engineer`
5. Agent continues from where it left off

### Q: Can agents communicate?
**A:** Yes, via channels:
```bash
# Send message
bc send eng-01 "Check your email for requirements"

# Send to channel
bc send #engineering "API ready for testing"

# View messages
bc logs eng-01
```

### Q: How do I review work before merging?
**A:**
```bash
# View what changed
git diff main..<agent-branch>

# View commits
git log main..<agent-branch> --oneline

# View in worktree
cd .bc/worktrees/eng-01/
git log --oneline

# Merge when ready
bc merge process
```

---

## Performance & Optimization

### Q: How do I optimize mycel for large teams?
**A:** Scaling strategies:
1. **Hierarchical teams**: PM → Managers → Engineers
   ```bash
   bc spawn pm-01 --role product-manager
   bc spawn mgr-01 --role manager --parent pm-01
   bc spawn eng-01 --role engineer --parent mgr-01
   ```

2. **Parallel work queues**: Assign multiple tasks
   ```bash
   bc queue assign work-0001 eng-01
   bc queue assign work-0002 eng-02
   bc queue assign work-0003 eng-03
   ```

3. **Dedicated roles**: QA, TechLead, etc.

### Q: What's the maximum number of agents?
**A:** Theoretically unlimited, practically:
- **Recommended**: 3-20 agents per workspace
- **Bottleneck**: Git repository size (becomes slow >1M files)
- **Tmux sessions**: Each requires minimal resources
- **Memory**: ~10MB per agent session

Test with your workload to find optimal team size.

### Q: How do I speed up mycel operations?
**A:**
1. **Shallow clone**: `git clone --depth 1` for faster setup
2. **Fewer agents**: Each agent = worktree checkout time
3. **Smaller repo**: Archive old branches
4. **Better disk**: SSD recommended for worktree operations
5. **Parallel tasks**: Use multiple agents

### Q: Does mycel support CI/CD integration?
**A:** Yes, mycel plays well with CI/CD:
- Export build artifacts from agent worktrees
- Trigger tests from merged commits
- Webhooks for completion events
- Works with GitHub Actions, GitLab CI, Jenkins, etc.

Example:
```bash
# Agent completes work
bc report done "Feature ready"

# GitHub Actions triggered by push to main
# CI/CD runs tests automatically
```

---

## Troubleshooting Common Issues

### Q: Agents are stuck (not running)
**A:** Check status:
```bash
bc status
# If shows "stuck", investigate:
bc logs eng-01  # View agent logs
ps aux | grep tmux  # Check tmux sessions
```

Solutions:
```bash
# Force restart
bc kill eng-01
bc spawn eng-01 --role engineer

# Or full reset
bc down
bc init
bc up
```

### Q: "Merge conflict" when trying to merge
**A:** Handle conflicting changes:
```bash
# View conflicts
bc merge list

# Resolve using git
cd .bc/worktrees/eng-01/
# Edit conflicted files
git add .
git commit -m "resolve conflicts"

# Retry merge
bc merge process
```

### Q: "Permission denied" errors
**A:**
```bash
# Check permissions
ls -la .bc/worktrees/

# Fix if needed
chmod -R 755 .bc/
chmod -R 755 .bc/worktrees/

# Retry command
bc status
```

### Q: tmux session won't start
**A:**
```bash
# Check if tmux running
tmux list-sessions

# Kill stale sessions
tmux kill-server

# Restart mycel
bc down
bc init
bc up
```

### Q: Agent can't access files
**A:** Agents only see their worktree:
```bash
# Agent's view (limited):
cd .bc/worktrees/eng-01/
ls  # Only files in worktree

# Main project view:
ls  # Full project files

# Solution: Copy needed files to worktree
cp -r src/ .bc/worktrees/eng-01/
```

---

## Advanced Questions

### Q: Can I use mycel with monorepos?
**A:** Yes, perfectly designed for monorepos:
- Each agent in separate worktree
- Can work on different packages simultaneously
- Worktree isolation prevents conflicts
- Merge strategy handles cross-package changes

### Q: Does mycel work with GitHub/GitLab?
**A:** Yes! mycel works with any git repository:
```bash
# Clone from GitHub
git clone https://github.com/yourorg/project.git
cd project

# Initialize mycel
bc init
bc up

# Work normally
# bc creates branches, agents work, merge to main
# Push to GitHub when ready
git push origin main
```

### Q: Can I use mycel in production?
**A:** Yes, with caveats:
- **Agents = Code Execution**: All agents can execute code
- **Security**: Restrict agent roles as needed
- **Audit**: Full event log for compliance
- **Testing**: QA agents should validate before merge
- **Approvals**: Tech leads should review before merge

Recommended production setup:
1. Dev team: Full access
2. Prod deployment: Manager/TechLead approval required
3. Monitoring: Watch merge queue for errors
4. Rollback: Easy via git history

### Q: How do I backup my bc workspace?
**A:**
```bash
# Git clone handles it automatically
# All state in .bc/ is git-tracked

# Manual backup
cp -r .mycel/ .mycel.backup

# Or use git
git commit -am "workspace snapshot"
git tag snapshot-$(date +%Y%m%d)
```

### Q: Can I integrate mycel with existing workflows?
**A:** Absolutely:
- mycel is agent-agnostic (works with Claude, Cursor, etc.)
- Agents use standard git commands
- Integrates with any CI/CD system
- Works alongside traditional development

---

## Contributing & Support

### Q: How do I contribute to mycel?
**A:** mycel is open source:
1. Fork: [bcinfra1/bc](https://github.com/bcinfra1/bc)
2. Branch: `git checkout -b feature/your-feature`
3. Implement improvements
4. Test thoroughly
5. Submit PR with description
6. Engage in review

### Q: Where can I report bugs?
**A:**
- [GitHub Issues](https://github.com/bcinfra1/bc/issues)
- Include: version, OS, steps to reproduce
- Attach logs: `bc logs`

### Q: How do I get help?
**A:**
- **Documentation**: [mycel GitHub Wiki](https://github.com/bcinfra1/bc/wiki)
- **Issues**: [Feature requests & bugs](https://github.com/bcinfra1/bc/issues)
- **Discussions**: [GitHub Discussions](https://github.com/bcinfra1/bc/discussions)
- **Community**: Ask in channels within bc workspace

---

## Glossary

| Term | Definition |
|------|-----------|
| **Agent** | AI instance (Claude Code) running in isolated tmux session |
| **Worktree** | Isolated copy of repo where agent works (`.bc/worktrees/<agent>/`) |
| **Workspace** | Project directory containing `.bc/` state directory |
| **Queue** | Work tracking system (`.bc/queue.json`) |
| **Channel** | Communication system for agent messaging |
| **Role** | Agent type (ProductManager, Manager, Engineer, QA) with capabilities |
| **Capability** | Action an agent is authorized to perform |
| **Merge** | Combining agent branch changes into main |
| **Tmux** | Terminal multiplexer managing agent sessions |
| **Persistence** | State surviving crashes via git-backed storage |

---

**Have a question not answered here?** Open an issue on [GitHub](https://github.com/bcinfra1/bc/issues) or ask in [Discussions](https://github.com/bcinfra1/bc/discussions).
