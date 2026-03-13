package handler

import (
	"encoding/json"
	"net/http"

	"github.com/getkaze/keel/internal/updater"
)

// VersionHandler responds with the current and latest version info.
type VersionHandler struct {
	Version string
}

func (h *VersionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	result, err := updater.Check(h.Version)
	if err != nil {
		// If check fails (no internet), return current version with no update
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"current":   h.Version,
			"latest":    h.Version,
			"available": false,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
