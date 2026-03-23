package handler

import (
	"fmt"
	"net/http"

	"github.com/getkaze/keel/internal/tunnel"
)

// TunnelStatusHandler streams tunnel status changes as SSE events.
type TunnelStatusHandler struct {
	Monitor *tunnel.Monitor // nil for local targets
}

func (h *TunnelStatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// For local targets, send "connected" and close.
	if h.Monitor == nil {
		fmt.Fprintf(w, "data: connected\n\n")
		flusher.Flush()
		return
	}

	// Send current status immediately.
	fmt.Fprintf(w, "data: %s\n\n", h.Monitor.Status())
	flusher.Flush()

	// Subscribe to status changes.
	ch := h.Monitor.Subscribe()
	defer h.Monitor.Unsubscribe(ch)

	for {
		select {
		case <-r.Context().Done():
			return
		case status, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", status)
			flusher.Flush()
		}
	}
}
