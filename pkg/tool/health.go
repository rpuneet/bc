package tool

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// HealthResult is the outcome of a single tool's health check.
type HealthResult struct { //nolint:govet // field order matches JSON/API contract
	Name   string `json:"name"`
	Type   string `json:"type"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// checkOne inspects t's install status via exec.LookPath. It never shells
// out beyond a PATH lookup, so it is safe to run unattended and frequently.
func checkOne(t *Tool) HealthResult {
	r := HealthResult{Name: t.Name, Type: t.Type, Status: "ok"}
	switch t.Type {
	case ToolTypeMCP:
		// Only a stdio server runs a local command. An SSE server's endpoint
		// is not probed here, so it keeps the default "ok".
		if t.Transport != "stdio" {
			break
		}
		// Length check rather than t.Command != "": a command of only
		// whitespace is non-empty yet yields no fields to look up.
		fields := strings.Fields(t.Command)
		if len(fields) == 0 {
			r.Status = "error"
			r.Error = "no command configured"
			break
		}
		if _, err := exec.LookPath(fields[0]); err != nil {
			r.Status = "error"
			r.Error = "command not found: " + fields[0]
		}
	case ToolTypeCLI, ToolTypeProvider:
		fields := strings.Fields(t.Command)
		if len(fields) == 0 {
			r.Status = "not_installed"
			r.Error = "no command configured"
			break
		}
		if _, err := exec.LookPath(fields[0]); err != nil {
			r.Status = "not_installed"
			r.Error = "not found in PATH"
		} else {
			r.Status = "installed"
		}
	}
	return r
}

// CheckAll runs a health check on every stored tool and persists fresh
// health_status + last_checked back via UpdateHealth, so subsequent
// List/Get calls serve recently-verified status rather than the seed-time
// default. Used by both the manual force-refresh endpoint
// (POST /api/tools/unified/check) and the background auto-check loop started at
// daemon boot. A persistence failure for one tool does not abort the batch.
func (s *Store) CheckAll(ctx context.Context) ([]HealthResult, error) {
	tools, err := s.List(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	results := make([]HealthResult, 0, len(tools))
	for _, t := range tools {
		r := checkOne(t)
		results = append(results, r)
		_ = s.UpdateHealth(ctx, t.Name, r.Status, now) //nolint:errcheck // best-effort per-tool; batch continues regardless
	}
	return results, nil
}
