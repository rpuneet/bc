package handlers

import (
	"net/http"
	"strings"

	"github.com/rpuneet/mycel/pkg/doctor"
	"github.com/rpuneet/mycel/pkg/home"
)

// DoctorHandler handles /api/doctor routes.
type DoctorHandler struct {
	h *home.Home
}

// NewDoctorHandler creates a DoctorHandler.
func NewDoctorHandler(h *home.Home) *DoctorHandler {
	return &DoctorHandler{h: h}
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
	report := doctor.RunAll(r.Context(), h.h)
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
	report := doctor.CategoryByName(r.Context(), h.h, category)
	if report == nil {
		httpError(w, "unknown category", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
