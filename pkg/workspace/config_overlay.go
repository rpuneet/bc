package workspace

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/rpuneet/mycel/pkg/log"
)

// ApplyOverlay applies raw JSON config data on top of c, section by
// section. Sections present in the overlay replace the corresponding
// section using the same semantics as the settings PATCH endpoint
// (decode into a copy of the current section); the gateways section is
// deep-merged per platform key so an overlay defining only "slack" does
// not wipe other configured gateways. Unknown top-level keys are
// ignored and "version" is never overridden. On error c is left with
// its original section values.
func (c *Config) ApplyOverlay(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse overlay: %w", err)
	}

	merged := *c
	for key, sec := range raw {
		var err error
		switch key {
		case "user":
			err = json.Unmarshal(sec, &merged.User)
		case "server":
			err = json.Unmarshal(sec, &merged.Server)
		case "runtime":
			err = json.Unmarshal(sec, &merged.Runtime)
		case "providers":
			err = json.Unmarshal(sec, &merged.Providers)
		case "gateways":
			err = MergeGatewaysPatch(&merged.Gateways, sec)
		case "cron":
			err = json.Unmarshal(sec, &merged.Cron)
		case "storage":
			err = json.Unmarshal(sec, &merged.Storage)
		case "logs":
			err = json.Unmarshal(sec, &merged.Logs)
		case "ui":
			err = json.Unmarshal(sec, &merged.UI)
		case "version":
			// The active config's schema version stands.
		default:
			// Ignore unknown sections — the overlay file may come from
			// an older or newer bc.
		}
		if err != nil {
			return fmt.Errorf("overlay section %q: %w", key, err)
		}
	}
	*c = merged
	return nil
}

// applyNewerSettingsOverlay overlays a legacy settings.json onto cfg
// when one exists that is strictly newer (mtime) than the active
// preferences file at prefsPath (#3239). Candidates are
// <stateDir>/settings.json and the project's <rootDir>/.bc/settings.json;
// when both qualify the newest wins. Equal mtimes mean preferences.json
// wins. Returns the path of the applied overlay, or "" when nothing was
// applied. A malformed or unreadable overlay is skipped with a warning
// so the active preferences survive.
func applyNewerSettingsOverlay(cfg *Config, prefsPath, stateDir, rootDir string) string {
	prefsInfo, err := os.Stat(prefsPath)
	if err != nil {
		return ""
	}

	candidates := []string{filepath.Join(stateDir, LegacySettingsFileName)}
	if projectPath := filepath.Join(rootDir, ".bc", LegacySettingsFileName); projectPath != candidates[0] {
		candidates = append(candidates, projectPath)
	}

	newest := ""
	var newestMod time.Time
	for _, p := range candidates {
		info, statErr := os.Stat(p)
		if statErr != nil || !info.ModTime().After(prefsInfo.ModTime()) {
			continue
		}
		if newest == "" || info.ModTime().After(newestMod) {
			newest = p
			newestMod = info.ModTime()
		}
	}
	if newest == "" {
		return ""
	}

	data, readErr := os.ReadFile(newest) //nolint:gosec // workspace-owned path
	if readErr != nil {
		log.Warn("config overlay unreadable; keeping preferences.json",
			"path", newest, "error", readErr)
		return ""
	}
	if applyErr := cfg.ApplyOverlay(data); applyErr != nil {
		log.Warn("config overlay skipped; keeping preferences.json",
			"path", newest, "error", applyErr)
		return ""
	}
	return newest
}

// ConfigDriftSections reports which top-level config sections would
// change if the file at overlayPath (a legacy settings.json) were
// applied on top of the active config at activePath. An empty result
// means the two files are effectively in sync. Used by `mycel doctor`
// to flag dead config edits (#3239).
func ConfigDriftSections(activePath, overlayPath string) ([]string, error) {
	active, err := LoadConfig(activePath)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", activePath, err)
	}
	overlayData, err := os.ReadFile(overlayPath) //nolint:gosec // caller-constructed path
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", overlayPath, err)
	}

	// Re-parse for an independent copy, then apply the overlay to it.
	merged, err := LoadConfig(activePath)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", activePath, err)
	}
	if applyErr := merged.ApplyOverlay(overlayData); applyErr != nil {
		return nil, applyErr
	}

	activeSections, err := configSections(active)
	if err != nil {
		return nil, err
	}
	mergedSections, err := configSections(merged)
	if err != nil {
		return nil, err
	}

	var drift []string
	for key, av := range activeSections {
		if !bytes.Equal(av, mergedSections[key]) {
			drift = append(drift, key)
		}
	}
	sort.Strings(drift)
	return drift, nil
}

// configSections marshals a Config and splits it into top-level raw
// JSON sections for byte-wise comparison. Struct marshaling is
// deterministic, so equal sections produce identical bytes.
func configSections(c *Config) (map[string]json.RawMessage, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}
