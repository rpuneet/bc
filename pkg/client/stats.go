package client

import "context"

// StatsClient provides system and fleet stats via the daemon.
type StatsClient struct {
	client *Client
}

// SystemStats represents system-level metrics returned by the daemon.
type SystemStats struct {
	Hostname         string  `json:"hostname"`
	OS               string  `json:"os"`
	Arch             string  `json:"arch"`
	GoVersion        string  `json:"go_version"`
	CPUUsagePercent  float64 `json:"cpu_usage_percent"`
	MemoryPercent    float64 `json:"memory_usage_percent"`
	DiskPercent      float64 `json:"disk_usage_percent"`
	MemoryTotalBytes uint64  `json:"memory_total_bytes"`
	MemoryUsedBytes  uint64  `json:"memory_used_bytes"`
	DiskTotalBytes   uint64  `json:"disk_total_bytes"`
	DiskUsedBytes    uint64  `json:"disk_used_bytes"`
	UptimeSeconds    int64   `json:"uptime_seconds"`
	CPUs             int     `json:"cpus"`
	Goroutines       int     `json:"goroutines"`
}

// SummaryStats represents fleet-level summary metrics.
type SummaryStats struct {
	AgentsTotal   int     `json:"agents_total"`
	AgentsRunning int     `json:"agents_running"`
	AgentsStopped int     `json:"agents_stopped"`
	ChannelsTotal int     `json:"channels_total"`
	MessagesTotal int     `json:"messages_total"`
	TotalCostUSD  float64 `json:"total_cost_usd"`
	RolesTotal    int     `json:"roles_total"`
	ToolsTotal    int     `json:"tools_total"`
	UptimeSeconds int64   `json:"uptime_seconds"`
}

// System returns system-level metrics.
func (s *StatsClient) System(ctx context.Context) (*SystemStats, error) {
	var stats SystemStats
	if err := s.client.get(ctx, "/api/stats/system", &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

// Summary returns fleet-level summary metrics.
func (s *StatsClient) Summary(ctx context.Context) (*SummaryStats, error) {
	var stats SummaryStats
	if err := s.client.get(ctx, "/api/stats/summary", &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}
