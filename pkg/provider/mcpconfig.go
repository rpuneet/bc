package provider

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
)

// mcpJSONEntry represents one server entry in an mcp.json config file.
type mcpJSONEntry struct {
	Command string   `json:"command,omitempty"`
	URL     string   `json:"url,omitempty"`
	Type    string   `json:"type,omitempty"`
	Args    []string `json:"args,omitempty"`
}

// readMCPJSONFile reads an mcp.json-format file ({"mcpServers": {...}})
// and returns its servers sorted by name. Missing or malformed files
// yield an empty, non-nil slice — MCP listing is best-effort and must
// never fail a caller.
func readMCPJSONFile(path string) []MCPServerInfo {
	data, err := os.ReadFile(path) //nolint:gosec // path is derived from the workspace root, not user input
	if err != nil {
		return []MCPServerInfo{}
	}

	var cfg struct {
		MCPServers map[string]mcpJSONEntry `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return []MCPServerInfo{}
	}

	servers := make([]MCPServerInfo, 0, len(cfg.MCPServers))
	for name, entry := range cfg.MCPServers {
		s := MCPServerInfo{Name: name, Enabled: true}
		if entry.Type == "sse" || entry.URL != "" {
			s.Transport = "sse"
			s.URL = entry.URL
		} else {
			s.Transport = "stdio"
			cmd := entry.Command
			if len(entry.Args) > 0 {
				cmd += " " + strings.Join(entry.Args, " ")
			}
			s.Command = cmd
		}
		servers = append(servers, s)
	}

	sort.Slice(servers, func(i, j int) bool {
		return servers[i].Name < servers[j].Name
	})

	return servers
}
