package home

// global.go — path helpers for the entity-scoped ~/.mycel tree.
//
// The mycel home is flat and entity-scoped: deleting an entity's
// directory (plus its db rows and vault keys) removes it completely.
//
//	~/.mycel/
//	  prefs.json           # THE global config
//	  mycel.db             # THE database
//	  secrets.vault
//	  agents/<name>/       # worktree/ session/ logs/ tmp/
//	  apps/<name>/         # only stateful apps create one
//	  templates/
//	  logs/                # daemon + process logs
//	  run/                 # daemon.pid, daemon.addr
//
// These helpers centralize path resolution so tests can swap MYCEL_HOME
// and production code stays consistent.

import (
	"fmt"
	"os"
	"path/filepath"
)

// Subdirectories and files relative to MycelHome().
const (
	globalTemplatesDirName = "templates"
	globalSecretsFileName  = "secrets.vault"
	globalMCPFileName      = "mcps.json"
	globalToolsFileName    = "tools.json"
	globalAgentsDirName    = "agents"
	globalAppsDirName      = "apps"
	globalLogsDirName      = "logs"
	globalRunDirName       = "run"
	globalDaemonPidName    = "daemon.pid"
	globalDaemonLogName    = "daemon.log"
	globalDaemonAddrName   = "daemon.addr"
)

// Agent entity subdirectories under agents/<name>/.
const (
	agentWorktreeDirName = "worktree"
	agentSessionDirName  = "session"
	agentLogsDirName     = "logs"
	agentTmpDirName      = "tmp"
)

// AgentsDir returns the root of all agent entity directories
// (~/.mycel/agents/).
func AgentsDir() (string, error) {
	return globalPath(globalAgentsDirName)
}

// AgentDir returns the entity directory for one agent
// (~/.mycel/agents/<name>/). Deleting this directory removes every
// piece of filesystem state the agent owns.
func AgentDir(name string) (string, error) {
	if name == "" || !filepath.IsLocal(name) {
		return "", fmt.Errorf("invalid agent name %q", name)
	}
	agents, err := AgentsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(agents, name), nil
}

// AgentWorktreeDir returns the agent's git worktree directory
// (~/.mycel/agents/<name>/worktree/).
func AgentWorktreeDir(name string) (string, error) {
	return agentSubdir(name, agentWorktreeDirName)
}

// AgentSessionDir returns the agent's provider-state directory
// (~/.mycel/agents/<name>/session/). Provider config and transcripts
// (e.g. the Claude home dir for docker agents) live here so they
// persist on the host.
func AgentSessionDir(name string) (string, error) {
	return agentSubdir(name, agentSessionDirName)
}

// AgentLogsDir returns the agent's log directory
// (~/.mycel/agents/<name>/logs/).
func AgentLogsDir(name string) (string, error) {
	return agentSubdir(name, agentLogsDirName)
}

// AgentTmpDir returns the agent's scratch directory
// (~/.mycel/agents/<name>/tmp/).
func AgentTmpDir(name string) (string, error) {
	return agentSubdir(name, agentTmpDirName)
}

func agentSubdir(name, sub string) (string, error) {
	dir, err := AgentDir(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, sub), nil
}

// AppsDir returns the root of app instance state directories
// (~/.mycel/apps/). Only stateful apps create a subdirectory.
func AppsDir() (string, error) {
	return globalPath(globalAppsDirName)
}

// GlobalTemplatesDir returns the user-global templates directory
// (~/.mycel/templates/) — the single template store.
func GlobalTemplatesDir() (string, error) {
	return globalPath(globalTemplatesDirName)
}

// GlobalSecretsVault returns the path to the user-global secrets vault
// (~/.mycel/secrets.vault). This is a SQLite database holding the user's
// encrypted key/value secrets.
func GlobalSecretsVault() (string, error) {
	return globalPath(globalSecretsFileName)
}

// GlobalMCPConfig returns the path to the user-global MCP trust config
// (~/.mycel/mcps.json).
func GlobalMCPConfig() (string, error) {
	return globalPath(globalMCPFileName)
}

// GlobalToolsConfig returns the path to the user-global CLI tools
// registry (~/.mycel/tools.json). Tools here describe machine-level
// dependencies (claude, bun, docker helpers, etc.).
func GlobalToolsConfig() (string, error) {
	return globalPath(globalToolsFileName)
}

// GlobalLogsDir returns the daemon/process log directory
// (~/.mycel/logs/).
func GlobalLogsDir() (string, error) {
	return globalPath(globalLogsDirName)
}

// RunDir returns the runtime-files directory (~/.mycel/run/) holding
// daemon.pid and daemon.addr.
func RunDir() (string, error) {
	return globalPath(globalRunDirName)
}

// EnsureRunDir creates ~/.mycel/run/ (and its parents) if missing and
// returns its path.
func EnsureRunDir() (string, error) {
	dir, err := RunDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0750); err != nil {
		return "", fmt.Errorf("create run dir %s: %w", dir, err)
	}
	return dir, nil
}

// EnsureGlobalLogsDir creates ~/.mycel/logs/ if missing and returns its
// path.
func EnsureGlobalLogsDir() (string, error) {
	dir, err := GlobalLogsDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0750); err != nil {
		return "", fmt.Errorf("create logs dir %s: %w", dir, err)
	}
	return dir, nil
}

// DaemonPidPath returns the path to the daemon pid file
// (~/.mycel/run/daemon.pid). The daemon is user-scoped — a single
// process serves everything.
func DaemonPidPath() (string, error) {
	dir, err := RunDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, globalDaemonPidName), nil
}

// DaemonLogPath returns the path to the daemon log file
// (~/.mycel/logs/daemon.log).
func DaemonLogPath() (string, error) {
	dir, err := GlobalLogsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, globalDaemonLogName), nil
}

// DaemonAddrPath returns the path to the daemon address file
// (~/.mycel/run/daemon.addr). `mycel up` writes the currently-listening
// address (scheme + host:port, e.g. "http://127.0.0.1:8080") so the CLI
// and agents can locate the daemon without requiring MYCEL_DAEMON_ADDR
// when the daemon runs on a non-default port.
func DaemonAddrPath() (string, error) {
	dir, err := RunDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, globalDaemonAddrName), nil
}

// EnsureGlobalDir makes sure ~/.mycel/ exists with 0750 permissions. It is
// idempotent and safe to call from any process path that needs to write
// a global asset. Returns the resolved MycelHome path for convenience.
func EnsureGlobalDir() (string, error) {
	home, err := MycelHome()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(home, 0750); err != nil {
		return "", fmt.Errorf("create mycel home %s: %w", home, err)
	}
	return home, nil
}

// globalPath joins MycelHome() with name. It does NOT create the parent; use
// EnsureGlobalDir when writing. Returns an error only if MycelHome cannot
// be resolved (HOME unset and MYCEL_HOME unset).
func globalPath(name string) (string, error) {
	home, err := MycelHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, name), nil
}
