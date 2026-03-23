package metrics

import (
	"github.com/getkaze/keel/internal/model"
	"github.com/shirou/gopsutil/v4/disk"
)

// ReadDisk reads disk usage for the root partition via gopsutil.
func ReadDisk() (model.DiskMetrics, error) {
	u, err := disk.Usage("/")
	if err != nil {
		return model.DiskMetrics{}, err
	}
	return model.DiskMetrics{
		TotalBytes:     u.Total,
		UsedBytes:      u.Used,
		AvailableBytes: u.Free,
		UsagePercent:   u.UsedPercent,
	}, nil
}
