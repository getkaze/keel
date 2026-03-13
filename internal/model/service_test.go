package model

import (
	"encoding/json"
	"testing"
)

func TestService_Unmarshal(t *testing.T) {
	raw := `{
		"name": "redis",
		"hostname": "keel-redis",
		"image": "redis:7",
		"network": "keel-net",
		"ports": {"internal": 6379, "external": 6379},
		"ram_estimate_mb": 64
	}`

	var svc Service
	if err := json.Unmarshal([]byte(raw), &svc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc.Name != "redis" {
		t.Errorf("expected name 'redis', got %q", svc.Name)
	}
	if svc.Ports.Internal != 6379 {
		t.Errorf("expected internal port 6379, got %d", svc.Ports.Internal)
	}
	if svc.Network != "keel-net" {
		t.Errorf("expected network 'keel-net', got %q", svc.Network)
	}
}

func TestService_Marshal_Roundtrip(t *testing.T) {
	svc := Service{
		Name:     "mysql",
		Hostname: "keel-mysql",
		Image:    "mysql:8.4",
		Network:  "keel-net",
		Ports:    PortConfig{Internal: 3306, External: 3306},
	}

	data, err := json.Marshal(svc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var out Service
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if out.Name != "mysql" {
		t.Errorf("roundtrip name mismatch: %q", out.Name)
	}
}

func TestService_OptionalFields_Nil(t *testing.T) {
	raw := `{"name":"s","hostname":"h","image":"i","network":"n","ports":{}}`

	var svc Service
	if err := json.Unmarshal([]byte(raw), &svc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc.HealthCheck != nil {
		t.Error("expected HealthCheck to be nil when absent")
	}
	if len(svc.Logs) != 0 {
		t.Error("expected Logs to be empty when absent")
	}
}

func TestGlobalConfig_Unmarshal(t *testing.T) {
	raw := `{"network":"keel-net","network_subnet":"172.20.1.0/24"}`
	var cfg GlobalConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Network != "keel-net" {
		t.Errorf("expected network 'keel-net', got %q", cfg.Network)
	}
}

func TestContainerStatus_Constants(t *testing.T) {
	cases := []struct {
		status ContainerStatus
		val    string
	}{
		{StatusRunning, "running"},
		{StatusStopped, "stopped"},
		{StatusUnhealthy, "unhealthy"},
		{StatusMissing, "missing"},
	}
	for _, c := range cases {
		if string(c.status) != c.val {
			t.Errorf("expected %q, got %q", c.val, c.status)
		}
	}
}

func TestSystemMetrics_Marshal(t *testing.T) {
	m := SystemMetrics{
		CPU:    CPUMetrics{UsagePercent: 42.5},
		Memory: MemoryMetrics{TotalBytes: 8 * 1024 * 1024 * 1024, UsedBytes: 4 * 1024 * 1024 * 1024, UsagePercent: 50},
		Disk:   DiskMetrics{TotalBytes: 500 * 1024 * 1024 * 1024, UsedBytes: 100 * 1024 * 1024 * 1024, UsagePercent: 20},
	}

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, key := range []string{"cpu", "memory", "disk"} {
		if _, ok := out[key]; !ok {
			t.Errorf("expected %q field in JSON output", key)
		}
	}
}
