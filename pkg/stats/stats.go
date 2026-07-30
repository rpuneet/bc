// Package stats provides fleet metrics and statistics tracking.
package stats

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rpuneet/mycel/pkg/agent"
)

// AgentMetrics tracks agent statistics.
type AgentMetrics struct {
	// Per-agent stats
	AgentStats []AgentStat `json:"agent_stats"`

	// Counts
	TotalAgents  int `json:"total_agents"`
	ActiveAgents int `json:"active_agents"`
	Coordinators int `json:"coordinators"`
	Workers      int `json:"workers"`

	// By state
	Idle    int `json:"idle"`
	Working int `json:"working"`
	Done    int `json:"done"`
	Stuck   int `json:"stuck"`
	Error   int `json:"error"`
	Stopped int `json:"stopped"`
}

// AgentStat holds stats for a single agent.
type AgentStat struct {
	Name   string        `json:"name"`
	Role   string        `json:"role"`
	State  string        `json:"state"`
	Uptime time.Duration `json:"uptime"`
}

// Stats holds fleet statistics.
type Stats struct {
	CollectedAt time.Time `json:"collected_at"`
	path        string
	RepoPath    string       `json:"workspace_path"`
	Agents      AgentMetrics `json:"agents"`
}

// New creates a new Stats instance for the given state dir.
func New(stateDir string) *Stats {
	return &Stats{
		path:        filepath.Join(stateDir, "stats.json"),
		CollectedAt: time.Now(),
	}
}

// Load reads stats from disk and refreshes with live data.
func Load(stateDir string) (*Stats, error) {
	s := New(stateDir)

	// Try to load existing stats for historical data
	data, err := os.ReadFile(s.path)
	if err == nil {
		_ = json.Unmarshal(data, s) //nolint:errcheck // ignore error, use defaults
	}

	// Refresh with live data
	if err := s.refresh(stateDir); err != nil {
		return nil, err
	}

	return s, nil
}

// refresh updates stats from current state.
func (s *Stats) refresh(stateDir string) error {
	s.CollectedAt = time.Now()
	s.RepoPath = filepath.Dir(stateDir)

	// Load agents
	mgr := agent.NewManagerWithRepo(
		filepath.Join(stateDir, "agents"),
		filepath.Dir(stateDir),
	)
	if err := mgr.LoadState(); err != nil {
		// Agents might not exist yet
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to load agents: %w", err)
		}
	}
	s.collectAgentMetrics(mgr)

	return nil
}

// collectAgentMetrics populates agent stats from the manager.
func (s *Stats) collectAgentMetrics(mgr *agent.Manager) {
	agents := mgr.ListAgents()
	m := &s.Agents

	m.TotalAgents = len(agents)
	m.ActiveAgents = 0
	m.Coordinators = 0
	m.Workers = 0
	m.Idle = 0
	m.Working = 0
	m.Done = 0
	m.Stuck = 0
	m.Error = 0
	m.Stopped = 0
	m.AgentStats = nil

	for _, a := range agents {
		// Count by role
		switch a.Role {
		case agent.RoleRoot:
			m.Coordinators++ // Use Coordinators field for root agent count
		default:
			// Custom roles are counted in Workers field
			m.Workers++
		}

		// Count by state
		switch a.State {
		case agent.StateIdle:
			m.Idle++
			m.ActiveAgents++
		case agent.StateWorking:
			m.Working++
			m.ActiveAgents++
		case agent.StateDone:
			m.Done++
			m.ActiveAgents++
		case agent.StateStuck:
			m.Stuck++
			m.ActiveAgents++
		case agent.StateError:
			m.Error++
		case agent.StateStopped:
			m.Stopped++
		}

		// Per-agent stats
		stat := AgentStat{
			Name:  a.Name,
			Role:  string(a.Role),
			State: string(a.State),
		}

		// Calculate uptime
		if a.State != agent.StateStopped && !a.StartedAt.IsZero() {
			stat.Uptime = time.Since(a.StartedAt)
		}

		m.AgentStats = append(m.AgentStats, stat)
	}
}

// Save persists stats to disk.
func (s *Stats) Save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, data, 0600)
}

// Summary returns a formatted string for display.
func (s *Stats) Summary() string {
	var b strings.Builder

	b.WriteString("=== mycel Stats ===\n")
	b.WriteString(fmt.Sprintf("Collected: %s\n\n", s.CollectedAt.Format(time.RFC3339)))

	// Agents
	b.WriteString("--- Agents ---\n")
	b.WriteString(fmt.Sprintf("Total:  %d (%d active)\n", s.Agents.TotalAgents, s.Agents.ActiveAgents))
	b.WriteString(fmt.Sprintf("Roles:  %d coordinators, %d workers\n",
		s.Agents.Coordinators, s.Agents.Workers))
	b.WriteString(fmt.Sprintf("States: %d idle, %d working, %d done, %d stuck, %d stopped\n",
		s.Agents.Idle, s.Agents.Working, s.Agents.Done, s.Agents.Stuck, s.Agents.Stopped))
	b.WriteString("\n")

	if len(s.Agents.AgentStats) > 0 {
		b.WriteString("Per Agent:\n")
		for _, a := range s.Agents.AgentStats {
			uptimeStr := "-"
			if a.Uptime > 0 {
				uptimeStr = formatDuration(a.Uptime)
			}
			b.WriteString(fmt.Sprintf("  %-15s %-12s %-10s uptime:%s\n",
				a.Name, a.Role, a.State, uptimeStr))
		}
	}

	return b.String()
}

// Utilization returns current agent utilization (working/active).
func (s *Stats) Utilization() float64 {
	if s.Agents.ActiveAgents == 0 {
		return 0
	}
	return float64(s.Agents.Working) / float64(s.Agents.ActiveAgents)
}

// formatDuration formats a duration for human display.
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	sec := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, sec)
	}
	return fmt.Sprintf("%ds", sec)
}
