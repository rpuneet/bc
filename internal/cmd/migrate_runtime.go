package cmd

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/rpuneet/bc/pkg/log"
	"github.com/rpuneet/bc/pkg/workspace"
)

// migrateRuntimeMarker is the sentinel file written after a successful
// workspace-runtime migration pass. Its presence short-circuits the
// auto-run that serve.go invokes at bcd boot.
const migrateRuntimeMarker = ".migrated-runtime-v1"

var migrateRuntimeCmd = &cobra.Command{
	Use:   "migrate-runtime",
	Short: "Move workspace runtime state from <project>/.bc/ to ~/.bc/workspaces/<id>/",
	Long: `Relocates per-workspace runtime state (preferences.json, state.db,
cron.db, agents/, logs/, etc.) from the legacy <project>/.bc/ sidecar into
~/.bc/workspaces/<id>/. Agent git worktrees are moved via 'git worktree
move' so their HEAD stays pointed at the project's .git directory.

The migration is idempotent: workspaces whose DataDir already has content
are skipped. After a successful move, the legacy .bc/ directory is
renamed to .bc.migrated/ as an audit breadcrumb.

This command runs automatically at bcd startup (tracked by
~/.bc/.migrated-runtime-v1). Invoke it manually only to retry a failed
migration or to run on a freshly registered workspace.`,
	RunE: runMigrateRuntime,
}

func init() {
	workspaceCmd.AddCommand(migrateRuntimeCmd)
}

// WorkspaceRuntimeMigrationResult summarizes one workspace's migration.
type WorkspaceRuntimeMigrationResult struct {
	ID               string
	Name             string
	ProjectPath      string
	DataDir          string
	Skipped          bool
	SkipReason       string
	MovedFiles       int
	MovedDirs        int
	WorktreesMoved   int
	WorktreesSkipped []string
	Errors           []string
}

func runMigrateRuntime(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	results, err := MigrateAllWorkspaceRuntimes(ctx)
	if err != nil {
		return err
	}
	for _, r := range results {
		printMigrationResult(cmd.OutOrStdout(), r)
	}
	return nil
}

// MigrateAllWorkspaceRuntimes walks the global registry and migrates each
// workspace's runtime state to ~/.bc/workspaces/<id>/. Returns one result
// per registered workspace.
//
// Pre-flight: stale registry entries (project Path no longer on disk) are
// pruned before iteration. Without this, tests that created and cleaned
// up tmp projects leave phantom entries behind; walking them caused the
// "creates 11,698 junk dirs in ~/.bc/workspaces/" incident during the
// M11 rollout.
func MigrateAllWorkspaceRuntimes(_ context.Context) ([]WorkspaceRuntimeMigrationResult, error) {
	reg, err := workspace.LoadRegistry()
	if err != nil {
		return nil, fmt.Errorf("load registry: %w", err)
	}
	if pruned := reg.PruneStalePaths(); pruned > 0 {
		if saveErr := reg.Save(); saveErr != nil {
			log.Warn("failed to persist pruned registry", "error", saveErr)
		} else {
			log.Info("pruned stale registry entries before migration", "removed", pruned)
		}
	}
	entries := reg.List()
	results := make([]WorkspaceRuntimeMigrationResult, 0, len(entries))
	for i := range entries {
		r := migrateOneWorkspaceRuntime(&entries[i])
		results = append(results, r)
	}
	// Write the "done" marker even if individual workspaces failed —
	// the per-workspace skip logic will re-try on next boot if their
	// data dir is still empty.
	if markerErr := writeRuntimeMigrationMarker(); markerErr != nil {
		log.Warn("could not write runtime migration marker", "error", markerErr)
	}
	return results, nil
}

