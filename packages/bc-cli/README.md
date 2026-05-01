# bc-cli

npm installer for [bc](https://github.com/rpuneet/mycel) — a CLI-first AI agent orchestration system. bc coordinates teams of Claude, Gemini, Cursor, Codex, and other AI agents working in isolated environments with per-agent git worktrees.

This package downloads the pre-built Go binary for your platform on `npm install`. No build tools or Go toolchain required.

## Install

```bash
npm install -g bc-cli
```

Or run directly:

```bash
npx bc-cli init
bunx bc-cli init
```

## Quick start

```bash
# Initialize a workspace in your project
bc init

# Start the daemon
bc up

# Create an agent
bc agent create --role engineer --provider claude

# Check status
bc status
```

## Supported platforms

| OS    | Architecture | Archive                        |
|-------|-------------|--------------------------------|
| macOS | arm64       | `bc_VERSION_darwin_arm64.tar.gz` |
| macOS | amd64       | `bc_VERSION_darwin_amd64.tar.gz` |
| Linux | amd64       | `bc_VERSION_linux_amd64.tar.gz`  |

## How it works

The `postinstall` script (`install.mjs`) runs after `npm install` and:

1. Detects your OS and CPU architecture
2. Fetches the latest release version from the GitHub API
3. Downloads the matching `bc_VERSION_OS_ARCH.tar.gz` from [GitHub Releases](https://github.com/rpuneet/mycel/releases)
4. Extracts the `bc` binary into `bin/bc`
5. Verifies the binary runs

The script uses only Node.js built-ins (no dependencies). If the download fails, it exits cleanly so `npm install` doesn't break, and the placeholder `bin/bc` script prints install instructions.

## Alternative install methods

If the npm postinstall doesn't work (corporate firewalls, CI restrictions, etc.):

```bash
# Homebrew (macOS)
brew install rpuneet/mycel/bc-infra

# From source
git clone https://github.com/rpuneet/mycel && cd bc && make install-local-bc

# Direct download
# https://github.com/rpuneet/mycel/releases/latest
```

## Troubleshooting

**"bc binary not installed"** — The postinstall script didn't run or failed. Run `node node_modules/bc-cli/install.mjs` manually to see the error, or install via Homebrew/source.

**Permission denied** — The binary needs execute permission. Run `chmod +x node_modules/bc-cli/bin/bc`.

**Unsupported platform** — bc provides pre-built binaries for macOS (amd64/arm64) and Linux (amd64). For other platforms, build from source.

## License

MIT
