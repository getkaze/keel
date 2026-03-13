package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/getkaze/keel/internal/metrics"
	"github.com/getkaze/keel/internal/model"
)

// MetricsHandler handles GET /api/metrics.
type MetricsHandler struct {
	Remote *metrics.RemoteCollector // nil for local targets
}

func (h *MetricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var (
		cpu        model.CPUMetrics
		mem        model.MemoryMetrics
		disk       model.DiskMetrics
		loadAvg    model.LoadAvgMetrics
		uptime     model.UptimeMetrics
		containers []model.ContainerStats
		wg         sync.WaitGroup
	)

	if h.Remote != nil {
		wg.Add(2)
		go func() {
			defer wg.Done()
			var err error
			cpu, mem, disk, loadAvg, uptime, err = h.Remote.ReadAll()
			if err != nil {
				log.Printf("remote metrics error: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			containers, _ = metrics.ReadDockerStats(r.Context())
		}()
	} else {
		wg.Add(4)
		go func() {
			defer wg.Done()
			var err error
			cpu, err = metrics.ReadCPU()
			if err != nil {
				log.Printf("cpu metrics error: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			var err error
			mem, err = metrics.ReadMemory()
			if err != nil {
				log.Printf("memory metrics error: %v", err)
			}
			disk, _ = metrics.ReadDisk()
		}()
		go func() {
			defer wg.Done()
			loadAvg, _ = metrics.ReadLoadAvg()
			uptime, _ = metrics.ReadUptime()
		}()
		go func() {
			defer wg.Done()
			containers, _ = metrics.ReadDockerStats(r.Context())
		}()
	}
	wg.Wait()

	result := model.SystemMetrics{
		CPU: cpu, Memory: mem, Disk: disk,
		LoadAvg: loadAvg, Uptime: uptime, Containers: containers,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
