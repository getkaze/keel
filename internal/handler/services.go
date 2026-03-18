package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/getkaze/keel/internal/config"
	"github.com/getkaze/keel/internal/docker"
	"github.com/getkaze/keel/internal/metrics"
	"github.com/getkaze/keel/internal/model"
)

// ServiceView combines a service definition with its runtime status.
type ServiceView struct {
	model.Service
	Status    model.ContainerStatus  `json:"status"`
	Container *docker.ContainerInfo  `json:"container,omitempty"`
	Stats     *model.ContainerStats  `json:"stats,omitempty"`
}

// ServiceDeps bundles dependencies for service handlers.
type ServiceDeps struct {
	Services   *config.ServiceStore
	Docker     *docker.StatusPoller
	Stats      *metrics.StatsPoller
	Executor   *docker.Executor
	SeederExec *docker.SeederExecutor
	Mutex      *OpMutex
	AppCtx     context.Context
	Tmpl       *template.Template
}

// RegisterServiceRoutes wires up all service-related routes.
func RegisterServiceRoutes(mux *http.ServeMux, deps *ServiceDeps) {
	mux.HandleFunc("GET /api/services", deps.listServices)
	mux.HandleFunc("GET /api/services/{name}", deps.getService)
	mux.HandleFunc("POST /api/services", deps.createService)
	mux.HandleFunc("DELETE /api/services/{name}", deps.deleteService)
	mux.HandleFunc("POST /api/services/{name}/start", deps.startService)
	mux.HandleFunc("POST /api/services/{name}/stop", deps.stopService)
	mux.HandleFunc("POST /api/services/{name}/restart", deps.restartService)
	mux.HandleFunc("POST /api/services/{name}/update", deps.updateService)
	mux.HandleFunc("POST /api/services/start-all", deps.startAll)
	mux.HandleFunc("POST /api/services/stop-all", deps.stopAll)
	mux.HandleFunc("GET /api/services/{name}/metrics", deps.getServiceMetrics)
	mux.HandleFunc("GET /api/services/{name}/config", deps.getServiceConfig)
	mux.HandleFunc("PUT /api/services/{name}/config", deps.saveServiceConfig)
}

