package metrics

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/getkaze/keel/internal/config"
	"github.com/getkaze/keel/internal/model"
	keelssh "github.com/getkaze/keel/internal/ssh"
)

const (
	cacheTTL       = 10 * time.Second
	sshExecTimeout = 5 * time.Second
)

type cachedMetrics struct {
	cpu    model.CPUMetrics
	mem    model.MemoryMetrics
	disk   model.DiskMetrics
	load   model.LoadAvgMetrics
	uptime model.UptimeMetrics
	err    error
	at     time.Time
}

// RemoteCollector reads system metrics from a remote host via SSH.
type RemoteCollector struct {
	target *config.TargetConfig

	mu       sync.RWMutex
	cache    *cachedMetrics
	fetching bool
}

// NewRemoteCollector creates a collector for the given remote target.
func NewRemoteCollector(target *config.TargetConfig) *RemoteCollector {
	return &RemoteCollector{target: target}
}

// SetTarget updates the remote target and invalidates the cache.
func (rc *RemoteCollector) SetTarget(target *config.TargetConfig) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.target = target
	rc.cache = nil
	rc.fetching = false
}

// ReadAll collects CPU, memory, disk, load average, and uptime from the remote host
// in a single SSH call to minimize latency. Results are cached for 10 seconds;
// expired cache triggers a background refresh while returning stale data.
func (rc *RemoteCollector) ReadAll() (model.CPUMetrics, model.MemoryMetrics, model.DiskMetrics, model.LoadAvgMetrics, model.UptimeMetrics, error) {
	rc.mu.RLock()
	c := rc.cache
	rc.mu.RUnlock()

	if c != nil && time.Since(c.at) < cacheTTL {
		return c.cpu, c.mem, c.disk, c.load, c.uptime, c.err
	}

	// Cache expired or empty — check if a background fetch is already running.
	rc.mu.Lock()
	if rc.fetching {
		// Another goroutine is already fetching; return stale data if available.
		c = rc.cache
		rc.mu.Unlock()
		if c != nil {
			return c.cpu, c.mem, c.disk, c.load, c.uptime, c.err
		}
		// No stale data — do a blocking fetch.
		return rc.fetchAndCache()
	}

	if rc.cache != nil && time.Since(rc.cache.at) < cacheTTL {
		// Re-check after acquiring write lock — another goroutine may have refreshed.
		c = rc.cache
		rc.mu.Unlock()
		return c.cpu, c.mem, c.disk, c.load, c.uptime, c.err
	}

	hasStale := rc.cache != nil
	rc.fetching = true
	rc.mu.Unlock()

	if hasStale {
		// Return stale data immediately, refresh in background.
		go func() {
			defer func() {
				rc.mu.Lock()
				rc.fetching = false
				rc.mu.Unlock()
			}()
			rc.fetchAndCache()
		}()
		rc.mu.RLock()
		c = rc.cache
		rc.mu.RUnlock()
		return c.cpu, c.mem, c.disk, c.load, c.uptime, c.err
	}

	// First call ever — blocking fetch.
	cpu, mem, disk, load, uptime, err := rc.fetchAndCache()
	rc.mu.Lock()
	rc.fetching = false
	rc.mu.Unlock()
	return cpu, mem, disk, load, uptime, err
}

func (rc *RemoteCollector) fetchAndCache() (model.CPUMetrics, model.MemoryMetrics, model.DiskMetrics, model.LoadAvgMetrics, model.UptimeMetrics, error) {
	cpu, mem, disk, load, uptime, err := rc.fetchRemote()

	rc.mu.Lock()
	rc.cache = &cachedMetrics{
		cpu: cpu, mem: mem, disk: disk,
		load: load, uptime: uptime,
		err: err, at: time.Now(),
	}
	rc.mu.Unlock()

	return cpu, mem, disk, load, uptime, err
}

func (rc *RemoteCollector) fetchRemote() (model.CPUMetrics, model.MemoryMetrics, model.DiskMetrics, model.LoadAvgMetrics, model.UptimeMetrics, error) {
	// Single command that reads all proc files + disk usage.
	// Two CPU samples 1 second apart for usage calculation.
	script := `cat /proc/stat | head -1; sleep 1; cat /proc/stat | head -1; cat /proc/meminfo; echo '---LOADAVG---'; cat /proc/loadavg; echo '---UPTIME---'; cat /proc/uptime; echo '---DISK---'; df -B1 / | tail -1`

	out, err := rc.sshExec(script)
	if err != nil {
		return model.CPUMetrics{}, model.MemoryMetrics{}, model.DiskMetrics{}, model.LoadAvgMetrics{}, model.UptimeMetrics{}, err
	}

	return parseRemoteMetrics(out)
}

