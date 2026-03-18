package metrics

import (
	"time"

	"github.com/getkaze/keel/internal/model"
	"github.com/shirou/gopsutil/v4/cpu"
)

// ReadCPU reads CPU usage with two samples 1 second apart via gopsutil.
func ReadCPU() (model.CPUMetrics, error) {
	percents, err := cpu.Percent(1*time.Second, false)
	if err != nil {
		return model.CPUMetrics{}, err
	}
	if len(percents) == 0 {
		return model.CPUMetrics{UsagePercent: 0}, nil
	}
	return model.CPUMetrics{UsagePercent: percents[0]}, nil
}
