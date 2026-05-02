# Code Coverage Standards

## Overview
mycel maintains code quality through continuous coverage enforcement. All PRs must meet coverage thresholds before merge.

## Coverage Requirements

### Current Minimum: 60%
All pull requests must maintain or improve overall code coverage to **60% or higher**.

**Current Status**: 62.7%
**Target**: 90%+

### Roadmap
| Phase | Threshold | Status |
|-------|-----------|--------|
| Phase 1 | 60% | **Enforced in CI** |
| Phase 2 | 70% | Planned |
| Phase 3 | 80% | Planned |
| Phase 4 | 90%+ | Target |

### Per-Package Guidelines (future)

| Category | Target | Examples |
|----------|--------|----------|
| Critical Infrastructure | 95%+ | agent, tmux, channel, db |
| Core Features | 85%+ | cost, cron, workspace |
| Support Packages | 80%+ | log, names, stats, ui |
| CLI Commands | 70%+ | internal/cmd |

## Measuring Coverage

```bash
# Run coverage locally
make coverage-go

# View by function
go tool cover -func=coverage.out

# View in browser
go tool cover -html=coverage.out
```

## CI Enforcement

- **Location**: `.github/workflows/ci.yml`
- **Step**: Check coverage threshold
- **Action**: Fail PR if below 60%

---

**Last Updated**: 2026-03-21
