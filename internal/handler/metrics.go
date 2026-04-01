package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/getkaze/keel/internal/metrics"
	"github.com/getkaze/keel/internal/model"
)

// RemoteRef is a shared atomic reference to a RemoteCollector.
// It allows hot-reloading when the active target changes.
type RemoteRef = atomic.Pointer[metrics.RemoteCollector]

// MetricsHandler handles GET /api/metrics.
type MetricsHandler struct {
	Remote *RemoteRef // shared atomic reference; value is nil for local targets
	Stats  *metrics.StatsPoller
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

	if remote := h.Remote.Load(); remote != nil {
		wg.Add(2)
		go func() {
			defer wg.Done()
			var err error
			cpu, mem, disk, loadAvg, uptime, err = remote.ReadAll()
			if err != nil {
				log.Printf("remote metrics error: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			containers, _ = h.Stats.ReadStats(r.Context())
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
			containers, _ = h.Stats.ReadStats(r.Context())
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