func (d *ServiceDeps) listServices(w http.ResponseWriter, r *http.Request) {
	views, err := d.buildServiceViews(r.Context())
	if err != nil {
		log.Printf("list services error: %v", err)
		http.Error(w, "failed to list services", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(views)
}

func (d *ServiceDeps) getService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	svc, err := d.Services.Get(name)
	if err != nil {
		http.Error(w, "failed to read service", http.StatusInternalServerError)
		return
	}
	if svc == nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	containers, _ := d.Docker.ListContainers(r.Context())
	ci := docker.MatchServiceToContainer(name, svc.Hostname, containers)

	view := ServiceView{
		Service:   *svc,
		Status:    docker.ContainerToStatus(ci),
		Container: ci,
	}

	// Return HTML partial for HTMX requests, JSON otherwise
	if r.Header.Get("HX-Request") == "true" && d.Tmpl != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := d.Tmpl.ExecuteTemplate(w, "service-card", view); err != nil {
			log.Printf("template error: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(view)
}

func (d *ServiceDeps) createService(w http.ResponseWriter, r *http.Request) {
	release := d.Mutex.TryAcquire(w, "create")
	if release == nil {
		return
	}
	defer release()

	// Limit body to 64 KB to prevent abuse.
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	image := strings.TrimSpace(r.FormValue("image"))
	hostname := strings.TrimSpace(r.FormValue("hostname"))
	network := strings.TrimSpace(r.FormValue("network"))
	command := strings.TrimSpace(r.FormValue("command"))
	registry := strings.TrimSpace(r.FormValue("registry"))
	group := strings.TrimSpace(r.FormValue("group"))

	if name == "" || image == "" {
		http.Error(w, "name and image are required", http.StatusBadRequest)
		return
	}
	if hostname == "" {
		hostname = "keel-" + name
	}
	if network == "" {
		network = "keel-net"
	}

	extPort, _ := strconv.Atoi(r.FormValue("port_external"))
	intPort, _ := strconv.Atoi(r.FormValue("port_internal"))

	// Parse environment: KEY=VALUE lines
	env := make(map[string]string)
	for _, line := range strings.Split(r.FormValue("environment"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			env[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	// Parse volumes: one per line
	var volumes []string
	for _, line := range strings.Split(r.FormValue("volumes"), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			volumes = append(volumes, line)
		}
	}

	svc := model.Service{
		Name:        name,
		Group:       group,
		Hostname:    hostname,
		Image:       image,
		Registry:    registry,
		Network:     network,
		Ports:       model.PortConfig{External: extPort, Internal: intPort},
		Environment: env,
		Volumes:     volumes,
		Command:     command,
	}

	// Check for name conflict
	existing, _ := d.Services.Get(name)
	if existing != nil {
		http.Error(w, "service already exists: "+name, http.StatusConflict)
		return
	}

	if err := d.Services.Save(svc); err != nil {
		http.Error(w, "failed to save service: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Boot the container (streaming SSE)
	d.streamCommand(w, r, "start", name)
}

func (d *ServiceDeps) deleteService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := d.Services.Get(name)
	if err != nil || svc == nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	// Stop + remove container (best effort)
	ctx := r.Context()
	_ = d.Executor.RemoveContainer(ctx, svc.Hostname)

	// Delete config file
	if err := d.Services.Delete(name); err != nil {
		http.Error(w, "failed to delete service: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// For HTMX: trigger redirect to overview
	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusOK)
}

func (d *ServiceDeps) startService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := d.validateServiceName(name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	release := d.Mutex.TryAcquire(w, "start")
	if release == nil {
		return
	}
	defer release()
	d.streamCommand(w, r, "start", name)
}

func (d *ServiceDeps) stopService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := d.validateServiceName(name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	release := d.Mutex.TryAcquire(w, "stop")
	if release == nil {
		return
	}
	defer release()
	d.streamCommand(w, r, "stop", name)
}

func (d *ServiceDeps) restartService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := d.validateServiceName(name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	release := d.Mutex.TryAcquire(w, "restart")
	if release == nil {
		return
	}
	defer release()
	d.streamCommand(w, r, "restart", name)
}

func (d *ServiceDeps) startAll(w http.ResponseWriter, r *http.Request) {
	release := d.Mutex.TryAcquire(w, "start-all")
	if release == nil {
		return
	}
	defer release()

	services, err := d.Services.List()
	if err != nil {
		http.Error(w, "failed to list services", http.StatusInternalServerError)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()

	// Partition services into infra and rest
	infra, rest := partitionByGroup(services)

	// 1. Start infra services first
	for _, svc := range infra {
		lines, errc := d.Executor.Stream(ctx, "start", svc.Name)
		for line := range lines {
			fmt.Fprintf(w, "data: %s\n\n", line)
			flusher.Flush()
		}
		if err := <-errc; err != nil {
			fmt.Fprintf(w, "data: [%s] error: %s\n\n", svc.Name, err.Error())
			flusher.Flush()
		}
	}

	// 2. Run seeders (if any exist and executor is available)
	if d.SeederExec != nil && d.SeederExec.HasSeeders() {
		fmt.Fprintf(w, "data: --- running seeders ---\n\n")
		flusher.Flush()

		out := make(chan string, 64)
		errc := make(chan error, 1)
		go func() {
			defer close(out)
			defer close(errc)
			errc <- d.SeederExec.RunAll(ctx, out)
		}()

		for line := range out {
			fmt.Fprintf(w, "data: %s\n\n", line)
			flusher.Flush()
		}

		if err := <-errc; err != nil {
			fmt.Fprintf(w, "event: app-error\ndata: seeder failed: %s\n\n", err.Error())
			flusher.Flush()
			d.Docker.Invalidate()
			return // STOP — don't start app services
		}
	}

	// 3. Start remaining services
	for _, svc := range rest {
		lines, errc := d.Executor.Stream(ctx, "start", svc.Name)
		for line := range lines {
			fmt.Fprintf(w, "data: %s\n\n", line)
			flusher.Flush()
		}
		if err := <-errc; err != nil {
			fmt.Fprintf(w, "data: [%s] error: %s\n\n", svc.Name, err.Error())
			flusher.Flush()
		}
	}

	fmt.Fprintf(w, "event: done\ndata: all services started\n\n")
	flusher.Flush()
	d.Docker.Invalidate()
}

func (d *ServiceDeps) stopAll(w http.ResponseWriter, r *http.Request) {
	release := d.Mutex.TryAcquire(w, "stop-all")
	if release == nil {
		return
	}
	defer release()

	services, err := d.Services.List()
	if err != nil {
		http.Error(w, "failed to list services", http.StatusInternalServerError)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()

	// Stop in reverse order: app first, then infra
	infra, rest := partitionByGroup(services)

	for _, svc := range rest {
		lines, errc := d.Executor.Stream(ctx, "stop", svc.Name)
		for line := range lines {
			fmt.Fprintf(w, "data: %s\n\n", line)
			flusher.Flush()
		}
		if err := <-errc; err != nil {
			fmt.Fprintf(w, "data: [%s] error: %s\n\n", svc.Name, err.Error())
			flusher.Flush()
		}
	}

	for _, svc := range infra {
		lines, errc := d.Executor.Stream(ctx, "stop", svc.Name)
		for line := range lines {
			fmt.Fprintf(w, "data: %s\n\n", line)
			flusher.Flush()
		}
		if err := <-errc; err != nil {
			fmt.Fprintf(w, "data: [%s] error: %s\n\n", svc.Name, err.Error())
			flusher.Flush()
		}
	}

	fmt.Fprintf(w, "event: done\ndata: all services stopped\n\n")
	flusher.Flush()
	d.Docker.Invalidate()
}

// partitionByGroup splits services into infra (group == "infra") and rest,
// each sorted by start_order (0 = last).
func partitionByGroup(services []model.Service) (infra, rest []model.Service) {
	for _, svc := range services {
		if svc.Group == "infra" {
			infra = append(infra, svc)
		} else {
			rest = append(rest, svc)
		}
	}
	sortByStartOrder(infra)
	sortByStartOrder(rest)
	return
}

func sortByStartOrder(s []model.Service) {
	sort.SliceStable(s, func(i, j int) bool {
		oi, oj := s[i].StartOrder, s[j].StartOrder
		// 0 means unset — push to end
		if oi == 0 {
			return false
		}
		if oj == 0 {
			return true
		}
		return oi < oj
	})
}

func (d *ServiceDeps) updateService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := d.validateServiceName(name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	release := d.Mutex.TryAcquire(w, "update")
	if release == nil {
		return
	}
	defer release()
	d.streamCommand(w, r, "update", name)
}

func (d *ServiceDeps) getServiceMetrics(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	svc, err := d.Services.Get(name)
	if err != nil || svc == nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	allStats, err := d.Stats.ReadStats(r.Context())
	if err != nil {
		http.Error(w, "failed to read docker stats", http.StatusInternalServerError)
		return
	}

	// Match by hostname (docker container name)
	hostname := svc.Hostname
	for _, cs := range allStats {
		if cs.Name == hostname || cs.Name == "keel-"+name || cs.Name == name {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if d.Tmpl != nil {
				d.Tmpl.ExecuteTemplate(w, "service-metrics", cs)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cs)
			return
		}
	}

	// Container not found in docker stats — return empty metrics
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if d.Tmpl != nil {
		d.Tmpl.ExecuteTemplate(w, "service-metrics", model.ContainerStats{})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(model.ContainerStats{})
}

func (d *ServiceDeps) getServiceConfig(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	data, err := d.Services.GetRaw(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (d *ServiceDeps) saveServiceConfig(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	// Limit body to 1 MB to prevent abuse.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	// Accept raw JSON body OR form field "config"
	var data []byte
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		var err error
		data, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
	} else {
		r.ParseForm()
		data = []byte(r.FormValue("config"))
	}

	if len(data) == 0 {
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	if !json.Valid(data) {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if err := d.Services.SaveRaw(name, data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// streamCommand executes a Docker operation and streams output as SSE.
func (d *ServiceDeps) streamCommand(w http.ResponseWriter, r *http.Request, command string, args ...string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()
	lines, errc := d.Executor.Stream(ctx, command, args...)

	for line := range lines {
		fmt.Fprintf(w, "data: %s\n\n", line)
		flusher.Flush()
	}

	if err := <-errc; err != nil {
		fmt.Fprintf(w, "event: app-error\ndata: %s\n\n", err.Error())
	} else {
		fmt.Fprintf(w, "event: done\ndata: %s %s completed\n\n", command, args)
	}
	flusher.Flush()

	d.Docker.Invalidate()
}

func (d *ServiceDeps) validateServiceName(name string) error {
	svc, err := d.Services.Get(name)
	if err != nil {
		return fmt.Errorf("failed to look up service: %w", err)
	}
	if svc == nil {
		return fmt.Errorf("unknown service: %s", name)
	}
	return nil
}

func (d *ServiceDeps) buildServiceViews(ctx context.Context) ([]ServiceView, error) {
	services, err := d.Services.List()
	if err != nil {
		return nil, err
	}

	containers, _ := d.Docker.ListContainers(ctx)
	allStats, _ := d.Stats.ReadStats(ctx)

	// Index stats by container name for fast lookup
	statsMap := make(map[string]*model.ContainerStats, len(allStats))
	for i := range allStats {
		statsMap[allStats[i].Name] = &allStats[i]
	}

	var views []ServiceView
	for _, svc := range services {
		ci := docker.MatchServiceToContainer(svc.Name, svc.Hostname, containers)
		view := ServiceView{
			Service:   svc,
			Status:    docker.ContainerToStatus(ci),
			Container: ci,
		}
		// Attach stats for running containers
		if view.Status == model.StatusRunning {
			if s := statsMap[svc.Hostname]; s != nil {
				view.Stats = s
			} else if s := statsMap["keel-"+svc.Name]; s != nil {
				view.Stats = s
			} else if s := statsMap[svc.Name]; s != nil {
				view.Stats = s
			}
		}
		views = append(views, view)
	}
	return views, nil
}
