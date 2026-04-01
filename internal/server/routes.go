package server

import (
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/getkaze/keel/internal/config"
	"github.com/getkaze/keel/internal/docker"
	"github.com/getkaze/keel/internal/handler"
	"github.com/getkaze/keel/internal/metrics"
)

// registerRoutes sets up all HTTP routes.
func registerRoutes(mux *http.ServeMux, cfg Config) {
	// Static assets
	var staticFS http.FileSystem
	if cfg.Dev {
		staticFS = http.Dir("web/static")
	} else {
		sub, err := fs.Sub(cfg.StaticFS, "web/static")
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create static sub-fs: %v\n", err)
			os.Exit(1)
		}
		staticFS = http.FS(sub)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(staticFS)))

	// Service worker at root scope for PWA
	mux.HandleFunc("GET /sw.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Service-Worker-Allowed", "/")
		f, err := staticFS.Open("sw.js")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		stat, _ := f.Stat()
		http.ServeContent(w, r, "sw.js", stat.ModTime(), f.(http.File))
	})

	// Shared dependencies
	services := config.NewServiceStore(cfg.KeelDir)
	poller := docker.NewStatusPoller()
	executor := docker.NewExecutor(cfg.KeelDir, services, cfg.Runner)
	opMutex := handler.NewOpMutex()

	// Parse templates
	tmpl := parseTemplates(cfg)

	// Page renderer.
	render := func(w http.ResponseWriter, page, serviceName string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		targetInfo, _ := config.ReadTarget(cfg.KeelDir)
		targetName := "local"
		if targetInfo != nil {
			targetName = targetInfo.Name
		}
		data := map[string]any{
			"Version":     cfg.Version,
			"Page":        page,
			"ServiceName": serviceName,
			"TargetName":  targetName,
		}
		if err := tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
			log.Printf("template error: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	}

	// SPA pages — render layout; HTMX loads the right partial
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		render(w, "/", "")
	})
	mux.HandleFunc("GET /logs", func(w http.ResponseWriter, r *http.Request) {
		render(w, "/logs", "")
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		render(w, "/metrics", "")
	})
	mux.HandleFunc("GET /settings", func(w http.ResponseWriter, r *http.Request) {
		render(w, "/settings", "")
	})
	mux.HandleFunc("GET /seeders", func(w http.ResponseWriter, r *http.Request) {
		render(w, "/seeders", "")
	})

	// Per-container pages
	mux.HandleFunc("GET /services/new", func(w http.ResponseWriter, r *http.Request) {
		render(w, "/services/new", "new")
	})
	mux.HandleFunc("GET /services/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		render(w, "/services/"+name, name)
	})

	// Shared stats poller (caches docker stats for 5s)
	statsPoller := metrics.NewStatsPoller()

	// Shared service deps
	svcDeps := &handler.ServiceDeps{
		Services: services,
		Docker:   poller,
		Stats:    statsPoller,
		Executor: executor,
		Mutex:    opMutex,
		AppCtx:   cfg.Ctx,
		Tmpl:     tmpl,
	}

	// API: services (CRUD + ops)
	handler.RegisterServiceRoutes(mux, svcDeps)

	// API: seeders
	seeders := config.NewSeederStore(cfg.KeelDir)
	seederExec := docker.NewSeederExecutor(cfg.KeelDir, services, seeders)
	handler.RegisterSeederRoutes(mux, &handler.SeederDeps{
		Seeders:  seeders,
		Services: services,
		Executor: seederExec,
		Docker:   poller,
		Mutex:    opMutex,
	})

	// Also inject seeder executor into service deps for start-all
	svcDeps.SeederExec = seederExec

	// API: logs
	logHandler := &handler.LogHandler{Services: services, Docker: poller}
	mux.Handle("GET /api/logs/{name}", logHandler)
	mux.HandleFunc("GET /api/logs/{name}/sources", logHandler.ServeLogSources)

	// WebSocket: terminal
	mux.Handle("GET /ws/terminal", &handler.TerminalHandler{})
	mux.Handle("GET /ws/terminal/exec/{name}", &handler.ExecTerminalHandler{Services: services})

	// API: tunnel status (SSE)
	mux.Handle("GET /api/tunnel/status", &handler.TunnelStatusHandler{Monitor: cfg.Tunnel})

	// API: target, health, metrics, version
	mux.Handle("GET /api/target", &handler.TargetHandler{KeelDir: cfg.KeelDir})
	mux.Handle("GET /api/health", &handler.HealthHandler{Services: services, Docker: poller})
	metricsHandler := &handler.MetricsHandler{Stats: statsPoller, Remote: cfg.RemoteRef}
	mux.Handle("GET /api/metrics", metricsHandler)
	mux.Handle("GET /api/version", &handler.VersionHandler{Version: cfg.Version})
	mux.Handle("POST /api/update", &handler.UpdateHandler{Version: cfg.Version})

	// Page partials (HTMX)
	handler.RegisterPageRoutes(mux, &handler.PageDeps{
		Services:       services,
		Seeders:        seeders,
		Docker:         poller,
		Stats:          statsPoller,
		Tmpl:           tmpl,
		Version:        cfg.Version,
		Remote:         cfg.RemoteRef,
		SeederExecutor: seederExec,
	})
}

var templateFuncs = template.FuncMap{
	"progressColor": func(percent float64) string {
		switch {
		case percent >= 85:
			return "bar-error"
		case percent >= 70:
			return "bar-warning"
		default:
			return "bar-success"
		}
	},
	"formatBytes": func(bytes uint64) string {
		const (
			gb = 1024 * 1024 * 1024
			mb = 1024 * 1024
		)
		switch {
		case bytes >= gb:
			return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
		case bytes >= mb:
			return fmt.Sprintf("%.0f MB", float64(bytes)/float64(mb))
		default:
			return fmt.Sprintf("%d B", bytes)
		}
	},
	"deref": func(b *bool) bool {
		if b == nil {
			return false
		}
		return *b
	},
	"hasPrefix": strings.HasPrefix,
	"formatUptime": func(seconds float64) string {
		d := int(seconds) / 86400
		h := (int(seconds) % 86400) / 3600
		m := (int(seconds) % 3600) / 60
		if d > 0 {
			return fmt.Sprintf("%dd %dh %dm", d, h, m)
		}
		return fmt.Sprintf("%dh %dm", h, m)
	},
}

func parseTemplates(cfg Config) *template.Template {
	if cfg.Dev {
		tmpl := template.Must(template.New("").Funcs(templateFuncs).ParseGlob("web/templates/*.html"))
		if matches, _ := fs.Glob(os.DirFS("."), "web/templates/partials/*.html"); len(matches) > 0 {
			template.Must(tmpl.ParseGlob("web/templates/partials/*.html"))
		}
		return tmpl
	}

	tmpl := template.Must(template.New("").Funcs(templateFuncs).ParseFS(cfg.StaticFS, "web/templates/*.html"))
	if matches, _ := fs.Glob(cfg.StaticFS, "web/templates/partials/*.html"); len(matches) > 0 {
		template.Must(tmpl.ParseFS(cfg.StaticFS, "web/templates/partials/*.html"))
	}
	return tmpl
}
