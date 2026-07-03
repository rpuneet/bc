// Package workspace provides workspace discovery functionality.
package workspace

import (
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// DiscoveredWorkspace represents a workspace found during discovery.
type DiscoveredWorkspace struct {
	Path      string // Absolute path to workspace root
	Name      string // Workspace name (from config or directory name)
	IsV2      bool   // True if v2 (TOML) workspace
	FromCache bool   // True if found in registry (not disk scan)
}

// DiscoverOptions configures workspace discovery.
type DiscoverOptions struct {
	// ScanPaths additional paths to scan for workspaces
	ScanPaths []string
	// MaxDepth maximum directory depth to scan (default 3)
	MaxDepth int
	// IncludeCached includes workspaces from the global registry (~/.mycel/workspaces.json)
	IncludeCached bool
	// ScanHome scans ~/Projects and ~/Developer directories
	ScanHome bool
}

// DefaultDiscoverOptions returns sensible defaults for discovery.
func DefaultDiscoverOptions() DiscoverOptions {
	return DiscoverOptions{
		IncludeCached: true,
		ScanHome:      true,
		MaxDepth:      3,
	}
}

// Discover finds all mycel workspaces on the machine.
// It checks the global registry and optionally scans common directories.
func Discover(opts DiscoverOptions) ([]DiscoveredWorkspace, error) {
	seen := make(map[string]bool)
	var workspaces []DiscoveredWorkspace
	var mu sync.Mutex

	// 1. Load from registry first (these are known workspaces)
	if opts.IncludeCached {
		registry, err := LoadRegistry()
		if err == nil {
			for _, entry := range registry.Workspaces {
				// Verify workspace still exists
				if IsWorkspace(entry.Path) {
					seen[entry.Path] = true
					workspaces = append(workspaces, DiscoveredWorkspace{
						Path:      entry.Path,
						Name:      entry.Name,
						FromCache: true,
						IsV2:      isV2Workspace(entry.Path),
					})
				}
			}
		}
	}

	// 2. Scan home directories
	if opts.ScanHome {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			commonDirs := []string{
				filepath.Join(homeDir, "Projects"),
				filepath.Join(homeDir, "Developer"),
				filepath.Join(homeDir, "dev"),
				filepath.Join(homeDir, "code"),
				filepath.Join(homeDir, "repos"),
				filepath.Join(homeDir, "src"),
			}
			for _, dir := range commonDirs {
				if _, err := os.Stat(dir); err == nil {
					scanDir(dir, opts.MaxDepth, seen, &workspaces, &mu)
				}
			}
		}
	}

	// 3. Scan additional paths
	for _, dir := range opts.ScanPaths {
		if _, err := os.Stat(dir); err == nil {
			scanDir(dir, opts.MaxDepth, seen, &workspaces, &mu)
		}
	}

	// Sort by name for consistent output
	sort.Slice(workspaces, func(i, j int) bool {
		return workspaces[i].Name < workspaces[j].Name
	})

	return workspaces, nil
}

// scanDir recursively scans a directory for workspaces up to maxDepth.
func scanDir(dir string, maxDepth int, seen map[string]bool, workspaces *[]DiscoveredWorkspace, mu *sync.Mutex) {
	if maxDepth < 0 {
		return
	}

	// Check if this directory is a workspace
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return
	}

	mu.Lock()
	alreadySeen := seen[absDir]
	if !alreadySeen {
		seen[absDir] = true
	}
	mu.Unlock()

	if alreadySeen {
		return
	}

	if IsWorkspace(absDir) {
		ws := DiscoveredWorkspace{
			Path:      absDir,
			Name:      filepath.Base(absDir),
			FromCache: false,
			IsV2:      isV2Workspace(absDir),
		}

		// Try to get name from config
		if w, loadErr := Load(absDir); loadErr == nil {
			ws.Name = w.Name()
		}

		mu.Lock()
		*workspaces = append(*workspaces, ws)
		mu.Unlock()

		// Don't recurse into workspace directories
		return
	}

	// Recurse into subdirectories
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Skip hidden directories and common non-project dirs
		if name[0] == '.' || name == "node_modules" || name == "vendor" || name == "__pycache__" {
			continue
		}

		subdir := filepath.Join(dir, name)
		scanDir(subdir, maxDepth-1, seen, workspaces, mu)
	}
}

// isV2Workspace checks if a workspace uses v2 (JSON) config: a
// preferences.json in the global ~/.mycel/workspaces/<id>/ dir.
func isV2Workspace(dir string) bool {
	globalDir, err := GlobalStateDir(dir)
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(globalDir, PreferencesFileName))
	return err == nil
}

// DiscoverAndRegister discovers workspaces and adds new ones to the registry.
// Returns the number of newly registered workspaces.
func DiscoverAndRegister(opts DiscoverOptions) (int, error) {
	workspaces, err := Discover(opts)
	if err != nil {
		return 0, err
	}

	registry, err := LoadRegistry()
	if err != nil {
		return 0, err
	}

	newCount := 0
	for _, ws := range workspaces {
		if !ws.FromCache {
			// This is a newly discovered workspace
			registry.Register(ws.Path, ws.Name)
			newCount++
		}
	}

	if newCount > 0 {
		if err := registry.Save(); err != nil {
			return newCount, err
		}
	}

	return newCount, nil
}
