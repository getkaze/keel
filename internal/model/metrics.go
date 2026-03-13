package model

// SystemMetrics aggregates all system resource metrics.
type SystemMetrics struct {
	CPU        CPUMetrics       `json:"cpu"`
	Memory     MemoryMetrics    `json:"memory"`
	Disk       DiskMetrics      `json:"disk"`
	LoadAvg    LoadAvgMetrics   `json:"load_avg"`
	Uptime     UptimeMetrics    `json:"uptime"`
	Containers []ContainerStats `json:"containers"`
}

// CPUMetrics holds CPU usage information from /proc/stat.
type CPUMetrics struct {
	UsagePercent float64 `json:"usage_percent"`
}

// MemoryMetrics holds RAM usage information from /proc/meminfo.
type MemoryMetrics struct {
	TotalBytes     uint64  `json:"total_bytes"`
	UsedBytes      uint64  `json:"used_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	UsagePercent   float64 `json:"usage_percent"`
}

// DiskMetrics holds disk usage information from syscall.Statfs.
type DiskMetrics struct {
	TotalBytes     uint64  `json:"total_bytes"`
	UsedBytes      uint64  `json:"used_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	UsagePercent   float64 `json:"usage_percent"`
}

// LoadAvgMetrics holds load average from /proc/loadavg.
type LoadAvgMetrics struct {
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
}

// UptimeMetrics holds host uptime from /proc/uptime.
type UptimeMetrics struct {
	UptimeSeconds float64 `json:"uptime_seconds"`
}

// ContainerStats holds per-container resource usage from docker stats.
type ContainerStats struct {
	Name     string `json:"name"`
	CPUPerc  string `json:"cpu_perc"`
	MemUsage string `json:"mem_usage"`
	MemPerc  string `json:"mem_perc"`
	NetIO    string `json:"net_io"`
	BlockIO  string `json:"block_io"`
}
