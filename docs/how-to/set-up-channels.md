# Channel Message Conventions

> **Superseded**: This document has been superseded by [docs/architecture/channels.md](../architecture/channels.md).
> The PR workflow conventions below remain valid reference material.
> The channel system itself is now **inbound notification-only** -- see the architecture doc for details.

This document defines the standard message formats for bc's channel-based PR workflow.

## Channel Types

### #all
Broadcast channel for system-wide announcements and nudges.

### #reviews
PR workflow channel for review requests, approvals, and merge notifications.

## Message Formats

### Review Request

```
@<target> PR #<number> ready for review
```

**Examples:**
- `@tech-lead PR #123 ready for review`
- `@tech-lead-01 PR #456 ready for review: Add user authentication`

**With URL:**
- `@tech-lead PR #789 ready for review: https://github.com/org/repo/pull/789`

### Approval

```
PR #<number> approved [checkmark]
LGTM PR #<number>
PR #<number> looks good
```

**Examples:**
- `PR #123 approved`
- `LGTM PR #456`
- `PR #789 looks good to me`

### Changes Requested

```
PR #<number> needs changes: <reason>
PR #<number> please fix <issue>
```

**Examples:**
- `PR #123 needs changes: fix the failing tests`
- `PR #456 please fix the formatting issues`

### Merge Notification

```
PR #<number> merged to <branch>
Merged PR #<number>
```

**Examples:**
- `PR #123 merged to main`
- `Merged PR #456`
- `PR #789 pushed to develop`

## Parsing Rules

### PR Number Extraction
PR numbers are extracted using pattern: `(?i)(?:pr\s*#?|#)(\d+)`
- Matches: `PR #123`, `PR 123`, `#123`, `pr#456`
- Does NOT match standalone numbers like agent IDs: `tech-lead-01`

### @Mentions
Mentions are extracted using pattern: `@([a-zA-Z0-9_-]+)`
- Matches: `@tech-lead`, `@tech-lead-01`, `@user_name`

### Approval Detection
Approvals are detected by these patterns (case-insensitive):
- `approved`
- `lgtm`
- `looks good`
- `ship it`

### Changes Requested Detection
Change requests are detected by these patterns (case-insensitive):
- `needs changes`
- `changes requested`
- `please fix`
- `needs work`

### Merge Detection
Merges are detected by these patterns (case-insensitive):
- `merged`
- `merge to`
- `pushed to`

## Workflow

1. **Engineer** opens PR, posts to #reviews:
   ```
   @tech-lead-01 PR #123 ready for review
   ```

2. **Tech Lead** reviews and responds:
   - Approved: `PR #123 approved`
   - Needs work: `PR #123 needs changes: please add tests`

3. **Automation** detects approval, notifies manager:
   ```
   @manager PR #123 approved by tech-lead-01 - ready to merge to main
   ```

4. **Manager** merges and posts:
   ```
   PR #123 merged to main
   ```

## Message Types

| Type | Code | Description |
|------|------|-------------|
| text | `TypeText` | Regular conversation |
| task | `TypeTask` | Work assignment with @mention |
| review | `TypeReview` | PR review request |
| approval | `TypeApproval` | Tech lead approval |
| merge | `TypeMerge` | Merge request/notification |
| status | `TypeStatus` | Agent status update |

## Code Reference

The parsing and formatting functions are in `pkg/channel/`:
- `review_request.go` - Review request parsing/formatting
- `approval_message.go` - Approval and merge message handling
- `automation.go` - Approval-to-merge workflow automation
- `message_type.go` - Message type definitions
