package metrics

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/getkaze/keel/internal/model"
)

// ReadCPU reads CPU usage from /proc/stat with two samples 1 second apart.
func ReadCPU() (model.CPUMetrics, error) {
	idle1, total1, err := readCPUSample()
	if err != nil {
		return model.CPUMetrics{}, err
	}

	time.Sleep(1 * time.Second)

	idle2, total2, err := readCPUSample()
	if err != nil {
		return model.CPUMetrics{}, err
	}

	idleDelta := float64(idle2 - idle1)
	totalDelta := float64(total2 - total1)

	if totalDelta == 0 {
		return model.CPUMetrics{UsagePercent: 0}, nil
	}

	usage := (1.0 - idleDelta/totalDelta) * 100.0
	return model.CPUMetrics{UsagePercent: usage}, nil
}

func readCPUSample() (idle, total uint64, err error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, fmt.Errorf("open /proc/stat: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 5 {
			return 0, 0, fmt.Errorf("unexpected /proc/stat format")
		}

		var values []uint64
		for _, field := range fields[1:] {
			v, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				return 0, 0, fmt.Errorf("parse /proc/stat: %w", err)
			}
			values = append(values, v)
		}

		for _, v := range values {
			total += v
		}
		// idle is the 4th field (index 3)
		if len(values) > 3 {
			idle = values[3]
		}
		return idle, total, nil
	}

	return 0, 0, fmt.Errorf("cpu line not found in /proc/stat")
}
