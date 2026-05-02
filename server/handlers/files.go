package handlers

import (
	"io"
	"net/http"
	"strings"

	"github.com/rpuneet/mycel/pkg/attachment"
)

// FileHandler handles file upload and download routes.
type FileHandler struct {
	store *attachment.Store
}

// NewFileHandler creates a FileHandler.
func NewFileHandler(store *attachment.Store) *FileHandler {
	return &FileHandler{store: store}
}

// Register mounts file routes on mux.
func (h *FileHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/files/upload", h.upload)
	mux.HandleFunc("/api/files/", h.download)
}

// upload handles multipart file upload.
// POST /api/files/upload
// Form fields: file (required), channel (required), sender (optional, default "web")
func (h *FileHandler) upload(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	// Limit request body to max file size + overhead
	r.Body = http.MaxBytesReader(w, r.Body, attachment.MaxFileSize+1024)

	if err := r.ParseMultipartForm(attachment.MaxFileSize); err != nil {
		httpError(w, "file too large or invalid multipart form", http.StatusBadRequest)
		return
	}

	channel := r.FormValue("channel")
	if channel == "" {
		httpError(w, "channel field required", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httpError(w, "file field required", http.StatusBadRequest)
		return
	}
	defer file.Close() //nolint:errcheck

	data, err := io.ReadAll(file)
	if err != nil {
		httpError(w, "failed to read file", http.StatusBadRequest)
		return
	}

	sender := r.FormValue("sender")
	if sender == "" {
		sender = "web"
	}

	meta, err := h.store.Save(data, header.Filename, channel, sender)
	if err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusCreated, meta)
}

// download serves a stored attachment.
// GET /api/files/{id}
func (h *FileHandler) download(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/files/")
	if id == "" || id == "upload" {
		httpError(w, "file ID required", http.StatusBadRequest)
		return
	}

	data, meta, err := h.store.Get(id)
	if err != nil {
		httpError(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", meta.MIMEType)
	w.Header().Set("Content-Disposition", "inline; filename=\""+meta.Filename+"\"")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data) //nolint:errcheck
}
