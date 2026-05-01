# mycel Examples - Real-World Use Cases

Practical examples of mycel in action for common development scenarios.

## Table of Contents

1. [Startup MVP Development](#startup-mvp-development)
2. [Enterprise Microservices](#enterprise-microservices)
3. [Open Source Project Maintenance](#open-source-project-maintenance)
4. [SaaS Feature Development](#saas-feature-development)
5. [Mobile App Development](#mobile-app-development)
6. [Data Pipeline Development](#data-pipeline-development)
7. [API Gateway & Backends](#api-gateway--backends)

---

## Startup MVP Development

**Scenario:** 3-person startup building an MVP in 4 weeks to launch

### Team Structure
```bash
# Product Manager coordinates
bc agent create pm-01 --role product-manager

# Manager executes strategy
bc agent create mgr-01 --role manager --parent pm-01

# 2 Engineers + 1 QA
bc agent create eng-01 --role engineer --parent mgr-01  # Backend
bc agent create eng-02 --role engineer --parent mgr-01  # Frontend
bc agent create qa-01 --role qa --parent mgr-01         # Testing
```

### Week 1: User Authentication
```bash
# PM creates epic
bc queue add "Week 1: User Authentication System"

# Break into tasks
bc queue add "Backend: JWT auth endpoints" --priority high
bc queue add "Frontend: Login/Signup UI" --priority high
bc queue add "Test: Auth flow end-to-end" --priority high

# Assign parallel work
bc queue assign work-0001 eng-01  # Backend engineer
bc queue assign work-0002 eng-02  # Frontend engineer
bc queue assign work-0003 qa-01   # QA engineer

# Monitor progress
bc home
# Both engineers work simultaneously in isolated worktrees
# Zero merge conflicts even though working on same auth system
```

### Parallelization Benefits
```
Timeline WITHOUT mycel (sequential):
Day 1-2: Backend builds JWT auth (eng-01 works)
Day 3-4: Frontend builds login UI (waits for API)
Day 5-6: Integration & testing
Day 7: Deploy

Timeline WITH mycel (parallel):
Day 1-3: Backend (eng-01) + Frontend (eng-02) work simultaneously
Day 4: QA tests both in parallel
Day 5: Deploy

Result: 2-day acceleration, same team size
```

### Code Example: Parallel Development
```bash
# eng-01 working on authentication service
cd .bc/worktrees/eng-01/
git checkout -b feature/jwt-auth
# Implement:
# - POST /auth/login
# - POST /auth/register
# - Middleware for token validation
git commit -m "feat: JWT authentication service"

# Meanwhile, eng-02 working on frontend (simultaneously, no conflicts)
cd .bc/worktrees/eng-02/
git checkout -b feature/auth-ui
# Implement:
# - Login form component
# - Signup form component
# - Session management
git commit -m "feat: authentication UI"

# Both merge to main without conflicts
bc merge process
```

---

## Enterprise Microservices

**Scenario:** 15-person team building distributed system with 5 microservices

### Architecture
```
┌─────────────────────────────────────────────────┐
│         Root (Product Manager)                  │
└────────────┬──────────────────────────────────────┘
             │
    ┌────────┼────────┬──────────┬──────────┐
    ▼        ▼        ▼          ▼          ▼
  PM-01    PM-02    PM-03      PM-04     PM-05
  (User    (Order   (Payment   (Inventory (Analytics
   Svc)    Svc)     Svc)       Svc)      Svc)

Each PM spawns 3 engineers + 1 QA
```

### Initialization
```bash
# Root coordinates
bc agent create root-01 --role product-manager

# 5 service managers
bc agent create pm-user --role manager --parent root-01
bc agent create pm-order --role manager --parent root-01
bc agent create pm-payment --role manager --parent root-01
bc agent create pm-inventory --role manager --parent root-01
bc agent create pm-analytics --role manager --parent root-01

# Each manager spawns engineers
for service in user order payment inventory analytics; do
  bc agent create eng-${service}-01 --role engineer --parent pm-${service}
  bc agent create eng-${service}-02 --role engineer --parent pm-${service}
  bc agent create qa-${service}-01 --role qa --parent pm-${service}
done

# Result: 15 agents in organized hierarchy
```

### Sprint Planning
```bash
# All 5 services developing in parallel
bc queue add "Sprint 23: User service - add profile updates"
bc queue add "Sprint 23: Order service - implement cancellation"
bc queue add "Sprint 23: Payment service - add refund logic"
bc queue add "Sprint 23: Inventory service - sync across regions"
bc queue add "Sprint 23: Analytics service - track user journey"

# Each service team executes independently
bc queue assign work-0001 eng-user-01
bc queue assign work-0002 eng-order-01
bc queue assign work-0003 eng-payment-01
bc queue assign work-0004 eng-inventory-01
bc queue assign work-0005 eng-analytics-01

# Managers review cross-service dependencies
bc send pm-order "Need updated user IDs from user-service"
bc send pm-user "User updates ready for order service"

# All merge simultaneously when ready
bc merge process
# Result: 5 microservices evolved, deployed together
```

### Zero Conflicts at Scale
```
Traditional Approach:
- 5 teams, each modifying shared interfaces
- Constant merge conflicts in API contracts
- Manual conflict resolution
- Integration testing nightmares
- 2-week integration cycle

mycel Approach:
- 5 teams in isolated worktrees
- No merge conflicts in code
- API contracts defined upfront
- Contract tests run per-service
- Integration automatic on merge
- Daily integration cycle
```

---

## Open Source Project Maintenance

**Scenario:** Maintaining popular library with 10 maintainers + 20 issue contributors

### Team Roles
```bash
# Core maintainers (permanent)
bc agent create maintainer-01 --role product-manager  # Lead
bc agent create maintainer-02 --role manager
bc agent create maintainer-03 --role manager

# Triage: QA role
bc agent create qa-triage-01 --role qa

# Contributor engineers (rotating)
bc agent create contributor-01 --role engineer
bc agent create contributor-02 --role engineer
# ... etc
```

### Issue Processing
```bash
# Issues come in from GitHub
bc queue add "Fix: Memory leak in parser (Issue #542)"
bc queue add "Feature: Add TypeScript support (Issue #438)"
bc queue add "Docs: Update API reference"
bc queue add "Perf: Optimize critical path"
bc queue add "Test: Add Windows CI support"

# Triage: High priority issues
bc send qa-triage-01 "Review high-priority issues"

# Assign to available contributors
bc queue assign work-0001 contributor-01  # Memory leak
bc queue assign work-0002 contributor-02  # TypeScript
bc queue assign work-0003 contributor-03  # Docs

# Monitor contributions
bc home
# Shows: 3 contributors working, 2 pending review
```

### Maintainer Workflow
```bash
# Contributors work independently
bc attach contributor-01
# Implements memory leak fix
git commit -m "fix: memory leak in parser"

# Maintainer reviews
bc merge list
# Shows: contributor-01 work ready for review

# Code review via GitHub
# + automated CI/CD testing

# Maintainer approves
bc merge process
# Merges to main

# Auto-publish to npm
# Update released
```

### Benefits for OSS
```
✓ Multiple contributors work simultaneously
✓ No contributor conflicts (worktree isolation)
✓ Automatic version management
✓ Release coordination automatic
✓ Contributor motivation (fast merge)
✓ Project velocity increases
```

---

## SaaS Feature Development

**Scenario:** SaaS product with 2-week sprint cycles, 8 engineers

### Sprint Board as Work Queue
```bash
# Sprint 15 created
bc queue add "Feature: Dark mode support" --priority high
bc queue add "Feature: Bulk export CSV" --priority high
bc queue add "Feature: Custom dashboard layouts" --priority medium
bc queue add "Fix: Email notification delay" --priority high
bc queue add "Perf: Optimize database queries" --priority medium
bc queue add "Docs: Update API v2 docs" --priority low
bc queue add "Test: Load testing for 1M users" --priority medium
bc queue add "Infra: CDN optimization" --priority medium

# Daily standup shows progress
bc queue list
# Shows: 3 done, 2 working, 3 pending

# Sprint metrics
bc metrics
# Shows: 8 items, 5 completed, on track for Friday release
```

### Continuous Integration
```bash
# Multiple engineers working on different features
bc agent create eng-01 --role engineer  # Dark mode
bc agent create eng-02 --role engineer  # Bulk export
bc agent create eng-03 --role engineer  # Dashboards
bc agent create eng-04 --role engineer  # Email fix
bc agent create eng-05 --role engineer  # Performance
bc agent create eng-06 --role engineer  # Documentation
bc agent create qa-01 --role qa         # Testing
bc agent create tech-lead-01 --role tech-lead

# Wednesday: Merge features to staging
bc merge process --to staging
# Result: All features integrated on staging branch

# Thursday: Final testing on staging
bc send qa-01 "Run full test suite on staging"

# Friday: Release to production
bc merge process --to main
# Automatic CI/CD deploys
```

---

## Mobile App Development

**Scenario:** Cross-platform mobile app (iOS + Android) with 6 engineers

### Team Specialization
```bash
# 3 iOS engineers
bc agent create ios-lead --role manager
bc agent create ios-eng-01 --role engineer --parent ios-lead
bc agent create ios-eng-02 --role engineer --parent ios-lead
bc agent create ios-qa --role qa --parent ios-lead

# 3 Android engineers
bc agent create android-lead --role manager
bc agent create android-eng-01 --role engineer --parent android-lead
bc agent create android-eng-02 --role engineer --parent android-lead
bc agent create android-qa --role qa --parent android-lead

# Shared features (backend)
bc agent create backend-lead --role manager
bc agent create backend-eng --role engineer --parent backend-lead
```

### Parallel Platform Development
```bash
# iOS implementing user auth
cd .bc/worktrees/ios-eng-01/
# Implement: BiometricAuth.swift, LoginViewController.swift
# No conflicts with Android engineers

# Android implementing user auth
cd .bc/worktrees/android-eng-01/
# Implement: BiometricAuthManager.java, LoginActivity.java
# No conflicts with iOS engineers

# Backend implementing endpoints
cd .bc/worktrees/backend-eng/
# Implement: /auth/login, /auth/register endpoints
# Used by both platforms

# All merge to main simultaneously
bc merge process
# Result: Complete auth system integrated across platforms
```

### Platform-Specific Workflow
```
iOS Team:
- Xcode development in iOS worktree
- Swift code, SwiftUI components
- iOS-specific testing

Android Team:
- Android Studio development in Android worktree
- Kotlin code, Jetpack components
- Android-specific testing

Result:
- Both teams work independently
- Zero code conflicts (different languages)
- Both benefit from shared backend
- Fast parallel development
```

---

## Data Pipeline Development

**Scenario:** ETL system with data, analytics, and infrastructure engineers

### Architecture
```
Data Ingestion → Transformation → Storage → Analytics
(Eng-01)        (Eng-02)        (Eng-03)  (Eng-04)
```

### Parallel Pipeline Development
```bash
# Each stage has owner
bc agent create eng-01 --role engineer  # Ingestion (APIs, webhooks)
bc agent create eng-02 --role engineer  # Transformation (cleaning, enrichment)
bc agent create eng-03 --role engineer  # Storage (DB, data warehouse)
bc agent create eng-04 --role engineer  # Analytics (dashboards, reports)
bc agent create qa-01 --role qa         # Data quality testing

# Sprint: Add email interaction tracking
bc queue add "Ingest email events from SendGrid"
bc queue add "Transform email events for analytics"
bc queue add "Store email events in warehouse"
bc queue add "Build email interaction dashboard"

# Parallel development
bc queue assign work-0001 eng-01  # Ingestion: Accept SendGrid webhooks
bc queue assign work-0002 eng-02  # Transform: Extract fields
bc queue assign work-0003 eng-03  # Store: Create table, indexes
bc queue assign work-0004 eng-04  # Analytics: Create visualizations
```

### Data Quality Validation
```bash
# QA creates validation rules
bc send qa-01 "Validate email tracking:"
bc send qa-01 "- No duplicates"
bc send qa-01 "- Timestamps valid"
bc send qa-01 "- All required fields present"

# Each stage validates output
eng-01 output: 10,000 events ingested ✓
eng-02 output: 10,000 events transformed ✓
eng-03 output: 10,000 events stored ✓
eng-04: Ready to visualize ✓

# Monitor data flow
bc home
# Shows: 4 engineers working on pipeline stages
```

---

## API Gateway & Backends

**Scenario:** Building API platform with 4 backend services + gateway

### Microservice Coordination
```bash
# API Gateway
bc agent create gateway-eng --role engineer

# 4 Backend Services
bc agent create auth-service-eng --role engineer
bc agent create users-service-eng --role engineer
bc agent create products-service-eng --role engineer
bc agent create orders-service-eng --role engineer

# Shared utilities
bc agent create utils-eng --role engineer

# Infrastructure
bc agent create ops-eng --role engineer
```

### Contract-First API Development
```bash
# Week 1: Define API contracts
bc queue add "Define Auth service API contract"
bc queue add "Define Users service API contract"
bc queue add "Define Products service API contract"
bc queue add "Define Orders service API contract"

# Week 2: Implement services in parallel
bc queue add "Auth: Implement JWT endpoints"
bc queue add "Users: Implement user CRUD"
bc queue add "Products: Implement product catalog"
bc queue add "Orders: Implement order pipeline"

# Week 3: Implement gateway routing
bc queue add "Gateway: Route to auth service"
bc queue add "Gateway: Route to users service"
bc queue add "Gateway: Route to products service"
bc queue add "Gateway: Route to orders service"

# Parallel implementation
bc queue assign work-0008 auth-service-eng
bc queue assign work-0009 users-service-eng
bc queue assign work-0010 products-service-eng
bc queue assign work-0011 orders-service-eng
```

### Contract Verification
```bash
# Contract tests ensure API compatibility
bc send qa-01 "Run contract tests:"
bc send qa-01 "- Auth service returns JWT tokens"
bc send qa-01 "- User service returns user objects"
bc send qa-01 "- Product service returns product list"
bc send qa-01 "- Order service returns order confirmation"

# Each service validates its output
# Gateway integration automatic on merge
```

---

## Lessons Across Examples

### Pattern 1: Hierarchy for Scale
```
1-5 people: Single manager level
5-10 people: Manager + team leads
10+ people: Root PM → Managers → Team leads → Engineers
```

### Pattern 2: Parallel When Possible
```
✓ Use parallel work for:
  - Different features
  - Different services
  - Different platforms
  - Different code layers

✗ Don't parallelize:
  - Shared infrastructure
  - Blocking dependencies
```

### Pattern 3: Communication Cadence
```
Daily:
- bc home (dashboard check)
- bc queue list (progress)

Weekly:
- Sprint planning (new tasks)
- Merge review (code quality)

Per-release:
- Final testing
- Deployment coordination
```

### Pattern 4: Merge Strategy
```
Small teams (3-5 people):
- Merge after every task

Medium teams (5-15 people):
- Merge daily or twice daily
- Stagger across time zones

Large teams (15+ people):
- Merge per service
- Coordinate across services
```

---

## Performance Metrics from Examples

### Startup (3 people, 4 weeks)
```
Without mycel: 4 weeks sequential → Launch Day 28
With mycel: 2.5 weeks parallel → Launch Day 18

Result: 10 days faster to market
```

### Enterprise (15 people, microservices)
```
Without mycel: 1 feature per 2 weeks
With mycel: 5 features per 2 weeks (parallel services)

Result: 5x feature velocity
```

### OSS (10 maintainers, 50 issues)
```
Without mycel: 3-month backlog
With mycel: 2-week backlog (parallel contributors)

Result: 6x faster issue resolution
```

---

## Common Success Factors

1. **Clear Task Definition** - Each task must be discrete and independent
2. **Upfront Planning** - Define dependencies before assigning work
3. **Regular Synchronization** - Daily standup via mycel dashboard
4. **Code Review** - Tech leads review work before merge
5. **Automated Testing** - CI/CD validates all merged changes
6. **Communication** - Use channels for coordination, not interruption

---

## Anti-Patterns to Avoid

❌ **Assigning 1 task to 5 people** - Causes thrashing, no parallelization
❌ **Ignoring dependencies** - Creates merge conflicts despite worktrees
❌ **No code review** - Bugs merge to main undetected
❌ **Overloading agents** - Agent stuck with no progress reporting
❌ **Skipping tests** - Broken merges accumulate over time

---

**Next:** See [Getting Started](./getting-started.md) to implement these patterns in your project.
