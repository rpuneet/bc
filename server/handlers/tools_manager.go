package handlers

import (
	"path/filepath"
	"strings"
)

// Where a CLI tool came from is inferable, so the UI should not ask for it.
// Two independent kinds of evidence are available, and they answer slightly
// different questions:
//
//   - The resolved path says what actually installed the binary that is on
//     PATH right now. Strongest evidence, and the only one that exists for a
//     tool nobody configured an install command for (git, go, jq…).
//   - The configured install command says how mycel would install or update
//     it. Used as a fallback, since a tool that is not installed yet has no
//     path to inspect.
//
// Path wins when both are present: config can be stale or aspirational, while
// a binary at /opt/homebrew/bin/rg is a fact.

// managerCommands maps the binary that leads an install command to the manager
// id the rest of the API uses (see searchSpecs / installSpecs). Only managers
// whose name is unambiguous as a leading token are listed — `go install` is
// deliberately absent, since "go" is far more often the tool than the manager.
var managerCommands = map[string]string{
	"brew":    "brew",
	"npm":     "npm",
	"pnpm":    "pnpm",
	"yarn":    "yarn",
	"pipx":    "pipx",
	"pip":     "pip",
	"pip3":    "pip",
	"uv":      "uv",
	"cargo":   "cargo",
	"gem":     "gem",
	"apt":     "apt",
	"apt-get": "apt",
	"dnf":     "dnf",
	"yum":     "yum",
	"pacman":  "pacman",
	"zypper":  "zypper",
	"winget":  "winget",
	"scoop":   "scoop",
	"choco":   "choco",
}

// pathMarkers maps a substring of an installed binary's real path to the
// manager that puts binaries there. Ordered checks live in managerFromPath;
// this table only covers markers specific enough to be conclusive anywhere in
// the path.
var pathMarkers = []struct {
	marker  string
	manager string
}{
	{"/cellar/", "brew"},       // macOS Homebrew, incl. the symlink target
	{"/opt/homebrew/", "brew"}, // Apple Silicon Homebrew prefix
	{"/linuxbrew/", "brew"},    // Homebrew on Linux
	{"/node_modules/", "npm"},  // a global npm install
	{"/.cargo/", "cargo"},      // cargo install
	{"/pipx/", "pipx"},         // pipx venvs
	{"/.pyenv/", "pyenv"},      // pyenv shims
	{"/.rbenv/", "rbenv"},      //
	{"/.nvm/", "nvm"},          // node from nvm — npm-installed globals live under it
	{"/.volta/", "volta"},      //
	{"/homebrew/", "brew"},     // non-standard brew prefix
	{"/scoop/", "scoop"},       //
	{"/chocolatey/", "choco"},  //
}

// systemPrefixes are the directories only the OS itself owns. Deliberately
// excludes /usr/local/bin: Homebrew on Intel macOS installs there, and so do
// hand-built binaries, so that path proves nothing on its own.
var systemPrefixes = []string{"/usr/bin/", "/bin/", "/usr/sbin/", "/sbin/"}

// InferToolManager reports which package manager owns a CLI tool, as a manager
// id ("brew", "npm", …), "system" for an OS-provided binary, or "" when there
// is no evidence either way. realPath should be the symlink-resolved path (a
// brew binary on PATH is usually a symlink into the Cellar, and only the target
// names the manager).
func InferToolManager(realPath, installCmd string) string {
	if m := managerFromPath(realPath); m != "" {
		return m
	}
	return managerFromInstallCmd(installCmd)
}

// managerFromPath infers the manager from where the binary actually lives.
func managerFromPath(realPath string) string {
	if realPath == "" {
		return ""
	}
	// Compare in lower case with forward slashes so the markers match on
	// Windows and on case-insensitive macOS volumes alike.
	p := strings.ToLower(filepath.ToSlash(realPath))
	for _, pm := range pathMarkers {
		if strings.Contains(p, pm.marker) {
			return pm.manager
		}
	}
	for _, prefix := range systemPrefixes {
		if strings.HasPrefix(p, prefix) {
			return "system"
		}
	}
	return ""
}

// managerFromInstallCmd infers the manager from a configured install command,
// e.g. "brew install ripgrep" or "sudo apt-get install -y jq". Returns "" when
// the leading token is not a manager this code recognizes, so a bespoke
// installer (a curl|sh line, a vendored script) is reported as unknown rather
// than guessed at.
func managerFromInstallCmd(installCmd string) string {
	for _, field := range strings.Fields(installCmd) {
		// Skip the wrappers and VAR=value prefixes that precede the real
		// command, so "sudo apt-get install" and "DEBIAN_FRONTEND=noninteractive
		// apt-get install" both resolve to apt.
		if field == "sudo" || field == "env" || strings.Contains(field, "=") {
			continue
		}
		// A manager may be invoked by absolute path.
		if m, ok := managerCommands[strings.ToLower(filepath.Base(filepath.ToSlash(field)))]; ok {
			return m
		}
		// The first token that is not a wrapper decides: anything past it is
		// a subcommand or a package name, and a package named "npm" must not
		// make a curl-based installer look like an npm install.
		return ""
	}
	return ""
}
