# mycel-cli

npm installer for [mycel](https://github.com/rpuneet/mycel) — a CLI-first AI agent orchestration system. mycel coordinates teams of Claude, Gemini, Cursor, Codex, and other AI agents working in isolated environments with per-agent git worktrees.

This package downloads the pre-built Go binary for your platform on `npm install`. No build tools or Go toolchain required.

## Install

```bash
npm install -g mycel-cli
```

Or run directly:

```bash
npx mycel-cli init
bunx mycel-cli init
```

## Quick start

```bash
# Start the daemon from your project — it bootstraps the workspace
mycel up

# Create an agent
mycel agent create --role engineer --provider claude

# Check status
mycel status
```

## Supported platforms

| OS    | Architecture | Archive                            |
|-------|-------------|------------------------------------|
| macOS | arm64       | `mycel_VERSION_darwin_arm64.tar.gz` |
| macOS | amd64       | `mycel_VERSION_darwin_amd64.tar.gz` |
| Linux | amd64       | `mycel_VERSION_linux_amd64.tar.gz`  |

## How it works

The `postinstall` script (`install.mjs`) runs after `npm install` and:

1. Detects your OS and CPU architecture
2. Fetches the latest release version from the GitHub API
3. Downloads the matching `mycel_VERSION_OS_ARCH.tar.gz` from [GitHub Releases](https://github.com/rpuneet/mycel/releases)
4. Extracts the `mycel` binary into `bin/mycel`
5. Verifies the binary runs

The script uses only Node.js built-ins (no dependencies). If the download fails, it exits cleanly so `npm install` doesn't break, and the placeholder `bin/mycel` script prints install instructions.

## Alternative install methods

If the npm postinstall doesn't work (corporate firewalls, CI restrictions, etc.):

```bash
# Homebrew (macOS)
brew install rpuneet/mycel/mycel

# From source
git clone https://github.com/rpuneet/mycel && cd mycel && make install-local-bc

# Direct download
# https://github.com/rpuneet/mycel/releases/latest
```

## Troubleshooting

**"mycel binary not installed"** — The postinstall script didn't run or failed. Run `node node_modules/mycel-cli/install.mjs` manually to see the error, or install via Homebrew/source.

**Permission denied** — The binary needs execute permission. Run `chmod +x node_modules/mycel-cli/bin/mycel`.

**Unsupported platform** — mycel provides pre-built binaries for macOS (amd64/arm64) and Linux (amd64). For other platforms, build from source.

## License

MIT
