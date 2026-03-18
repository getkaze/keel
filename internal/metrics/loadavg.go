package metrics

import (
	"github.com/getkaze/keel/internal/model"
	"github.com/shirou/gopsutil/v4/load"
)

// ReadLoadAvg reads load averages via gopsutil.
func ReadLoadAvg() (model.LoadAvgMetrics, error) {
	avg, err := load.Avg()
	if err != nil {
		return model.LoadAvgMetrics{}, err
	}
	return model.LoadAvgMetrics{
		Load1:  avg.Load1,
		Load5:  avg.Load5,
		Load15: avg.Load15,
	}, nil
}
