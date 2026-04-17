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
mycel spawn pm-01 --role product-manager

# Manager executes strategy
mycel spawn mgr-01 --role manager --parent pm-01

# 2 Engineers + 1 QA
mycel spawn eng-01 --role engineer --parent mgr-01  # Backend
mycel spawn eng-02 --role engineer --parent mgr-01  # Frontend
mycel spawn qa-01 --role qa --parent mgr-01         # Testing
```

### Week 1: User Authentication
```bash
# PM creates epic
mycel queue add "Week 1: User Authentication System"

# Break into tasks
mycel queue add "Backend: JWT auth endpoints" --priority high
mycel queue add "Frontend: Login/Signup UI" --priority high
mycel queue add "Test: Auth flow end-to-end" --priority high

# Assign parallel work
mycel queue assign work-0001 eng-01  # Backend engineer
mycel queue assign work-0002 eng-02  # Frontend engineer
mycel queue assign work-0003 qa-01   # QA engineer

# Monitor progress
mycel home
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
cd .mycel/worktrees/eng-01/
git checkout -b feature/jwt-auth
# Implement:
# - POST /auth/login
# - POST /auth/register
# - Middleware for token validation
git commit -m "feat: JWT authentication service"

# Meanwhile, eng-02 working on frontend (simultaneously, no conflicts)
cd .mycel/worktrees/eng-02/
git checkout -b feature/auth-ui
# Implement:
# - Login form component
# - Signup form component
# - Session management
git commit -m "feat: authentication UI"

# Both merge to main without conflicts
mycel merge process
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
mycel spawn root-01 --role product-manager

# 5 service managers
mycel spawn pm-user --role manager --parent root-01
mycel spawn pm-order --role manager --parent root-01
mycel spawn pm-payment --role manager --parent root-01
mycel spawn pm-inventory --role manager --parent root-01
mycel spawn pm-analytics --role manager --parent root-01

# Each manager spawns engineers
for service in user order payment inventory analytics; do
  mycel spawn eng-${service}-01 --role engineer --parent pm-${service}
  mycel spawn eng-${service}-02 --role engineer --parent pm-${service}
  mycel spawn qa-${service}-01 --role qa --parent pm-${service}
done

# Result: 15 agents in organized hierarchy
```

### Sprint Planning
```bash
# All 5 services developing in parallel
mycel queue add "Sprint 23: User service - add profile updates"
mycel queue add "Sprint 23: Order service - implement cancellation"
mycel queue add "Sprint 23: Payment service - add refund logic"
mycel queue add "Sprint 23: Inventory service - sync across regions"
mycel queue add "Sprint 23: Analytics service - track user journey"

# Each service team executes independently
mycel queue assign work-0001 eng-user-01
mycel queue assign work-0002 eng-order-01
mycel queue assign work-0003 eng-payment-01
mycel queue assign work-0004 eng-inventory-01
mycel queue assign work-0005 eng-analytics-01

# Managers review cross-service dependencies
mycel send pm-order "Need updated user IDs from user-service"
mycel send pm-user "User updates ready for order service"

# All merge simultaneously when ready
mycel merge process
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
mycel spawn maintainer-01 --role product-manager  # Lead
mycel spawn maintainer-02 --role manager
mycel spawn maintainer-03 --role manager

# Triage: QA role
mycel spawn qa-triage-01 --role qa

# Contributor engineers (rotating)
mycel spawn contributor-01 --role engineer
mycel spawn contributor-02 --role engineer
# ... etc
```

### Issue Processing
```bash
# Issues come in from GitHub
mycel queue add "Fix: Memory leak in parser (Issue #542)"
mycel queue add "Feature: Add TypeScript support (Issue #438)"
mycel queue add "Docs: Update API reference"
mycel queue add "Perf: Optimize critical path"
mycel queue add "Test: Add Windows CI support"

# Triage: High priority issues
mycel send qa-triage-01 "Review high-priority issues"

# Assign to available contributors
mycel queue assign work-0001 contributor-01  # Memory leak
mycel queue assign work-0002 contributor-02  # TypeScript
mycel queue assign work-0003 contributor-03  # Docs

# Monitor contributions
mycel home
# Shows: 3 contributors working, 2 pending review
```

### Maintainer Workflow
```bash
# Contributors work independently
mycel attach contributor-01
# Implements memory leak fix
git commit -m "fix: memory leak in parser"

# Maintainer reviews
mycel merge list
# Shows: contributor-01 work ready for review

# Code review via GitHub
# + automated CI/CD testing

# Maintainer approves
mycel merge process
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
mycel queue add "Feature: Dark mode support" --priority high
mycel queue add "Feature: Bulk export CSV" --priority high
mycel queue add "Feature: Custom dashboard layouts" --priority medium
mycel queue add "Fix: Email notification delay" --priority high
mycel queue add "Perf: Optimize database queries" --priority medium
mycel queue add "Docs: Update API v2 docs" --priority low
mycel queue add "Test: Load testing for 1M users" --priority medium
mycel queue add "Infra: CDN optimization" --priority medium

# Daily standup shows progress
mycel queue list
# Shows: 3 done, 2 working, 3 pending

