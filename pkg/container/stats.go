package container

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/rpuneet/mycel/pkg/log"
)

// ContainerStats holds resource usage metrics for a single Docker container.
type ContainerStats struct {
	Name          string  `json:"name"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryUsed    int64   `json:"memory_used_bytes"`
	MemoryLimit   int64   `json:"memory_limit_bytes"`
	MemoryPercent float64 `json:"memory_percent"`
	DiskRead      int64   `json:"disk_read_bytes"`
	DiskWrite     int64   `json:"disk_write_bytes"`
	NetRx         int64   `json:"net_rx_bytes"`
	NetTx         int64   `json:"net_tx_bytes"`
	PIDs          int     `json:"pids"`
}

// dockerStatsOneShot maps the JSON response from the Docker container stats API
// (stream=false). We query the Docker socket directly for structured data rather
// than parsing human-readable output from `docker stats`.
type dockerStatsOneShot struct { //nolint:govet // field order matches JSON/API contract
	Networks map[string]struct {
		RxBytes int64 `json:"rx_bytes"`
		TxBytes int64 `json:"tx_bytes"`
	} `json:"networks"`
	Name     string `json:"name"`
	CPUStats struct {
		CPUUsage struct {
			PercpuUsage []int64 `json:"percpu_usage"`
			TotalUsage  int64   `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage int64 `json:"system_cpu_usage"`
		OnlineCPUs     int   `json:"online_cpus"`
	} `json:"cpu_stats"`
	PrecpuStats struct {
		CPUUsage struct {
			TotalUsage int64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage int64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage int64 `json:"usage"`
		Limit int64 `json:"limit"`
		Stats struct {
			InactiveFile int64 `json:"inactive_file"`
			Cache        int64 `json:"cache"`
		} `json:"stats"`
	} `json:"memory_stats"`
	BlkioStats struct {
		IOServiceBytesRecursive []struct {
			Op    string `json:"op"`
			Value int64  `json:"value"`
		} `json:"io_service_bytes_recursive"`
	} `json:"blkio_stats"`
	PidsStats struct {
		Current int `json:"current"`
	} `json:"pids_stats"`
}

// Stats collects resource usage metrics for a single Docker container.
// Uses one-shot stats (stream=false) via the Docker API on the unix socket
// to avoid blocking. Returns zero-value stats if the container is not running.
func Stats(ctx context.Context, containerName string) (ContainerStats, error) {
	// Use curl against the Docker socket for one-shot stats.
	// This avoids adding the docker SDK as a dependency and matches the
	// existing CLI-based pattern, but uses the API directly for structured JSON.
	//nolint:gosec // containerName is from trusted internal sources (containerName method)
	cmd := exec.CommandContext(ctx, "curl", "--silent", "--unix-socket",
		"/var/run/docker.sock",
		fmt.Sprintf("http://localhost/containers/%s/stats?stream=false", containerName))

	output, err := cmd.Output()
	if err != nil {
		// Container may not be running — return zero stats rather than error
		log.Debug("failed to get container stats", "container", containerName, "error", err)
		return ContainerStats{Name: containerName}, nil
	}

	var raw dockerStatsOneShot
	if err := json.Unmarshal(output, &raw); err != nil {
		return ContainerStats{Name: containerName}, fmt.Errorf("failed to parse stats for %s: %w", containerName, err)
	}

	return parseStats(containerName, &raw), nil
}

// parseStats converts raw Docker stats JSON into our ContainerStats struct.
func parseStats(name string, raw *dockerStatsOneShot) ContainerStats {
	cs := ContainerStats{Name: name}

	// CPU percent: (cpuDelta / systemDelta) * numCPUs * 100
	cpuDelta := float64(raw.CPUStats.CPUUsage.TotalUsage - raw.PrecpuStats.CPUUsage.TotalUsage)
	systemDelta := float64(raw.CPUStats.SystemCPUUsage - raw.PrecpuStats.SystemCPUUsage)

	numCPUs := raw.CPUStats.OnlineCPUs
	if numCPUs == 0 {
		numCPUs = len(raw.CPUStats.CPUUsage.PercpuUsage)
	}

	if systemDelta > 0 && numCPUs > 0 {
		cs.CPUPercent = (cpuDelta / systemDelta) * float64(numCPUs) * 100.0
	}

	// Memory: subtract inactive file (cache) from usage for actual memory consumption.
	// This matches how `docker stats` reports memory.
	usedMemory := raw.MemoryStats.Usage - raw.MemoryStats.Stats.InactiveFile
	if usedMemory < 0 {
		usedMemory = raw.MemoryStats.Usage
	}
	cs.MemoryUsed = usedMemory
	cs.MemoryLimit = raw.MemoryStats.Limit

	if cs.MemoryLimit > 0 {
		cs.MemoryPercent = float64(cs.MemoryUsed) / float64(cs.MemoryLimit) * 100.0
	}

	// Disk IO
	for _, entry := range raw.BlkioStats.IOServiceBytesRecursive {
		switch strings.ToLower(entry.Op) {
		case "read":
			cs.DiskRead += entry.Value
		case "write":
			cs.DiskWrite += entry.Value
		}
	}

	// Network IO (sum across all interfaces)
	for _, iface := range raw.Networks {
		cs.NetRx += iface.RxBytes
		cs.NetTx += iface.TxBytes
	}

	// PIDs
	cs.PIDs = raw.PidsStats.Current

	return cs
}

// AllAgentStats collects resource metrics for all running containers matching the prefix.
// The prefix should be the bc container prefix (e.g., "bc-<hash>-") to scope results
// to a single workspace.
func AllAgentStats(ctx context.Context, containerPrefix string) ([]ContainerStats, error) {
	// List running containers matching the prefix using docker ps.
	//nolint:gosec // containerPrefix is from trusted internal sources
	cmd := exec.CommandContext(ctx, "docker", "ps",
		"--filter", "label=bc.managed=true",
		"--filter", "status=running",
		"--format", "{{.Names}}")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var stats []ContainerStats
	for _, name := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if name == "" {
			continue
		}
		if !strings.HasPrefix(name, containerPrefix) {
			continue
		}

		s, sErr := Stats(ctx, name)
		if sErr != nil {
			log.Warn("failed to collect stats for container", "container", name, "error", sErr)
			continue
		}
		stats = append(stats, s)
	}

	return stats, nil
}

// AgentStats is a convenience method on Backend that collects stats for a single agent.
func (b *Backend) AgentStats(ctx context.Context, name string) (ContainerStats, error) {
	return Stats(ctx, b.containerName(name))
}

// AllStats collects stats for all running agents in this workspace.
func (b *Backend) AllStats(ctx context.Context) ([]ContainerStats, error) {
	return AllAgentStats(ctx, b.prefix+b.repoHash+"-")
}