func (rc *RemoteCollector) sshExec(cmd string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sshExecTimeout)
	defer cancel()

	args := buildSSHArgs(rc.target)
	args = append(args, cmd)

	var stdout, stderr bytes.Buffer
	c := exec.CommandContext(ctx, "ssh", args...)
	c.Stdout = &stdout
	c.Stderr = &stderr

	if err := c.Run(); err != nil {
		return "", fmt.Errorf("ssh metrics: %w: %s", err, stderr.String())
	}
	return stdout.String(), nil
}

func buildSSHArgs(t *config.TargetConfig) []string {
	return keelssh.BuildArgs(t)
}

func parseRemoteMetrics(raw string) (model.CPUMetrics, model.MemoryMetrics, model.DiskMetrics, model.LoadAvgMetrics, model.UptimeMetrics, error) {
	var cpu model.CPUMetrics
	var mem model.MemoryMetrics
	var disk model.DiskMetrics
	var loadAvg model.LoadAvgMetrics
	var uptime model.UptimeMetrics

	lines := strings.Split(raw, "\n")
	if len(lines) < 4 {
		return cpu, mem, disk, loadAvg, uptime, fmt.Errorf("unexpected remote metrics output")
	}

	// Parse two CPU samples (first two "cpu " lines)
	var cpuLines []string
	var section string
	var memLines []string
	var loadLine, uptimeLine, diskLine string

	for _, line := range lines {
		switch {
		case line == "---LOADAVG---":
			section = "loadavg"
			continue
		case line == "---UPTIME---":
			section = "uptime"
			continue
		case line == "---DISK---":
			section = "disk"
			continue
		}

		switch section {
		case "loadavg":
			if strings.TrimSpace(line) != "" {
				loadLine = line
			}
		case "uptime":
			if strings.TrimSpace(line) != "" {
				uptimeLine = line
			}
		case "disk":
			if strings.TrimSpace(line) != "" {
				diskLine = line
			}
		default:
			if strings.HasPrefix(line, "cpu ") {
				cpuLines = append(cpuLines, line)
			} else if strings.Contains(line, ":") {
				memLines = append(memLines, line)
			}
		}
	}

	// CPU
	if len(cpuLines) >= 2 {
		idle1, total1 := parseCPULine(cpuLines[0])
		idle2, total2 := parseCPULine(cpuLines[1])
		totalDelta := float64(total2 - total1)
		if totalDelta > 0 {
			cpu.UsagePercent = (1.0 - float64(idle2-idle1)/totalDelta) * 100.0
		}
	}

	// Memory
	memValues := make(map[string]uint64)
	for _, line := range memLines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		if key != "MemTotal" && key != "MemAvailable" {
			continue
		}
		valStr := strings.TrimSpace(parts[1])
		valStr = strings.TrimSuffix(valStr, " kB")
		valStr = strings.TrimSpace(valStr)
		if v, err := strconv.ParseUint(valStr, 10, 64); err == nil {
			memValues[key] = v * 1024
		}
	}
	mem.TotalBytes = memValues["MemTotal"]
	mem.AvailableBytes = memValues["MemAvailable"]
	mem.UsedBytes = mem.TotalBytes - mem.AvailableBytes
	if mem.TotalBytes > 0 {
		mem.UsagePercent = float64(mem.UsedBytes) / float64(mem.TotalBytes) * 100.0
	}

	// Load average
	if loadLine != "" {
		fields := strings.Fields(loadLine)
		if len(fields) >= 3 {
			loadAvg.Load1, _ = strconv.ParseFloat(fields[0], 64)
			loadAvg.Load5, _ = strconv.ParseFloat(fields[1], 64)
			loadAvg.Load15, _ = strconv.ParseFloat(fields[2], 64)
		}
	}

	// Uptime
	if uptimeLine != "" {
		fields := strings.Fields(uptimeLine)
		if len(fields) >= 1 {
			uptime.UptimeSeconds, _ = strconv.ParseFloat(fields[0], 64)
		}
	}

	// Disk (df -B1 output: filesystem 1B-blocks used available use% mountpoint)
	if diskLine != "" {
		fields := strings.Fields(diskLine)
		if len(fields) >= 4 {
			disk.TotalBytes, _ = strconv.ParseUint(fields[1], 10, 64)
			disk.UsedBytes, _ = strconv.ParseUint(fields[2], 10, 64)
			disk.AvailableBytes, _ = strconv.ParseUint(fields[3], 10, 64)
			if disk.TotalBytes > 0 {
				disk.UsagePercent = float64(disk.UsedBytes) / float64(disk.TotalBytes) * 100.0
			}
		}
	}

	return cpu, mem, disk, loadAvg, uptime, nil
}

func parseCPULine(line string) (idle, total uint64) {
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return 0, 0
	}
	var values []uint64
	for _, f := range fields[1:] {
		v, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			continue
		}
		values = append(values, v)
	}
	for _, v := range values {
		total += v
	}
	if len(values) > 3 {
		idle = values[3]
	}
	return
}

