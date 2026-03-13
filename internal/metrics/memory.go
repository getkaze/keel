package metrics

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/getkaze/keel/internal/model"
)

// ReadMemory reads RAM usage from /proc/meminfo.
func ReadMemory() (model.MemoryMetrics, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return model.MemoryMetrics{}, fmt.Errorf("open /proc/meminfo: %w", err)
	}
	defer f.Close()

	values := make(map[string]uint64)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
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

		v, err := strconv.ParseUint(valStr, 10, 64)
		if err != nil {
			continue
		}
		values[key] = v * 1024 // convert kB to bytes
	}

	total := values["MemTotal"]
	available := values["MemAvailable"]
	used := total - available

	var percent float64
	if total > 0 {
		percent = float64(used) / float64(total) * 100.0
	}

	return model.MemoryMetrics{
		TotalBytes:     total,
		UsedBytes:      used,
		AvailableBytes: available,
		UsagePercent:   percent,
	}, nil
}
