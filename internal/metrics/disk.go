package metrics

import (
	"syscall"

	"github.com/getkaze/keel/internal/model"
)

// ReadDisk reads disk usage for the root partition via syscall.Statfs.
func ReadDisk() (model.DiskMetrics, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return model.DiskMetrics{}, err
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	available := stat.Bavail * uint64(stat.Bsize)
	used := total - free

	var percent float64
	if total > 0 {
		percent = float64(used) / float64(total) * 100.0
	}

	return model.DiskMetrics{
		TotalBytes:     total,
		UsedBytes:      used,
		AvailableBytes: available,
		UsagePercent:   percent,
	}, nil
}
