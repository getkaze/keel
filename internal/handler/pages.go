package handler

import (
	"bytes"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"sync"

	"github.com/getkaze/keel/internal/config"
	"github.com/getkaze/keel/internal/docker"
	"github.com/getkaze/keel/internal/metrics"
	"github.com/getkaze/keel/internal/model"
)

// PageDeps bundles dependencies for page handlers.
type PageDeps struct {
	Services       *config.ServiceStore
	Seeders        *config.SeederStore
	Docker         *docker.StatusPoller
	Stats          *metrics.StatsPoller
	Tmpl           *template.Template
	Version        string
	Remote         *metrics.RemoteCollector // nil for local targets
	SeederExecutor *docker.SeederExecutor
}

// renderTemplate executes a template into a buffer first. Only on success
// is the content written to w. On failure, a clean HTTP 500 is returned
// instead of partial HTML.
func (d *PageDeps) renderTemplate(w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer
	if err := d.Tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}

// RegisterPageRoutes wires up page rendering routes.
func RegisterPageRoutes(mux *http.ServeMux, deps *PageDeps) {
	mux.HandleFunc("GET /partials/overview", deps.overviewPartial)
	mux.HandleFunc("GET /partials/logs", deps.logsPartial)
	mux.HandleFunc("GET /partials/metrics", deps.metricsPartial)
	mux.HandleFunc("GET /partials/metrics-mini", deps.metricsMiniPartial)
	mux.HandleFunc("GET /partials/service/{name}", deps.serviceDetailPartial)
	mux.HandleFunc("GET /partials/service-new", deps.serviceNewPartial)
	mux.HandleFunc("GET /partials/settings", deps.settingsPartial)
	mux.HandleFunc("GET /partials/seeders", deps.seedersPartial)
}

// ServiceGroupView holds a group label and its service views for template rendering.
type ServiceGroupView struct {
	Name     string
	Services []ServiceView
}

func (d *PageDeps) overviewPartial(w http.ResponseWriter, r *http.Request) {
	services, err := d.Services.List()
	if err != nil {
		log.Printf("overview: %v (showing empty state)", err)
		services = nil
	}

	containers, _ := d.Docker.ListContainers(r.Context())
	var views []ServiceView
	for _, svc := range services {
		ci := docker.MatchServiceToContainer(svc.Name, svc.Hostname, containers)
		views = append(views, ServiceView{
			Service:   svc,
			Status:    docker.ContainerToStatus(ci),
			Container: ci,
		})
	}

	// Group services by their Group field (preserve insertion order).
	var groupOrder []string
	groupMap := map[string][]ServiceView{}
	for _, v := range views {
		g := v.Group
		if g == "" {
			g = "other"
		}
		if _, exists := groupMap[g]; !exists {
			groupOrder = append(groupOrder, g)
		}
		groupMap[g] = append(groupMap[g], v)
	}

	var groups []ServiceGroupView
	for _, g := range groupOrder {
		groups = append(groups, ServiceGroupView{Name: g, Services: groupMap[g]})
	}

	var hasRunning, hasStopped bool
	for _, v := range views {
		if v.Status == "running" {
			hasRunning = true
		} else {
			hasStopped = true
		}
	}

	d.renderTemplate(w, "service-grid", map[string]any{
		"Services":   views,
		"Groups":     groups,
		"HasRunning": hasRunning,
		"HasStopped": hasStopped,
	})
}

func (d *PageDeps) logsPartial(w http.ResponseWriter, r *http.Request) {
	services, err := d.Services.List()
	if err != nil {
		log.Printf("logs partial error: %v (showing empty state)", err)
		services = nil
	}

	svcMap := make(map[string]any)
	for _, svc := range services {
		svcMap[svc.Name] = map[string]any{"logs": svc.Logs}
	}
	servicesJSON, _ := json.Marshal(svcMap)

	d.renderTemplate(w, "log-viewer", map[string]any{
		"Services":     services,
		"ServicesJSON": template.JS(servicesJSON),
	})
}

// ServiceDetailView carries all data needed to render a container detail page.
type ServiceDetailView struct {
	model.Service
	Status      model.ContainerStatus
	Container   *docker.ContainerInfo
	RawConfig   string
	HasHostLogs bool // true when at least one log source has a host_path
}

