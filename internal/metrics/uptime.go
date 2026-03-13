package metrics

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/getkaze/keel/internal/model"
)

// ReadUptime reads host uptime from /proc/uptime.
func ReadUptime() (model.UptimeMetrics, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return model.UptimeMetrics{}, fmt.Errorf("read /proc/uptime: %w", err)
	}

	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return model.UptimeMetrics{}, fmt.Errorf("unexpected /proc/uptime format")
	}

	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return model.UptimeMetrics{}, fmt.Errorf("parse /proc/uptime: %w", err)
	}

	return model.UptimeMetrics{UptimeSeconds: secs}, nil
}