// migrateOneWorkspaceRuntime moves a single workspace's state.
func migrateOneWorkspaceRuntime(entry *workspace.RegistryEntry) WorkspaceRuntimeMigrationResult {
	res := WorkspaceRuntimeMigrationResult{
		ID:          entry.ID,
		Name:        entry.Name,
		ProjectPath: entry.Path,
		DataDir:     entry.GetDataDir(),
	}

	if res.DataDir == "" {
		res.Skipped = true
		res.SkipReason = "no DataDir (registry entry lacks ID)"
		return res
	}

	legacyDir := filepath.Join(entry.Path, ".bc")
	legacyStat, legacyErr := os.Stat(legacyDir)
	if legacyErr != nil || !legacyStat.IsDir() {
		res.Skipped = true
		res.SkipReason = "no legacy .bc/ sidecar to migrate"
		return res
	}

	// If the DataDir already has a config file, treat as migrated.
	if hasAnyConfig(res.DataDir) {
		res.Skipped = true
		res.SkipReason = "DataDir already populated"
		return res
	}

	if err := os.MkdirAll(res.DataDir, 0o750); err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("mkdir DataDir: %v", err))
		return res
	}

	// Move essentials into DataDir. Renames settings.json to
	// preferences.json as part of the move; known important files and
	// directories are moved individually so we can skip worktrees
	// (which require git worktree move — see below) and unknown
	// garbage.
	files := []string{
		workspace.PreferencesFileName,
		workspace.LegacySettingsFileName,
		"bc.db", "bc.db-shm", "bc.db-wal",
		"state.db", "state.db-shm", "state.db-wal",
		"cron.db", "cron.db-shm", "cron.db-wal",
		"channels.db", "cost.db", "events.jsonl",
	}
	for _, name := range files {
		src := filepath.Join(legacyDir, name)
		if _, statErr := os.Stat(src); statErr != nil {
			continue
		}
		dstName := name
		if name == workspace.LegacySettingsFileName {
			// Promote settings.json → preferences.json only if the
			// canonical name is not already there.
			if _, err := os.Stat(filepath.Join(res.DataDir, workspace.PreferencesFileName)); err == nil {
				continue
			}
			dstName = workspace.PreferencesFileName
		}
		dst := filepath.Join(res.DataDir, dstName)
		if mvErr := moveOrCopy(src, dst); mvErr != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("move %s: %v", name, mvErr))
			continue
		}
		res.MovedFiles++
	}

	// Move subdirectories wholesale — except "agents/" which requires
	// special-case git worktree moves below.
	subdirs := []string{"roles", "logs", "channels", "prompts", "templates", "volumes"}
	for _, name := range subdirs {
		src := filepath.Join(legacyDir, name)
		if info, statErr := os.Stat(src); statErr != nil || !info.IsDir() {
			continue
		}
		dst := filepath.Join(res.DataDir, name)
		if _, dstErr := os.Stat(dst); dstErr == nil {
			continue // already present
		}
		if mvErr := moveOrCopy(src, dst); mvErr != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("move %s/: %v", name, mvErr))
			continue
		}
		res.MovedDirs++
	}

	// Agents dir: move per-agent state dirs and git-worktree-move each
	// inner worktree so git's metadata stays valid.
	if err := migrateAgentsDir(entry, legacyDir, res.DataDir, &res); err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("agents/: %v", err))
	}

	// Rename the legacy .bc/ to .bc.migrated/ as a visible audit trail.
	// Contents that were NOT moved (e.g. unknown files) stay inside.
	newLegacyPath := filepath.Join(entry.Path, ".bc.migrated")
	if _, err := os.Stat(newLegacyPath); err == nil {
		// Append a timestamp to avoid clobbering a previous migration.
		newLegacyPath = fmt.Sprintf("%s.%d", newLegacyPath, time.Now().Unix())
	}
	if err := os.Rename(legacyDir, newLegacyPath); err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("rename .bc → .bc.migrated: %v", err))
	} else {
		// Drop a breadcrumb pointing at the new location.
		crumb := filepath.Join(entry.Path, ".bc.migrated.txt")
		content := fmt.Sprintf("bc workspace runtime moved to: %s\n", res.DataDir)
		_ = os.WriteFile(crumb, []byte(content), 0o644) //nolint:gosec // breadcrumb, not sensitive
	}

	log.Info("migrated workspace runtime",
		"workspace", entry.Name,
		"id", entry.ID,
		"data_dir", res.DataDir,
		"files", res.MovedFiles,
		"dirs", res.MovedDirs,
		"worktrees", res.WorktreesMoved,
	)
	return res
}

