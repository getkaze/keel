package server

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/getkaze/keel/internal/config"
)

// Config holds server configuration.
type Config struct {
	Port     int
	Bind     string
	Dev      bool
	KeelDir  string
	StaticFS fs.FS
	Version  string
	Ctx      context.Context        // Application-wide context, cancelled on shutdown
	Target   *config.TargetConfig   // Active target (nil = local)
}

// Server wraps the HTTP server with middleware and routing.
type Server struct {
	httpServer *http.Server
	config     Config
}

// New creates a new Server with the given configuration.
func New(cfg Config) *Server {
	mux := http.NewServeMux()

	s := &Server{
		config: cfg,
		httpServer: &http.Server{
			Addr:         fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port),
			Handler:      loggingMiddleware(recoveryMiddleware(mux)),
			ReadTimeout: 15 * time.Second,
			// WriteTimeout disabled to support long-lived SSE streams
			// (install/update/reset can take several minutes)
			IdleTimeout: 120 * time.Second,
		},
	}

	registerRoutes(mux, cfg)
	return s
}

// Start begins listening. Blocks until the server stops.
func (s *Server) Start() error {
	log.Printf("keel %s starting on %s", s.config.Version, s.httpServer.Addr)
	log.Printf("keel-dir=%s dev=%v", s.config.KeelDir, s.config.Dev)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// loggingMiddleware logs method, path, status, and duration.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.status, time.Since(start).Round(time.Microsecond))
	})
}

// recoveryMiddleware catches panics and returns 500.
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("PANIC: %s %s: %v", r.Method, r.URL.Path, err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// statusWriter wraps ResponseWriter to capture the status code.
// It also implements http.Hijacker and http.Flusher so that WebSocket
// upgrades and SSE streaming work through the middleware chain.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := sw.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
}

func (sw *statusWriter) Flush() {
	if fl, ok := sw.ResponseWriter.(http.Flusher); ok {
		fl.Flush()
	}
}
