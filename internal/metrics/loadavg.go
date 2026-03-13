package metrics

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/getkaze/keel/internal/model"
)

// ReadLoadAvg reads load averages from /proc/loadavg.
func ReadLoadAvg() (model.LoadAvgMetrics, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return model.LoadAvgMetrics{}, fmt.Errorf("read /proc/loadavg: %w", err)
	}

	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return model.LoadAvgMetrics{}, fmt.Errorf("unexpected /proc/loadavg format")
	}

	var vals [3]float64
	for i := 0; i < 3; i++ {
		vals[i], err = strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return model.LoadAvgMetrics{}, fmt.Errorf("parse /proc/loadavg: %w", err)
		}
	}

	return model.LoadAvgMetrics{Load1: vals[0], Load5: vals[1], Load15: vals[2]}, nil
}