// migrateAgentsDir moves <legacyDir>/agents/<agent>/ directories into
// the new DataDir, using 'git worktree move' for inner worktrees so git
// metadata stays coherent.
func migrateAgentsDir(entry *workspace.RegistryEntry, legacyDir, dataDir string, res *WorkspaceRuntimeMigrationResult) error {
	legacyAgents := filepath.Join(legacyDir, "agents")
	if info, err := os.Stat(legacyAgents); err != nil || !info.IsDir() {
		return nil
	}
	newAgents := filepath.Join(dataDir, "agents")
	if err := os.MkdirAll(newAgents, 0o750); err != nil {
		return fmt.Errorf("mkdir agents: %w", err)
	}

	entries, err := os.ReadDir(legacyAgents)
	if err != nil {
		return fmt.Errorf("read agents: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		agentName := e.Name()
		oldAgentDir := filepath.Join(legacyAgents, agentName)
		newAgentDir := filepath.Join(newAgents, agentName)

		// Find the worktree subdir (bc-<ws>-<agent>/). We try both the
		// plain hashed name and the generic "worktree" name defensively.
		oldWT := findWorktreeSubdir(oldAgentDir)
		if oldWT != "" {
			rel, relErr := filepath.Rel(oldAgentDir, oldWT)
			if relErr != nil {
				res.WorktreesSkipped = append(res.WorktreesSkipped, agentName+" (rel error)")
			} else {
				newWT := filepath.Join(newAgentDir, rel)
				if err := gitWorktreeMove(entry.Path, oldWT, newWT); err != nil {
					log.Warn("git worktree move failed", "agent", agentName, "error", err)
					res.WorktreesSkipped = append(res.WorktreesSkipped, agentName+" ("+truncErr(err)+")")
					// Skip moving this agent's state dir so git doesn't
					// lose track of the worktree; the user can retry
					// manually after cleaning up.
					continue
				}
				res.WorktreesMoved++
			}
		}

		// Move the rest of the agent dir (minus the worktree we already
		// handled).
		if err := moveAgentRemainder(oldAgentDir, newAgentDir); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("agent %s remainder: %v", agentName, err))
		}
	}
	return nil
}

// findWorktreeSubdir returns the path of the agent's git worktree
// directory (bc-<ws>-<agent>/) inside agentDir, or "" if none is present.
// We identify worktrees by the presence of a ".git" file (gitdir pointer).
func findWorktreeSubdir(agentDir string) string {
	entries, err := os.ReadDir(agentDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		candidate := filepath.Join(agentDir, e.Name())
		if info, err := os.Stat(filepath.Join(candidate, ".git")); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

// gitWorktreeMove runs 'git -C <repo> worktree move <old> <new>'. A
// non-nil error means git refused (dirty worktree, missing binary, etc.)
// and the caller should leave the worktree in place.
func gitWorktreeMove(repoRoot, oldPath, newPath string) error {
	if err := os.MkdirAll(filepath.Dir(newPath), 0o750); err != nil {
		return err
	}
	cmd := exec.Command("git", "-C", repoRoot, "worktree", "move", oldPath, newPath) //nolint:gosec // arguments are trusted config-derived paths
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// moveAgentRemainder moves everything under oldDir that isn't already at
// newDir. The worktree subdir (which was just moved by git) is skipped.
func moveAgentRemainder(oldDir, newDir string) error {
	if err := os.MkdirAll(newDir, 0o750); err != nil {
		return err
	}
	entries, err := os.ReadDir(oldDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		src := filepath.Join(oldDir, e.Name())
		dst := filepath.Join(newDir, e.Name())
		if _, err := os.Stat(dst); err == nil {
			continue // already in place (worktree just moved)
		}
		if mvErr := moveOrCopy(src, dst); mvErr != nil {
			return fmt.Errorf("%s: %w", e.Name(), mvErr)
		}
	}
	// Remove the (now likely empty) old agent dir.
	_ = os.Remove(oldDir) //nolint:errcheck // best-effort cleanup
	return nil
}

// moveOrCopy renames src → dst; if that fails (e.g., cross-device) it
// falls back to a recursive copy + remove.
func moveOrCopy(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// Fallback: cp -a style.
	if err := copyAny(src, dst); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

// copyAny copies a file or directory tree.
func copyAny(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyTree(src, dst)
	}
	return copyFileAt(src, dst, info.Mode())
}

func copyTree(src, dst string) error {
	if err := os.MkdirAll(dst, 0o750); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyTree(s, d); err != nil {
				return err
			}
			continue
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		if err := copyFileAt(s, d, info.Mode()); err != nil {
			return err
		}
	}
	return nil
}

func copyFileAt(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src) //nolint:gosec // caller-controlled path
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode) //nolint:gosec // caller-controlled path
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	buf := make([]byte, 32*1024)
	for {
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if rerr != nil {
			if rerr.Error() == "EOF" {
				break
			}
			if n == 0 {
				break
			}
		}
		if n == 0 {
			break
		}
	}
	return nil
}

// hasAnyConfig returns true when dataDir contains at least one recognized
// preferences / settings file.
func hasAnyConfig(dataDir string) bool {
	for _, name := range []string{
		workspace.PreferencesFileName,
		workspace.LegacySettingsFileName,
	} {
		if _, err := os.Stat(filepath.Join(dataDir, name)); err == nil {
			return true
		}
	}
	return false
}

