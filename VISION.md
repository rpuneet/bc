# Vision

## Multi-Agent Orchestration for Software Development

**mycel** is a CLI tool for orchestrating multiple AI agents to work together on software projects. Unlike single-agent tools, mycel enables teams of specialized agents coordinating through structured communication.

## Core Philosophy

### Agents as Team Members
Each agent operates as an isolated team member with:
- **Dedicated worktree**: Isolated git workspace preventing conflicts
- **Defined role**: Engineer, manager, reviewer, etc.
- **Clear capabilities**: What actions each role can perform
- **Persistent memory**: Learnings retained across sessions

### Structured Collaboration
Agents communicate through channels, not chaos:
- **#eng**: Engineering discussions and blockers
- **#pr**: Pull request announcements and reviews
- **#standup**: Async status updates
- **#leads**: Escalations requiring decisions

### Human Oversight
Humans remain in control:
- Set budgets and spending limits
- Define roles and capabilities
- Review and merge PRs
- Monitor agent activity via TUI

## Unique Differentiators

| Feature | mycel | Single-Agent Tools |
|---------|----|--------------------|
| Multiple parallel agents | Yes | No |
| Role-based hierarchy | Yes | No |
| Inter-agent communication | Yes | No |
| Git worktree isolation | Yes | No |
| Cost tracking per agent | Yes | Limited |
| Persistent agent memory | Yes | Session-only |

## Roadmap

### Near-term
- [ ] Homebrew installation
- [ ] Demo GIF/video for README
- [ ] Plugin system for custom roles
- [ ] WebSocket-based agent monitoring

### Medium-term
- [ ] Multi-repository orchestration
- [ ] Agent templates and presets
- [ ] Integration with CI/CD pipelines
- [ ] Cost optimization recommendations

### Long-term
- [ ] Distributed agent execution
- [ ] Cross-team agent collaboration
- [ ] ML-based agent capability matching
- [ ] Enterprise features (SSO, audit logs)

## Design Principles

1. **Isolation by default**: Agents cannot interfere with each other
2. **Explicit communication**: All inter-agent messages are logged
3. **Cost awareness**: Every action has tracked cost
4. **Human-in-the-loop**: Critical decisions require human approval
5. **Simplicity**: CLI-first, no complex infrastructure required

## Getting Involved

mycel is open source. Contributions welcome:
- Report issues on GitHub
- Submit PRs for features and fixes
- Share feedback on multi-agent workflows
- Help improve documentation

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.
