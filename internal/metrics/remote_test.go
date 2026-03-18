package metrics

import (
	"strings"
	"testing"
)

const sampleRemoteOutput = `cpu  7894 123 4567 890123 456 0 78 0 0 0
cpu  7900 123 4570 890200 456 0 80 0 0 0
MemTotal:       16384000 kB
MemFree:         2048000 kB
MemAvailable:    8192000 kB
Buffers:          512000 kB
---LOADAVG---
1.25 0.85 0.50 2/345 12345
---UPTIME---
86400.50 172800.00
---DISK---
/dev/sda1 500000000000 200000000000 250000000000 45% /
`

func TestParseRemoteMetrics_CPU(t *testing.T) {
	cpu, _, _, _, _, err := parseRemoteMetrics(sampleRemoteOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cpu.UsagePercent < 0 || cpu.UsagePercent > 100 {
		t.Errorf("CPU usage out of range: %f", cpu.UsagePercent)
	}
}

func TestParseRemoteMetrics_Memory(t *testing.T) {
	_, mem, _, _, _, err := parseRemoteMetrics(sampleRemoteOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedTotal := uint64(16384000) * 1024
	if mem.TotalBytes != expectedTotal {
		t.Errorf("expected total %d, got %d", expectedTotal, mem.TotalBytes)
	}
	expectedAvail := uint64(8192000) * 1024
	if mem.AvailableBytes != expectedAvail {
		t.Errorf("expected available %d, got %d", expectedAvail, mem.AvailableBytes)
	}
	if mem.UsedBytes != mem.TotalBytes-mem.AvailableBytes {
		t.Errorf("used bytes mismatch: %d != %d - %d", mem.UsedBytes, mem.TotalBytes, mem.AvailableBytes)
	}
	if mem.UsagePercent <= 0 || mem.UsagePercent >= 100 {
		t.Errorf("memory usage out of range: %f", mem.UsagePercent)
	}
}

func TestParseRemoteMetrics_LoadAvg(t *testing.T) {
	_, _, _, la, _, err := parseRemoteMetrics(sampleRemoteOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if la.Load1 != 1.25 {
		t.Errorf("expected Load1=1.25, got %f", la.Load1)
	}
	if la.Load5 != 0.85 {
		t.Errorf("expected Load5=0.85, got %f", la.Load5)
	}
	if la.Load15 != 0.50 {
		t.Errorf("expected Load15=0.50, got %f", la.Load15)
	}
}

func TestParseRemoteMetrics_Uptime(t *testing.T) {
	_, _, _, _, up, err := parseRemoteMetrics(sampleRemoteOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if up.UptimeSeconds != 86400.50 {
		t.Errorf("expected uptime 86400.50, got %f", up.UptimeSeconds)
	}
}

func TestParseRemoteMetrics_Disk(t *testing.T) {
	_, _, disk, _, _, err := parseRemoteMetrics(sampleRemoteOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if disk.TotalBytes != 500000000000 {
		t.Errorf("expected total 500000000000, got %d", disk.TotalBytes)
	}
	if disk.UsedBytes != 200000000000 {
		t.Errorf("expected used 200000000000, got %d", disk.UsedBytes)
	}
	if disk.AvailableBytes != 250000000000 {
		t.Errorf("expected available 250000000000, got %d", disk.AvailableBytes)
	}
	if disk.UsagePercent <= 0 {
		t.Errorf("expected positive disk usage, got %f", disk.UsagePercent)
	}
}

func TestParseRemoteMetrics_TooShort(t *testing.T) {
	_, _, _, _, _, err := parseRemoteMetrics("short")
	if err == nil {
		t.Error("expected error for short output")
	}
}

func TestParseCPULine_Valid(t *testing.T) {
	idle, total := parseCPULine("cpu  7894 123 4567 890123 456 0 78 0 0 0")
	if idle != 890123 {
		t.Errorf("expected idle=890123, got %d", idle)
	}
	if total == 0 {
		t.Error("expected non-zero total")
	}
}

func TestParseCPULine_TooShort(t *testing.T) {
	idle, total := parseCPULine("cpu 1 2")
	if idle != 0 || total != 0 {
		t.Errorf("expected 0,0 for short line, got %d,%d", idle, total)
	}
}

func TestParseRemoteMetrics_EmptySections(t *testing.T) {
	// Only CPU lines, no sections
	raw := strings.Join([]string{
		"cpu  100 0 50 800 10 0 5 0 0 0",
		"cpu  110 0 55 810 10 0 6 0 0 0",
		"MemTotal:       8192000 kB",
		"MemAvailable:   4096000 kB",
		"---LOADAVG---",
		"---UPTIME---",
		"---DISK---",
	}, "\n")
	cpu, mem, _, la, up, err := parseRemoteMetrics(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cpu.UsagePercent < 0 {
		t.Error("expected valid CPU usage")
	}
	if mem.TotalBytes == 0 {
		t.Error("expected non-zero memory")
	}
	// Empty sections should yield zero values
	if la.Load1 != 0 {
		t.Errorf("expected Load1=0 for empty section, got %f", la.Load1)
	}
	if up.UptimeSeconds != 0 {
		t.Errorf("expected uptime=0 for empty section, got %f", up.UptimeSeconds)
	}
}