# Sprint metrics
mycel metrics
# Shows: 8 items, 5 completed, on track for Friday release
```

### Continuous Integration
```bash
# Multiple engineers working on different features
mycel spawn eng-01 --role engineer  # Dark mode
mycel spawn eng-02 --role engineer  # Bulk export
mycel spawn eng-03 --role engineer  # Dashboards
mycel spawn eng-04 --role engineer  # Email fix
mycel spawn eng-05 --role engineer  # Performance
mycel spawn eng-06 --role engineer  # Documentation
mycel spawn qa-01 --role qa         # Testing
mycel spawn tech-lead-01 --role tech-lead

# Wednesday: Merge features to staging
mycel merge process --to staging
# Result: All features integrated on staging branch

# Thursday: Final testing on staging
mycel send qa-01 "Run full test suite on staging"

# Friday: Release to production
mycel merge process --to main
# Automatic CI/CD deploys
```

---

## Mobile App Development

**Scenario:** Cross-platform mobile app (iOS + Android) with 6 engineers

### Team Specialization
```bash
# 3 iOS engineers
mycel spawn ios-lead --role manager
mycel spawn ios-eng-01 --role engineer --parent ios-lead
mycel spawn ios-eng-02 --role engineer --parent ios-lead
mycel spawn ios-qa --role qa --parent ios-lead

# 3 Android engineers
mycel spawn android-lead --role manager
mycel spawn android-eng-01 --role engineer --parent android-lead
mycel spawn android-eng-02 --role engineer --parent android-lead
mycel spawn android-qa --role qa --parent android-lead

# Shared features (backend)
mycel spawn backend-lead --role manager
mycel spawn backend-eng --role engineer --parent backend-lead
```

### Parallel Platform Development
```bash
# iOS implementing user auth
cd .mycel/worktrees/ios-eng-01/
# Implement: BiometricAuth.swift, LoginViewController.swift
# No conflicts with Android engineers

# Android implementing user auth
cd .mycel/worktrees/android-eng-01/
# Implement: BiometricAuthManager.java, LoginActivity.java
# No conflicts with iOS engineers

# Backend implementing endpoints
cd .mycel/worktrees/backend-eng/
# Implement: /auth/login, /auth/register endpoints
# Used by both platforms

# All merge to main simultaneously
mycel merge process
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
mycel spawn eng-01 --role engineer  # Ingestion (APIs, webhooks)
mycel spawn eng-02 --role engineer  # Transformation (cleaning, enrichment)
mycel spawn eng-03 --role engineer  # Storage (DB, data warehouse)
mycel spawn eng-04 --role engineer  # Analytics (dashboards, reports)
mycel spawn qa-01 --role qa         # Data quality testing

# Sprint: Add email interaction tracking
mycel queue add "Ingest email events from SendGrid"
mycel queue add "Transform email events for analytics"
mycel queue add "Store email events in warehouse"
mycel queue add "Build email interaction dashboard"

# Parallel development
mycel queue assign work-0001 eng-01  # Ingestion: Accept SendGrid webhooks
mycel queue assign work-0002 eng-02  # Transform: Extract fields
mycel queue assign work-0003 eng-03  # Store: Create table, indexes
mycel queue assign work-0004 eng-04  # Analytics: Create visualizations
```

### Data Quality Validation
```bash
# QA creates validation rules
mycel send qa-01 "Validate email tracking:"
mycel send qa-01 "- No duplicates"
mycel send qa-01 "- Timestamps valid"
mycel send qa-01 "- All required fields present"

# Each stage validates output
eng-01 output: 10,000 events ingested ✓
eng-02 output: 10,000 events transformed ✓
eng-03 output: 10,000 events stored ✓
eng-04: Ready to visualize ✓

# Monitor data flow
mycel home
# Shows: 4 engineers working on pipeline stages
```

---

## API Gateway & Backends

**Scenario:** Building API platform with 4 backend services + gateway

### Microservice Coordination
```bash
# API Gateway
mycel spawn gateway-eng --role engineer

# 4 Backend Services
mycel spawn auth-service-eng --role engineer
mycel spawn users-service-eng --role engineer
mycel spawn products-service-eng --role engineer
mycel spawn orders-service-eng --role engineer

# Shared utilities
mycel spawn utils-eng --role engineer

# Infrastructure
mycel spawn ops-eng --role engineer
```

### Contract-First API Development
```bash
# Week 1: Define API contracts
mycel queue add "Define Auth service API contract"
mycel queue add "Define Users service API contract"
mycel queue add "Define Products service API contract"
mycel queue add "Define Orders service API contract"

# Week 2: Implement services in parallel
mycel queue add "Auth: Implement JWT endpoints"
mycel queue add "Users: Implement user CRUD"
mycel queue add "Products: Implement product catalog"
mycel queue add "Orders: Implement order pipeline"

# Week 3: Implement gateway routing
mycel queue add "Gateway: Route to auth service"
mycel queue add "Gateway: Route to users service"
mycel queue add "Gateway: Route to products service"
mycel queue add "Gateway: Route to orders service"

# Parallel implementation
mycel queue assign work-0008 auth-service-eng
mycel queue assign work-0009 users-service-eng
mycel queue assign work-0010 products-service-eng
mycel queue assign work-0011 orders-service-eng
```

### Contract Verification
```bash
# Contract tests ensure API compatibility
mycel send qa-01 "Run contract tests:"
mycel send qa-01 "- Auth service returns JWT tokens"
mycel send qa-01 "- User service returns user objects"
mycel send qa-01 "- Product service returns product list"
mycel send qa-01 "- Order service returns order confirmation"

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
- mycel home (dashboard check)
- mycel queue list (progress)

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
