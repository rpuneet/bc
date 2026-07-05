package stats

import (
	"context"
	"fmt"
)

// RecordSystem inserts a system container metric sample.
func (s *Store) RecordSystem(ctx context.Context, m SystemMetric) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO system_metrics (time, system_name, cpu_percent, mem_used_bytes, mem_limit_bytes, mem_percent, net_rx_bytes, net_tx_bytes, disk_read_bytes, disk_write_bytes)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		m.Time, m.SystemName, m.CPUPercent, m.MemUsedBytes, m.MemLimitBytes, m.MemPercent, m.NetRxBytes, m.NetTxBytes, m.DiskReadBytes, m.DiskWriteBytes,
	)
	if err != nil {
		return fmt.Errorf("record system metric: %w", err)
	}
	return nil
}

// RecordAgent inserts an agent container metric sample.
func (s *Store) RecordAgent(ctx context.Context, m AgentMetric) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_metrics (time, agent_name, role, tool, runtime, state, cpu_percent, mem_used_bytes, mem_limit_bytes, mem_percent, net_rx_bytes, net_tx_bytes, disk_read_bytes, disk_write_bytes)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		m.Time, m.AgentName, m.Role, m.Tool, m.Runtime, m.State, m.CPUPercent, m.MemUsedBytes, m.MemLimitBytes, m.MemPercent, m.NetRxBytes, m.NetTxBytes, m.DiskReadBytes, m.DiskWriteBytes,
	)
	if err != nil {
		return fmt.Errorf("record agent metric: %w", err)
	}
	return nil
}

// RecordChannel inserts a channel activity sample.
func (s *Store) RecordChannel(ctx context.Context, m ChannelMetric) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO channel_metrics (time, channel_name, message_count, member_count, reaction_count)
		 VALUES ($1, $2, $3, $4, $5)`,
		m.Time, m.ChannelName, m.MessageCount, m.MemberCount, m.ReactionCount,
	)
	if err != nil {
		return fmt.Errorf("record channel metric: %w", err)
	}
	return nil
}
