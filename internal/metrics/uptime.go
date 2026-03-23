package metrics

import (
	"github.com/getkaze/keel/internal/model"
	"github.com/shirou/gopsutil/v4/host"
)

// ReadUptime reads host uptime via gopsutil.
func ReadUptime() (model.UptimeMetrics, error) {
	secs, err := host.Uptime()
	if err != nil {
		return model.UptimeMetrics{}, err
	}
	return model.UptimeMetrics{UptimeSeconds: float64(secs)}, nil
}
