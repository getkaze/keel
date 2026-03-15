package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/getkaze/keel/internal/config"
	"github.com/getkaze/keel/internal/docker"
	"github.com/getkaze/keel/internal/model"
)

// SeederView combines a seeder definition with its target container status.
type SeederView struct {
	model.Seeder
	TargetStatus model.ContainerStatus `json:"target_status"`
	CommandCount int                   `json:"command_count"`
	LastStatus   string                `json:"last_status,omitempty"`
	LastRanAt    string                `json:"last_ran_at,omitempty"`
}

// SeederDeps bundles dependencies for seeder handlers.
type SeederDeps struct {
	Seeders  *config.SeederStore
	Services *config.ServiceStore
	Executor *docker.SeederExecutor
	Docker   *docker.StatusPoller
	Mutex    *OpMutex
}

// RegisterSeederRoutes wires up all seeder-related routes.
func RegisterSeederRoutes(mux *http.ServeMux, deps *SeederDeps) {
	mux.HandleFunc("GET /api/seeders", deps.listSeeders)
	mux.HandleFunc("GET /api/seeders/run", deps.runAll)
	mux.HandleFunc("GET /api/seeders/run/{name}", deps.runOne)
	mux.HandleFunc("GET /api/seeders/config/{name}", deps.getSeederConfig)
}

func (d *SeederDeps) listSeeders(w http.ResponseWriter, r *http.Request) {
	// Reload state from disk to pick up changes made by CLI.
	d.Executor.ReloadState()

	seeders, err := d.Seeders.List()
	if err != nil {
		http.Error(w, "failed to list seeders", http.StatusInternalServerError)
		return
	}
	if seeders == nil {
		seeders = []model.Seeder{}
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

	views := make([]SeederView, 0, len(seeders))
	for _, sd := range seeders {
		ci := docker.MatchServiceToContainer(sd.Target, svcHostnames[sd.Target], containers)
		lastRanAt := ""
		if t := d.Executor.GetLastRanAt(sd.Name); !t.IsZero() {
			lastRanAt = t.Format("2006-01-02 15:04")
		}
		views = append(views, SeederView{
			Seeder:       sd,
			TargetStatus: docker.ContainerToStatus(ci),
			CommandCount: len(sd.Commands),
			LastStatus:   d.Executor.GetLastStatus(sd.Name),
			LastRanAt:    lastRanAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(views)
}

func (d *SeederDeps) runAll(w http.ResponseWriter, r *http.Request) {
	release := d.Mutex.TryAcquire(w, "seeders-run")
	if release == nil {
		return
	}
	defer release()

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()
	out := make(chan string, 64)
	errc := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errc)
		errc <- d.Executor.RunAll(ctx, out)
	}()

	for line := range out {
		fmt.Fprintf(w, "data: %s\n\n", line)
		flusher.Flush()
	}

	if err := <-errc; err != nil {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
	} else {
		fmt.Fprintf(w, "event: done\ndata: all seeders completed\n\n")
	}
	flusher.Flush()
}

func (d *SeederDeps) runOne(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	seeder, err := d.Seeders.Get(name)
	if err != nil {
		http.Error(w, "failed to read seeder", http.StatusInternalServerError)
		return
	}
	if seeder == nil {
		http.Error(w, "seeder not found", http.StatusNotFound)
		return
	}

	release := d.Mutex.TryAcquire(w, "seeders-run")
	if release == nil {
		return
	}
	defer release()

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()
	out := make(chan string, 64)
	errc := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errc)
		errc <- d.Executor.RunOne(ctx, out, seeder)
	}()

	for line := range out {
		fmt.Fprintf(w, "data: %s\n\n", line)
		flusher.Flush()
	}

	if err := <-errc; err != nil {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
	} else {
		fmt.Fprintf(w, "event: done\ndata: seeder %s completed\n\n", name)
	}
	flusher.Flush()
}

func (d *SeederDeps) getSeederConfig(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	data, err := d.Seeders.GetRaw(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