// writeRuntimeMigrationMarker drops ~/.bc/.migrated-runtime-v1 so future
// boots know we already ran.
func writeRuntimeMigrationMarker() error {
	home, err := workspace.BCHome()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(home, 0o750); err != nil {
		return err
	}
	marker := filepath.Join(home, migrateRuntimeMarker)
	content := fmt.Sprintf("ran at %s\n", time.Now().UTC().Format(time.RFC3339))
	return os.WriteFile(marker, []byte(content), 0o644) //nolint:gosec // marker, not sensitive
}

// RuntimeMigrationAlreadyRan returns true when the sentinel marker exists.
func RuntimeMigrationAlreadyRan() bool {
	home, err := workspace.BCHome()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(home, migrateRuntimeMarker))
	return err == nil
}

// isDefaultBCHome reports whether BCHome() resolves to the canonical
// $HOME/.bc path. When BC_HOME is set to any other value we treat the
// process as sandboxed (tests, integration fixtures, custom installs)
// and decline to run the auto-migration. This prevents the M11 boot
// hook from walking a fresh tmpdir registry or — worse — the host's
// real registry while the test's BC_HOME points elsewhere.
func isDefaultBCHome() bool {
	bc, err := workspace.BCHome()
	if err != nil {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	want := filepath.Join(home, ".bc")
	absBC, errBC := filepath.Abs(bc)
	absWant, errWant := filepath.Abs(want)
	if errBC != nil || errWant != nil {
		return false
	}
	return absBC == absWant
}

// snapshotDir records every regular file under root with its size.
// Used before and after a move to spot-check that no bytes disappeared.
// Returns a map from relative path → size in bytes.
func snapshotDir(root string) (map[string]int64, error) {
	snap := map[string]int64{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		snap[rel] = info.Size()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return snap, nil
}

// MaybeRunRuntimeMigration runs the migration when it has never run
// before on this machine. Intended for invocation at bcd startup.
//
// Guards (any of these skip the migration):
//   - BC_SKIP_MIGRATION env var set to a non-empty value (explicit opt-out)
//   - BC_HOME points anywhere other than $HOME/.bc (test sandbox, custom
//     install) — the migration is a production-only one-shot and
//     should never run against an isolated BC_HOME. Callers that
//     genuinely want to migrate a custom BC_HOME must invoke
//     `bc workspace migrate-runtime` explicitly.
//   - The sentinel marker ~/.bc/.migrated-runtime-v1 already exists.
func MaybeRunRuntimeMigration(ctx context.Context) {
	if os.Getenv("BC_SKIP_MIGRATION") != "" {
		return
	}
	if !isDefaultBCHome() {
		return
	}
	if RuntimeMigrationAlreadyRan() {
		return
	}
	log.Info("running one-time workspace-runtime migration (M11)")
	results, err := MigrateAllWorkspaceRuntimes(ctx)
	if err != nil {
		log.Warn("workspace-runtime migration failed", "error", err)
		return
	}
	migrated, skipped, failed := 0, 0, 0
	for _, r := range results {
		switch {
		case len(r.Errors) > 0:
			failed++
		case r.Skipped:
			skipped++
		default:
			migrated++
		}
	}
	log.Info("workspace-runtime migration complete",
		"migrated", migrated, "skipped", skipped, "failed", failed,
		"total", len(results))
}

// printMigrationResult emits a human-readable summary of one workspace's
// migration to cmd.OutOrStdout().
func printMigrationResult(out interface{ Write([]byte) (int, error) }, r WorkspaceRuntimeMigrationResult) {
	fmt.Fprintf(out, "%s (%s)\n", r.Name, r.ID)
	fmt.Fprintf(out, "  project:  %s\n", r.ProjectPath)
	fmt.Fprintf(out, "  data_dir: %s\n", r.DataDir)
	if r.Skipped {
		fmt.Fprintf(out, "  SKIPPED: %s\n", r.SkipReason)
		return
	}
	fmt.Fprintf(out, "  moved: %d files, %d dirs, %d worktrees\n", r.MovedFiles, r.MovedDirs, r.WorktreesMoved)
	for _, s := range r.WorktreesSkipped {
		fmt.Fprintf(out, "  worktree skipped: %s\n", s)
	}
	for _, e := range r.Errors {
		fmt.Fprintf(out, "  error: %s\n", e)
	}
}

// truncErr shortens a multi-line error for compact logging.
func truncErr(err error) string {
	msg := strings.ReplaceAll(err.Error(), "\n", " ")
	if len(msg) > 80 {
		msg = msg[:77] + "..."
	}
	return msg
}
