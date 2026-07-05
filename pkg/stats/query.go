package stats

import (
	"context"
	"fmt"
	"strings"
)

// QuerySystemCPU returns CPU metrics for system containers.
func (s *Store) QuerySystemCPU(ctx context.Context, systems []string, tr TimeRange) ([]SystemMetric, error) {
	return s.querySystem(ctx, systems, tr)
}

// QuerySystemMem returns memory metrics for system containers.
func (s *Store) QuerySystemMem(ctx context.Context, systems []string, tr TimeRange) ([]SystemMetric, error) {
	return s.querySystem(ctx, systems, tr)
}

// QuerySystemNet returns network metrics for system containers.
func (s *Store) QuerySystemNet(ctx context.Context, systems []string, tr TimeRange) ([]SystemMetric, error) {
	return s.querySystem(ctx, systems, tr)
}

// QuerySystemDisk returns disk metrics for system containers.
func (s *Store) QuerySystemDisk(ctx context.Context, systems []string, tr TimeRange) ([]SystemMetric, error) {
	return s.querySystem(ctx, systems, tr)
}

func (s *Store) querySystem(ctx context.Context, systems []string, tr TimeRange) ([]SystemMetric, error) {
	query := `SELECT time_bucket($1::interval, time) AS bucket,
		system_name,
		AVG(cpu_percent), AVG(mem_used_bytes)::BIGINT, AVG(mem_limit_bytes)::BIGINT, AVG(mem_percent),
		AVG(net_rx_bytes)::BIGINT, AVG(net_tx_bytes)::BIGINT,
		AVG(disk_read_bytes)::BIGINT, AVG(disk_write_bytes)::BIGINT
	FROM system_metrics
	WHERE time >= $2 AND time < $3`

	args := []any{tr.PGInterval(), tr.From, tr.To}

	if len(systems) > 0 {
		placeholders := make([]string, len(systems))
		for i, sys := range systems {
			args = append(args, sys)
			placeholders[i] = fmt.Sprintf("$%d", len(args))
		}
		query += ` AND system_name IN (` + strings.Join(placeholders, ",") + `)`
	}

	query += ` GROUP BY bucket, system_name ORDER BY bucket`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query system metrics: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var result []SystemMetric
	for rows.Next() {
		var m SystemMetric
		if err := rows.Scan(&m.Time, &m.SystemName, &m.CPUPercent, &m.MemUsedBytes, &m.MemLimitBytes, &m.MemPercent, &m.NetRxBytes, &m.NetTxBytes, &m.DiskReadBytes, &m.DiskWriteBytes); err != nil {
			return nil, fmt.Errorf("scan system metric: %w", err)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// AgentFilter specifies which agents to query.
type AgentFilter struct {
	Role    string
	Tool    string
	Runtime string
	Agent   []string
}

// QueryAgentCPU returns CPU metrics for agents.
func (s *Store) QueryAgentCPU(ctx context.Context, f AgentFilter, tr TimeRange) ([]AgentMetric, error) {
	return s.queryAgent(ctx, f, tr)
}

// QueryAgentMem returns memory metrics for agents.
func (s *Store) QueryAgentMem(ctx context.Context, f AgentFilter, tr TimeRange) ([]AgentMetric, error) {
	return s.queryAgent(ctx, f, tr)
}

// QueryAgentNet returns network metrics for agents.
func (s *Store) QueryAgentNet(ctx context.Context, f AgentFilter, tr TimeRange) ([]AgentMetric, error) {
	return s.queryAgent(ctx, f, tr)
}

// QueryAgentDisk returns disk metrics for agents.
func (s *Store) QueryAgentDisk(ctx context.Context, f AgentFilter, tr TimeRange) ([]AgentMetric, error) {
	return s.queryAgent(ctx, f, tr)
}

func (s *Store) queryAgent(ctx context.Context, f AgentFilter, tr TimeRange) ([]AgentMetric, error) {
	query := `SELECT time_bucket($1::interval, time) AS bucket,
		agent_name, MAX(role), MAX(tool), MAX(runtime), MAX(state),
		AVG(cpu_percent), AVG(mem_used_bytes)::BIGINT, AVG(mem_limit_bytes)::BIGINT, AVG(mem_percent),
		AVG(net_rx_bytes)::BIGINT, AVG(net_tx_bytes)::BIGINT,
		AVG(disk_read_bytes)::BIGINT, AVG(disk_write_bytes)::BIGINT
	FROM agent_metrics
	WHERE time >= $2 AND time < $3`

	args := []any{tr.PGInterval(), tr.From, tr.To}

	if len(f.Agent) > 0 {
		ph := make([]string, len(f.Agent))
		for i, a := range f.Agent {
			args = append(args, a)
			ph[i] = fmt.Sprintf("$%d", len(args))
		}
		query += ` AND agent_name IN (` + strings.Join(ph, ",") + `)`
	}
	if f.Role != "" {
		args = append(args, f.Role)
		query += fmt.Sprintf(` AND role = $%d`, len(args))
	}
	if f.Tool != "" {
		args = append(args, f.Tool)
		query += fmt.Sprintf(` AND tool = $%d`, len(args))
	}
	if f.Runtime != "" {
		args = append(args, f.Runtime)
		query += fmt.Sprintf(` AND runtime = $%d`, len(args))
	}

	query += ` GROUP BY bucket, agent_name ORDER BY bucket`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query agent metrics: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var result []AgentMetric
	for rows.Next() {
		var m AgentMetric
		if err := rows.Scan(&m.Time, &m.AgentName, &m.Role, &m.Tool, &m.Runtime, &m.State,
			&m.CPUPercent, &m.MemUsedBytes, &m.MemLimitBytes, &m.MemPercent,
			&m.NetRxBytes, &m.NetTxBytes, &m.DiskReadBytes, &m.DiskWriteBytes); err != nil {
			return nil, fmt.Errorf("scan agent metric: %w", err)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// ChannelFilter specifies which channels to query.
type ChannelFilter struct {
	Channel []string
}

// QueryChannelMessages returns message count metrics.
func (s *Store) QueryChannelMessages(ctx context.Context, f ChannelFilter, tr TimeRange) ([]ChannelMetric, error) {
	return s.queryChannel(ctx, f, tr)
}

// QueryChannelMembers returns member count metrics.
func (s *Store) QueryChannelMembers(ctx context.Context, f ChannelFilter, tr TimeRange) ([]ChannelMetric, error) {
	return s.queryChannel(ctx, f, tr)
}

// QueryChannelReactions returns reaction count metrics.
func (s *Store) QueryChannelReactions(ctx context.Context, f ChannelFilter, tr TimeRange) ([]ChannelMetric, error) {
	return s.queryChannel(ctx, f, tr)
}

func (s *Store) queryChannel(ctx context.Context, f ChannelFilter, tr TimeRange) ([]ChannelMetric, error) {
	query := `SELECT time_bucket($1::interval, time) AS bucket,
		channel_name, SUM(message_count), MAX(member_count), SUM(reaction_count)
	FROM channel_metrics
	WHERE time >= $2 AND time < $3`

	args := []any{tr.PGInterval(), tr.From, tr.To}

	if len(f.Channel) > 0 {
		ph := make([]string, len(f.Channel))
		for i, ch := range f.Channel {
			args = append(args, ch)
			ph[i] = fmt.Sprintf("$%d", len(args))
		}
		query += ` AND channel_name IN (` + strings.Join(ph, ",") + `)`
	}

	query += ` GROUP BY bucket, channel_name ORDER BY bucket`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query channel metrics: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var result []ChannelMetric
	for rows.Next() {
		var m ChannelMetric
		if err := rows.Scan(&m.Time, &m.ChannelName, &m.MessageCount, &m.MemberCount, &m.ReactionCount); err != nil {
			return nil, fmt.Errorf("scan channel metric: %w", err)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// AgentSummary combines resource metrics and token/cost totals for a single agent.
type AgentSummary struct { //nolint:govet // field order matches JSON/API contract
	// Token and cost totals (over period)
	Models []ModelCostBreakdown `json:"models,omitempty"`

	AgentName string `json:"agent_name"`
	Role      string `json:"role"`
	Tool      string `json:"tool"`
	Runtime   string `json:"runtime"`
	State     string `json:"state"`

	// Resource metrics (latest sample)
	CPU    CPUSummary    `json:"cpu"`
	Memory MemorySummary `json:"memory"`
	Disk   DiskSummary   `json:"disk"`
	Net    NetSummary    `json:"network"`

	Tokens TokenSummary `json:"tokens"`
	Cost   CostSummary  `json:"cost"`
}

// CPUSummary holds aggregated CPU metrics.
type CPUSummary struct {
	AvgPercent float64 `json:"avg_percent"`
	MaxPercent float64 `json:"max_percent"`
}

// MemorySummary holds aggregated memory metrics.
type MemorySummary struct {
	AvgBytes   int64   `json:"avg_bytes"`
	MaxBytes   int64   `json:"max_bytes"`
	AvgPercent float64 `json:"avg_percent"`
}

// DiskSummary holds aggregated disk I/O metrics.
type DiskSummary struct {
	ReadBytes  int64 `json:"read_bytes"`
	WriteBytes int64 `json:"write_bytes"`
}

// NetSummary holds aggregated network metrics.
type NetSummary struct {
	RxBytes int64 `json:"rx_bytes"`
	TxBytes int64 `json:"tx_bytes"`
}

// TokenSummary holds aggregated token usage.
type TokenSummary struct {
	Input       int64 `json:"input"`
	Output      int64 `json:"output"`
	CacheRead   int64 `json:"cache_read"`
	CacheCreate int64 `json:"cache_create"`
}

// CostSummary holds aggregated cost data.
type CostSummary struct {
	TotalUSD float64 `json:"total_usd"`
}

// ModelCostBreakdown shows cost per model.
type ModelCostBreakdown struct {
	Model        string  `json:"model"`
	CostUSD      float64 `json:"cost_usd"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
}

// QueryAgentSummary returns a resource-metrics summary (avg/max over period)
// for a single agent. Token and cost totals are sourced from the cost ledger
// (pkg/cost, /api/costs/*) rather than the stats store, so the Tokens, Cost,
// and Models fields are left zero-valued here.
func (s *Store) QueryAgentSummary(ctx context.Context, agentName string, tr TimeRange) (*AgentSummary, error) {
	summary := &AgentSummary{AgentName: agentName}

	// Query resource metrics (aggregated over period)
	resQuery := `SELECT
		COALESCE(MAX(role), ''), COALESCE(MAX(tool), ''), COALESCE(MAX(runtime), ''), COALESCE(MAX(state), ''),
		COALESCE(AVG(cpu_percent), 0), COALESCE(MAX(cpu_percent), 0),
		COALESCE(AVG(mem_used_bytes)::BIGINT, 0), COALESCE(MAX(mem_used_bytes)::BIGINT, 0), COALESCE(AVG(mem_percent), 0),
		COALESCE(AVG(net_rx_bytes)::BIGINT, 0), COALESCE(AVG(net_tx_bytes)::BIGINT, 0),
		COALESCE(AVG(disk_read_bytes)::BIGINT, 0), COALESCE(AVG(disk_write_bytes)::BIGINT, 0)
	FROM agent_metrics
	WHERE agent_name = $1 AND time >= $2 AND time < $3`

	err := s.db.QueryRowContext(ctx, resQuery, agentName, tr.From, tr.To).Scan(
		&summary.Role, &summary.Tool, &summary.Runtime, &summary.State,
		&summary.CPU.AvgPercent, &summary.CPU.MaxPercent,
		&summary.Memory.AvgBytes, &summary.Memory.MaxBytes, &summary.Memory.AvgPercent,
		&summary.Net.RxBytes, &summary.Net.TxBytes,
		&summary.Disk.ReadBytes, &summary.Disk.WriteBytes,
	)
	if err != nil {
		return nil, fmt.Errorf("query agent resource summary: %w", err)
	}

	return summary, nil
}

// QueryLatestAgentMetrics returns the most recent metric sample for each agent.
// Used by the agents list table to show current CPU/Mem without N+1 queries.
func (s *Store) QueryLatestAgentMetrics(ctx context.Context) ([]AgentMetric, error) {
	query := `SELECT DISTINCT ON (agent_name)
		time, agent_name, role, tool, runtime, state,
		cpu_percent, mem_used_bytes, mem_limit_bytes, mem_percent,
		net_rx_bytes, net_tx_bytes, disk_read_bytes, disk_write_bytes
	FROM agent_metrics
	ORDER BY agent_name, time DESC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query latest agent metrics: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var result []AgentMetric
	for rows.Next() {
		var m AgentMetric
		if err := rows.Scan(&m.Time, &m.AgentName, &m.Role, &m.Tool, &m.Runtime, &m.State,
			&m.CPUPercent, &m.MemUsedBytes, &m.MemLimitBytes, &m.MemPercent,
			&m.NetRxBytes, &m.NetTxBytes, &m.DiskReadBytes, &m.DiskWriteBytes); err != nil {
			return nil, fmt.Errorf("scan latest agent metric: %w", err)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}
