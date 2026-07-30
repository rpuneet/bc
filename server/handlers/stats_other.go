//go:build !linux

package handlers

import (
	"bufio"
	"context"
	"os/exec"
	"strconv"
	"strings"
)

// getSystemMetrics returns system resource metrics using macOS CLI tools.
// Falls back to zero values if any command fails.
func getSystemMetrics(ctx context.Context, rootDir string) systemMetrics {
	var m systemMetrics

	m.MemoryTotalBytes, m.MemoryUsedBytes = macMemory(ctx)
	if m.MemoryTotalBytes > 0 {
		m.MemoryPercent = roundTo(float64(m.MemoryUsedBytes)/float64(m.MemoryTotalBytes)*100, 1)
	}

	m.CPUUsagePercent = macCPU(ctx)

	m.DiskTotalBytes, m.DiskUsedBytes = diskUsage(ctx, rootDir)
	if m.DiskTotalBytes > 0 {
		m.DiskPercent = roundTo(float64(m.DiskUsedBytes)/float64(m.DiskTotalBytes)*100, 1)
	}

	return m
}

// macMemory returns total and used physical RAM on macOS via sysctl and vm_stat.
func macMemory(ctx context.Context) (total, used uint64) {
	// Total RAM from sysctl
	out, err := exec.CommandContext(ctx, "sysctl", "-n", "hw.memsize").Output() //nolint:gosec // trusted fixed command
	if err != nil {
		return 0, 0
	}
	total, err = strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0, 0
	}

	// Page size from sysctl
	out, err = exec.CommandContext(ctx, "sysctl", "-n", "hw.pagesize").Output() //nolint:gosec // trusted fixed command
	if err != nil {
		return total, 0
	}
	pageSize, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return total, 0
	}

	// Parse vm_stat for active + wired pages
	out, err = exec.CommandContext(ctx, "vm_stat").Output() //nolint:gosec // trusted fixed command
	if err != nil {
		return total, 0
	}

	var active, wired uint64
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if v, ok := parseVMStatLine(line, "Pages active:"); ok {
			active = v
		} else if v, ok := parseVMStatLine(line, "Pages wired down:"); ok {
			wired = v
		}
	}

	used = (active + wired) * pageSize
	return total, used
}

// parseVMStatLine extracts the page count from a vm_stat output line matching the given prefix.
func parseVMStatLine(line, prefix string) (uint64, bool) {
	if !strings.HasPrefix(line, prefix) {
		return 0, false
	}
	s := strings.TrimPrefix(line, prefix)
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, ".")
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// macCPU returns current CPU usage percentage on macOS via top.
func macCPU(ctx context.Context) float64 {
	out, err := exec.CommandContext(ctx, "top", "-l", "1", "-n", "0", "-s", "0").Output() //nolint:gosec // trusted fixed command
	if err != nil {
		return 0
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		// Line format: "CPU usage: 12.50% user, 5.30% sys, 82.20% idle"
		if !strings.HasPrefix(line, "CPU usage:") {
			continue
		}
		line = strings.TrimPrefix(line, "CPU usage:")
		parts := strings.Split(line, ",")
		var userPct, sysPct float64
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if strings.HasSuffix(part, "user") {
				userPct = parsePercentField(part)
			} else if strings.HasSuffix(part, "sys") {
				sysPct = parsePercentField(part)
			}
		}
		return roundTo(userPct+sysPct, 1)
	}
	return 0
}

// parsePercentField extracts a float from a string like "12.50% user".
func parsePercentField(s string) float64 {
	s = strings.TrimSpace(s)
	idx := strings.Index(s, "%")
	if idx < 0 {
		return 0
	}
	v, err := strconv.ParseFloat(s[:idx], 64)
	if err != nil {
		return 0
	}
	return v
}

// diskUsage returns total and used disk bytes for the filesystem containing path, via df.
func diskUsage(ctx context.Context, path string) (total, used uint64) {
	out, err := exec.CommandContext(ctx, "df", "-k", path).Output() //nolint:gosec // path is repo root dir
	if err != nil {
		return 0, 0
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum < 2 {
			continue // skip header
		}
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		// df -k columns: Filesystem 1K-blocks Used Available ...
		totalKB, err1 := strconv.ParseUint(fields[1], 10, 64)
		usedKB, err2 := strconv.ParseUint(fields[2], 10, 64)
		if err1 != nil || err2 != nil {
			return 0, 0
		}
		return totalKB * 1024, usedKB * 1024
	}
	return 0, 0
}
