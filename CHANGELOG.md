# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.3] - 2026-05-02

### Added
- SDK runner skeleton (`packages/mycel-agent-runner`) — Phase 1 of the Claude Agent SDK migration; thin HTTP wrapper exposing one Claude agent over REST + SSE (#2990).
- Mycel-branded landing site at mycel.dev (#3004).

### Changed
- Rebranded `bc` to `mycel` across the binary (`bin/mycel`), release tarballs (`mycel_*`), npm package (`mycel-cli`), Go module (`github.com/rpuneet/mycel`), and install paths (#3053, #3059, #3060, #3062).
- Restored the original particle background animation on the landing site (#3055).
- Backend cleanup: SOLID refactor of server services, dedicated error types, config split, repo_root plumbing, and broad context propagation across notify/cron/tool/doctor/provider/agent stores (#3038, #3046).
- Test isolation: integration tests in `internal/cmd` now use an in-process `httptest.Server` instead of a live bcd, eliminating cross-test interference (#3056).
- CI: relaxed conventional-commits regex to allow `deps(...)` prefix (#3051).
- Dependency bumps across web/tui/landing: TypeScript 6, ESLint 10, react-router-dom 7, GitHub Actions group, @types/node, and lockfile regeneration to unblock CD (#3018, #3020, #3021, #3022, #3023, #3024, #3025, #3028, #3039, #3050).

### Fixed
- MCP sender spoof vulnerability and SSE CORS wildcard tightened (#2967, #2960, #3048).
- MCP tool input fields now have length caps to prevent abuse (#2961, #3045).
- Cron commands run in their own process group so they cannot accidentally signal-kill bcd (#2964, #3052).
- `useEffect` dependency for `selectTab` in `AgentDetail` corrected (#3044).
- Stale `RevealSection` import and vitals TODO removed from landing (#3043).

### Security
- MCP sender spoof patched (#2967).
- SSE CORS wildcard replaced with scoped allowlist (#2960).
- MCP tool input length caps mitigate resource-exhaustion abuse (#2961).
