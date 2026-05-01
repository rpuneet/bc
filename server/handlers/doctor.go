package handlers

import (
	"net/http"
	"strings"

	"github.com/rpuneet/mycel/pkg/doctor"
	"github.com/rpuneet/mycel/pkg/workspace"
)

// DoctorHandler handles /api/doctor routes.
type DoctorHandler struct {
	ws *workspace.Workspace
}

// NewDoctorHandler creates a DoctorHandler.
func NewDoctorHandler(ws *workspace.Workspace) *DoctorHandler {
	return &DoctorHandler{ws: ws}
}

// Register mounts doctor routes on mux.
func (h *DoctorHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/doctor", h.runAll)
	mux.HandleFunc("/api/doctor/", h.byCategory)
}

func (h *DoctorHandler) runAll(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	report := doctor.RunAll(r.Context(), h.ws)
	writeJSON(w, http.StatusOK, report)
}

func (h *DoctorHandler) byCategory(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	category := strings.TrimPrefix(r.URL.Path, "/api/doctor/")
	if category == "" {
		httpError(w, "category required", http.StatusBadRequest)
		return
	}
	report := doctor.CategoryByName(r.Context(), h.ws, category)
	if report == nil {
		httpError(w, "unknown category", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
