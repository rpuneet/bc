package container

import (
	"testing"
)

func TestParseStats_CPUPercent(t *testing.T) {
	raw := &dockerStatsOneShot{}
	raw.CPUStats.CPUUsage.TotalUsage = 500_000_000
	raw.PrecpuStats.CPUUsage.TotalUsage = 400_000_000
	raw.CPUStats.SystemCPUUsage = 2_000_000_000
	raw.PrecpuStats.SystemCPUUsage = 1_000_000_000
	raw.CPUStats.OnlineCPUs = 4

	cs := parseStats("test-container", raw)

	// cpuDelta=100M, systemDelta=1000M => (100/1000)*4*100 = 40%
	want := 40.0
	if cs.CPUPercent != want {
		t.Errorf("CPUPercent = %f, want %f", cs.CPUPercent, want)
	}
}

func TestParseStats_CPUPercent_ZeroDelta(t *testing.T) {
	raw := &dockerStatsOneShot{}
	raw.CPUStats.CPUUsage.TotalUsage = 100
	raw.PrecpuStats.CPUUsage.TotalUsage = 100
	raw.CPUStats.SystemCPUUsage = 1000
	raw.PrecpuStats.SystemCPUUsage = 1000
	raw.CPUStats.OnlineCPUs = 2

	cs := parseStats("test-container", raw)

	if cs.CPUPercent != 0 {
		t.Errorf("CPUPercent = %f, want 0 (zero delta)", cs.CPUPercent)
	}
}

func TestParseStats_CPUPercent_FallbackPercpuUsage(t *testing.T) {
	raw := &dockerStatsOneShot{}
	raw.CPUStats.CPUUsage.TotalUsage = 200_000_000
	raw.PrecpuStats.CPUUsage.TotalUsage = 100_000_000
	raw.CPUStats.SystemCPUUsage = 2_000_000_000
	raw.PrecpuStats.SystemCPUUsage = 1_000_000_000
	raw.CPUStats.OnlineCPUs = 0                           // force fallback
	raw.CPUStats.CPUUsage.PercpuUsage = []int64{100, 200} // 2 CPUs

	cs := parseStats("test-container", raw)

	// cpuDelta=100M, systemDelta=1000M => (100/1000)*2*100 = 20%
	want := 20.0
	if cs.CPUPercent != want {
		t.Errorf("CPUPercent = %f, want %f (fallback to percpu_usage len)", cs.CPUPercent, want)
	}
}

func TestParseStats_Memory(t *testing.T) {
	raw := &dockerStatsOneShot{}
	raw.MemoryStats.Usage = 1_073_741_824 // 1GiB
	raw.MemoryStats.Limit = 4_294_967_296 // 4GiB
	raw.MemoryStats.Stats.InactiveFile = 100_000_000

	cs := parseStats("test-container", raw)

	wantUsed := int64(1_073_741_824 - 100_000_000)
	if cs.MemoryUsed != wantUsed {
		t.Errorf("MemoryUsed = %d, want %d", cs.MemoryUsed, wantUsed)
	}
	if cs.MemoryLimit != 4_294_967_296 {
		t.Errorf("MemoryLimit = %d, want 4294967296", cs.MemoryLimit)
	}

	wantPercent := float64(wantUsed) / float64(cs.MemoryLimit) * 100.0
	if cs.MemoryPercent != wantPercent {
		t.Errorf("MemoryPercent = %f, want %f", cs.MemoryPercent, wantPercent)
	}
}

func TestParseStats_MemoryZeroLimit(t *testing.T) {
	raw := &dockerStatsOneShot{}
	raw.MemoryStats.Usage = 500_000
	raw.MemoryStats.Limit = 0

	cs := parseStats("test-container", raw)

	if cs.MemoryPercent != 0 {
		t.Errorf("MemoryPercent = %f, want 0 (zero limit)", cs.MemoryPercent)
	}
}

func TestParseStats_DiskIO(t *testing.T) {
	raw := &dockerStatsOneShot{}
	raw.BlkioStats.IOServiceBytesRecursive = []struct {
		Op    string `json:"op"`
		Value int64  `json:"value"`
	}{
		{Op: "Read", Value: 1024},
		{Op: "Write", Value: 2048},
		{Op: "Read", Value: 512},
		{Op: "Write", Value: 256},
		{Op: "Sync", Value: 999}, // should be ignored
	}

	cs := parseStats("test-container", raw)

	if cs.DiskRead != 1536 {
		t.Errorf("DiskRead = %d, want 1536", cs.DiskRead)
	}
	if cs.DiskWrite != 2304 {
		t.Errorf("DiskWrite = %d, want 2304", cs.DiskWrite)
	}
}

func TestParseStats_NetworkIO(t *testing.T) {
	raw := &dockerStatsOneShot{}
	raw.Networks = map[string]struct {
		RxBytes int64 `json:"rx_bytes"`
		TxBytes int64 `json:"tx_bytes"`
	}{
		"eth0": {RxBytes: 1000, TxBytes: 2000},
		"eth1": {RxBytes: 3000, TxBytes: 4000},
	}

	cs := parseStats("test-container", raw)

	if cs.NetRx != 4000 {
		t.Errorf("NetRx = %d, want 4000", cs.NetRx)
	}
	if cs.NetTx != 6000 {
		t.Errorf("NetTx = %d, want 6000", cs.NetTx)
	}
}

func TestParseStats_PIDs(t *testing.T) {
	raw := &dockerStatsOneShot{}
	raw.PidsStats.Current = 42

	cs := parseStats("test-container", raw)

	if cs.PIDs != 42 {
		t.Errorf("PIDs = %d, want 42", cs.PIDs)
	}
}

func TestParseStats_EmptyRaw(t *testing.T) {
	raw := &dockerStatsOneShot{}

	cs := parseStats("empty-container", raw)

	if cs.Name != "empty-container" {
		t.Errorf("Name = %q, want %q", cs.Name, "empty-container")
	}
	if cs.CPUPercent != 0 {
		t.Errorf("CPUPercent = %f, want 0", cs.CPUPercent)
	}
	if cs.MemoryUsed != 0 {
		t.Errorf("MemoryUsed = %d, want 0", cs.MemoryUsed)
	}
	if cs.PIDs != 0 {
		t.Errorf("PIDs = %d, want 0", cs.PIDs)
	}
}

func TestParseStats_Name(t *testing.T) {
	raw := &dockerStatsOneShot{}
	cs := parseStats("bc-a1b2c3-alice", raw)

	if cs.Name != "bc-a1b2c3-alice" {
		t.Errorf("Name = %q, want %q", cs.Name, "bc-a1b2c3-alice")
	}
}

func TestBackendAgentStats_ContainerName(t *testing.T) {
	b := &Backend{
		prefix:   "bc-",
		repoHash: "aabbcc",
	}

	// Verify the container name used for stats lookup matches the convention
	cn := b.containerName("alice")
	want := "bc-aabbcc-alice"
	if cn != want {
		t.Errorf("containerName = %q, want %q", cn, want)
	}
}
