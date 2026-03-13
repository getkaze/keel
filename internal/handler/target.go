package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/getkaze/keel/internal/config"
)

// TargetHandler serves target information.
type TargetHandler struct {
	KeelDir string
}

// ServeHTTP handles GET /api/target.
func (h *TargetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, err := config.ReadTarget(h.KeelDir)
	if err != nil {
		log.Printf("target error: %v", err)
		http.Error(w, "failed to read target", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}
