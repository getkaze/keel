package metrics

import (
	"testing"
)

func TestReadCPU_ReturnsValidStruct(t *testing.T) {
	cpu, err := ReadCPU()
	if err != nil {
		t.Fatalf("ReadCPU error: %v", err)
	}
	if cpu.UsagePercent < 0 || cpu.UsagePercent > 100 {
		t.Errorf("CPU usage out of range: %f", cpu.UsagePercent)
	}
}

func TestReadMemory_ReturnsValidStruct(t *testing.T) {
	mem, err := ReadMemory()
	if err != nil {
		t.Fatalf("ReadMemory error: %v", err)
	}
	if mem.TotalBytes == 0 {
		t.Error("expected non-zero TotalBytes")
	}
	if mem.UsagePercent < 0 || mem.UsagePercent > 100 {
		t.Errorf("memory usage out of range: %f", mem.UsagePercent)
	}
	if mem.UsedBytes > mem.TotalBytes {
		t.Errorf("used (%d) > total (%d)", mem.UsedBytes, mem.TotalBytes)
	}
}

func TestReadDisk_ReturnsValidStruct(t *testing.T) {
	disk, err := ReadDisk()
	if err != nil {
		t.Fatalf("ReadDisk error: %v", err)
	}
	if disk.TotalBytes == 0 {
		t.Error("expected non-zero TotalBytes")
	}
	if disk.UsagePercent < 0 || disk.UsagePercent > 100 {
		t.Errorf("disk usage out of range: %f", disk.UsagePercent)
	}
}

func TestReadLoadAvg_ReturnsValidStruct(t *testing.T) {
	la, err := ReadLoadAvg()
	if err != nil {
		t.Fatalf("ReadLoadAvg error: %v", err)
	}
	if la.Load1 < 0 {
		t.Errorf("Load1 should be non-negative, got %f", la.Load1)
	}
}

func TestReadUptime_ReturnsValidStruct(t *testing.T) {
	up, err := ReadUptime()
	if err != nil {
		t.Fatalf("ReadUptime error: %v", err)
	}
	if up.UptimeSeconds <= 0 {
		t.Errorf("expected positive uptime, got %f", up.UptimeSeconds)
	}
}

func TestReadCPU_UsageIsReasonable(t *testing.T) {
	cpu, err := ReadCPU()
	if err != nil {
		t.Fatalf("ReadCPU error: %v", err)
	}
	// Just verify it's a valid percentage
	if cpu.UsagePercent < 0 || cpu.UsagePercent > 100 {
		t.Errorf("CPU usage out of range [0-100]: %f", cpu.UsagePercent)
	}
}
