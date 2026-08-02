// mcp_health.go — health checks for registered MCP servers.
//
// A stale or misconfigured MCP server (a stdio command that no longer
// exists on PATH, an SSE URL that stopped responding) is otherwise
// silently skipped at agent spawn time with only a debug log. This
// category surfaces that state so `mycel doctor` catches it before an
// agent does.
package doctor

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/mcp"
)

const (
	// mcpProbeTimeout bounds a single SSE/URL reachability probe.
	mcpProbeTimeout = 3 * time.Second
	// mcpTotalTimeout bounds the whole MCP category so doctor stays fast
	// even with a large registry or several unreachable servers — probes
	// run in parallel, not serially, so this is not N * mcpProbeTimeout.
	mcpTotalTimeout = 8 * time.Second
)

// CheckMCP checks every MCP server registered in the user-global
// registry (~/.mycel/mcps.json): stdio servers must have a command that
// resolves on PATH, SSE/url servers must be reachable within a short,
// bounded timeout. Checks run in parallel. An empty registry reports ok
// ("no MCP servers configured"), not an error.
func CheckMCP(ctx context.Context, h *home.Home) CategoryReport {
	cat := CategoryReport{Name: "MCP"}

	path := mcpConfigPath(h)
	store := mcp.NewGlobalStore(path)
	servers, err := store.List()
	if err != nil {
		cat.Items = append(cat.Items, Item{
			Name:     "mcp servers",
			Message:  fmt.Sprintf("cannot read %s: %v", path, err),
			Severity: SeverityFail,
			Fix:      fmt.Sprintf("check %s for corruption, or remove it to reset the registry", path),
		})
		return cat
	}

	if len(servers) == 0 {
		cat.Items = append(cat.Items, Item{
			Name:     "mcp servers",
			Message:  "no MCP servers configured",
			Severity: SeverityOK,
		})
		return cat
	}

	// Stable, deterministic ordering for output and tests.
	sort.Slice(servers, func(i, j int) bool { return servers[i].Name < servers[j].Name })

	probeCtx, cancel := context.WithTimeout(ctx, mcpTotalTimeout)
	defer cancel()

	problems := make([]*Item, len(servers))
	var wg sync.WaitGroup
	for i, s := range servers {
		wg.Add(1)
		go func(i int, s *mcp.ServerConfig) {
			defer wg.Done()
			problems[i] = checkMCPServer(probeCtx, s)
		}(i, s)
	}
	wg.Wait()

	okCount := 0
	var problemMsgs []string
	for i, item := range problems {
		if item == nil {
			okCount++
			continue
		}
		problemMsgs = append(problemMsgs, fmt.Sprintf("%s: %s", servers[i].Name, item.Message))
		cat.Items = append(cat.Items, *item)
	}

	summary := fmt.Sprintf("%d of %d MCP servers OK", okCount, len(servers))
	if len(problemMsgs) > 0 {
		summary += "; " + strings.Join(problemMsgs, "; ")
	}
	cat.Items = append([]Item{{
		Name:     "mcp servers",
		Message:  summary,
		Severity: SeverityOK,
	}}, cat.Items...)

	return cat
}

// mcpConfigPath returns the path to the user-global MCP registry. Falls
// back to home.GlobalMCPConfig() (MYCEL_HOME-aware) when h is nil or its
// state dir can't be resolved, so the check still degrades gracefully
// outside a repo.
func mcpConfigPath(h *home.Home) string {
	if h != nil {
		if stateDir := h.StateDir(); stateDir != "" {
			return filepath.Join(stateDir, "mcps.json")
		}
	}
	if p, err := home.GlobalMCPConfig(); err == nil {
		return p
	}
	return "mcps.json"
}

// checkMCPServer checks one MCP server's health. Returns nil when
// healthy, or an Item describing the problem otherwise.
func checkMCPServer(ctx context.Context, s *mcp.ServerConfig) *Item {
	// Disabled servers are never spawned, so their command/URL health is
	// irrelevant — don't flag a stale disabled entry as a problem.
	if !s.Enabled {
		return nil
	}

	name := "mcp:" + s.Name

	switch s.Transport {
	case mcp.TransportStdio:
		if s.Command == "" {
			return &Item{
				Name:     name,
				Message:  "stdio server has no command configured",
				Severity: SeverityFail,
			}
		}
		if _, err := exec.LookPath(s.Command); err != nil {
			return &Item{
				Name:     name,
				Message:  fmt.Sprintf("command %q not found on PATH", s.Command),
				Severity: SeverityFail,
				Fix:      fmt.Sprintf("install %q, or run 'mycel mcp remove %s' if it's no longer needed", s.Command, s.Name),
			}
		}
		return nil

	case mcp.TransportSSE:
		if s.URL == "" {
			return &Item{
				Name:     name,
				Message:  "sse server has no url configured",
				Severity: SeverityFail,
			}
		}
		if err := probeMCPURL(ctx, s.URL); err != nil {
			return &Item{
				Name:     name,
				Message:  fmt.Sprintf("url %s unreachable: %v", s.URL, err),
				Severity: SeverityWarn,
				Fix:      fmt.Sprintf("verify %s is running and reachable, or run 'mycel mcp remove %s' if it's stale", s.URL, s.Name),
			}
		}
		return nil

	default:
		return &Item{
			Name:     name,
			Message:  fmt.Sprintf("unknown transport %q", s.Transport),
			Severity: SeverityWarn,
		}
	}
}

// probeMCPURL performs a lightweight reachability probe against an SSE
// server URL: HEAD first, falling back to GET for servers that reject
// HEAD. Any response (including non-2xx) counts as reachable — this
// checks network reachability, not protocol correctness.
func probeMCPURL(ctx context.Context, rawURL string) error {
	ctx, cancel := context.WithTimeout(ctx, mcpProbeTimeout)
	defer cancel()

	if err := doProbeRequest(ctx, http.MethodHead, rawURL); err == nil {
		return nil
	}
	return doProbeRequest(ctx, http.MethodGet, rawURL)
}

func doProbeRequest(ctx context.Context, method, rawURL string) error {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return nil
}
