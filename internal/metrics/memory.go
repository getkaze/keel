package metrics

import (
	"github.com/getkaze/keel/internal/model"
	"github.com/shirou/gopsutil/v4/mem"
)

// ReadMemory reads RAM usage via gopsutil.
func ReadMemory() (model.MemoryMetrics, error) {
	v, err := mem.VirtualMemory()
	if err != nil {
		return model.MemoryMetrics{}, err
	}
	return model.MemoryMetrics{
		TotalBytes:     v.Total,
		UsedBytes:      v.Used,
		AvailableBytes: v.Available,
		UsagePercent:   v.UsedPercent,
	}, nil
}