func (d *PageDeps) serviceDetailPartial(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := d.Services.Get(name)
	if err != nil || svc == nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	containers, _ := d.Docker.ListContainers(r.Context())
	ci := docker.MatchServiceToContainer(name, svc.Hostname, containers)

	raw, _ := d.Services.GetRaw(name)

	hasHostLogs := false
	for _, ls := range svc.Logs {
		if ls.HostPath != "" {
			hasHostLogs = true
			break
		}
	}

	view := ServiceDetailView{
		Service:     *svc,
		Status:      docker.ContainerToStatus(ci),
		Container:   ci,
		RawConfig:   string(raw),
		HasHostLogs: hasHostLogs,
	}

	d.renderTemplate(w, "service-detail", view)
}

func (d *PageDeps) serviceNewPartial(w http.ResponseWriter, r *http.Request) {
	cfg, _ := d.Services.GlobalConfig()
	network := "keel-net"
	if cfg != nil && cfg.Network != "" {
		network = cfg.Network
	}

	d.renderTemplate(w, "service-new", map[string]any{
		"DefaultNetwork": network,
	})
}

func (d *PageDeps) metricsPartial(w http.ResponseWriter, r *http.Request) {
	var (
		cpu        model.CPUMetrics
		mem        model.MemoryMetrics
		disk       model.DiskMetrics
		loadAvg    model.LoadAvgMetrics
		uptime     model.UptimeMetrics
		containers []model.ContainerStats
		wg         sync.WaitGroup
	)

	if d.Remote != nil {
		wg.Add(2)
		go func() {
			defer wg.Done()
			var err error
			cpu, mem, disk, loadAvg, uptime, err = d.Remote.ReadAll()
			if err != nil {
				log.Printf("remote metrics error: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			containers, _ = d.Stats.ReadStats(r.Context())
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
			containers, _ = d.Stats.ReadStats(r.Context())
		}()
	}
	wg.Wait()

	data := model.SystemMetrics{
		CPU: cpu, Memory: mem, Disk: disk,
		LoadAvg: loadAvg, Uptime: uptime, Containers: containers,
	}
	d.renderTemplate(w, "metrics-panel", data)
}

func (d *PageDeps) metricsMiniPartial(w http.ResponseWriter, r *http.Request) {
	var (
		cpu  model.CPUMetrics
		mem  model.MemoryMetrics
		disk model.DiskMetrics
	)

	if d.Remote != nil {
		cpu, mem, disk, _, _, _ = d.Remote.ReadAll()
	} else {
		cpu, _ = metrics.ReadCPU()
		mem, _ = metrics.ReadMemory()
		disk, _ = metrics.ReadDisk()
	}

	data := model.SystemMetrics{CPU: cpu, Memory: mem, Disk: disk}
	d.renderTemplate(w, "metrics-mini", data)
}

// SeederGroupView groups seeders by their target service.
type SeederGroupView struct {
	Target       string
	TargetStatus model.ContainerStatus
	Seeders      []SeederView
}

func (d *PageDeps) seedersPartial(w http.ResponseWriter, r *http.Request) {
	var groups []SeederGroupView
	if d.Seeders != nil {
		list, err := d.Seeders.List()
		if err != nil {
			log.Printf("seeders partial error: %v", err)
		}
		containers, _ := d.Docker.ListContainers(r.Context())

		// Build service hostname map for seeder target resolution
		svcHostnames := map[string]string{}
		if d.Services != nil {
			if svcs, err2 := d.Services.List(); err2 == nil {
				for _, s := range svcs {
					svcHostnames[s.Name] = s.Hostname
				}
			}
		}

		// Group seeders by target
		groupMap := make(map[string]*SeederGroupView)
		var order []string
		for _, sd := range list {
			ci := docker.MatchServiceToContainer(sd.Target, svcHostnames[sd.Target], containers)
			sv := SeederView{
				Seeder:       sd,
				TargetStatus: docker.ContainerToStatus(ci),
				CommandCount: len(sd.Commands),
			}
			if d.SeederExecutor != nil {
				sv.LastStatus = d.SeederExecutor.GetLastStatus(sd.Name)
				if t := d.SeederExecutor.GetLastRanAt(sd.Name); !t.IsZero() {
					sv.LastRanAt = t.Format("2006-01-02 15:04")
				}
			}
			g, ok := groupMap[sd.Target]
			if !ok {
				g = &SeederGroupView{
					Target:       sd.Target,
					TargetStatus: sv.TargetStatus,
				}
				groupMap[sd.Target] = g
				order = append(order, sd.Target)
			}
			g.Seeders = append(g.Seeders, sv)
		}
		for _, t := range order {
			groups = append(groups, *groupMap[t])
		}
	}

	d.renderTemplate(w, "seeders", map[string]any{
		"Groups": groups,
	})
}

func (d *PageDeps) settingsPartial(w http.ResponseWriter, r *http.Request) {
	d.renderTemplate(w, "settings", map[string]any{
		"Version": d.Version,
	})
}
