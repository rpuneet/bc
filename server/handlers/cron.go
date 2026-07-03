package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rpuneet/mycel/pkg/cron"
)

// CronHandler handles /api/cron routes.
type CronHandler struct {
	store     *cron.Store
	scheduler *cron.Scheduler
}

// NewCronHandler creates a CronHandler.
func NewCronHandler(store *cron.Store, scheduler *cron.Scheduler) *CronHandler {
	return &CronHandler{store: store, scheduler: scheduler}
}

// resolveStore returns the context-scoped cron store with closure fallback.
func (h *CronHandler) resolveStore(r *http.Request) *cron.Store {
	if view := WorkspaceFromContext(r.Context()); view != nil && view.Cron != nil {
		return view.Cron
	}
	return h.store
}

// resolveScheduler returns the context-scoped scheduler with closure fallback.
func (h *CronHandler) resolveScheduler(r *http.Request) *cron.Scheduler {
	if view := WorkspaceFromContext(r.Context()); view != nil && view.CronSched != nil {
		return view.CronSched
	}
	return h.scheduler
}

// Register mounts cron routes on mux.
func (h *CronHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/cron", h.list)
	mux.HandleFunc("/api/cron/", h.byName)
}

func (h *CronHandler) list(w http.ResponseWriter, r *http.Request) {
	store := h.resolveStore(r)
	sched := h.resolveScheduler(r)
	switch r.Method {
	case http.MethodGet:
		jobs, err := store.ListJobs(r.Context())
		if err != nil {
			httpInternalError(w, "list jobs", err)
			return
		}
		if jobs == nil {
			jobs = []*cron.Job{}
		}
		// Enrich with running state from scheduler
		if sched != nil {
			for _, j := range jobs {
				j.Running = sched.IsRunning(j.Name)
			}
		}
		limit, offset := parsePagination(r, 50)
		if offset >= len(jobs) {
			jobs = []*cron.Job{}
		} else {
			jobs = jobs[offset:]
			if len(jobs) > limit {
				jobs = jobs[:limit]
			}
		}
		writeJSON(w, http.StatusOK, jobs)

	case http.MethodPost:
		var job cron.Job
		if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
			httpError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if job.Command == "" && job.Prompt == "" {
			httpError(w, "command or prompt is required", http.StatusBadRequest)
			return
		}
		if err := store.AddJob(r.Context(), &job); err != nil {
			httpError(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, job)

	default:
		methodNotAllowed(w)
	}
}

func (h *CronHandler) byName(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/api/cron/"), "/", 2)
	name := parts[0]
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}
	if name == "" {
		httpError(w, "job name required", http.StatusBadRequest)
		return
	}

	switch sub {
	case "":
		h.job(w, r, name)
	case "enable":
		h.setEnabled(w, r, name, true)
	case "disable":
		h.setEnabled(w, r, name, false)
	case "run":
		h.run(w, r, name)
	case "logs":
		h.logs(w, r, name)
	case "logs/live":
		h.liveLogs(w, r, name)
	default:
		httpError(w, "not found", http.StatusNotFound)
	}
}

func (h *CronHandler) job(w http.ResponseWriter, r *http.Request, name string) {
	store := h.resolveStore(r)
	scheduler := h.resolveScheduler(r)
	switch r.Method {
	case http.MethodGet:
		job, err := store.GetJob(r.Context(), name)
		if err != nil {
			httpError(w, err.Error(), http.StatusNotFound)
			return
		}
		if job == nil {
			httpError(w, "cron job not found", http.StatusNotFound)
			return
		}
		if h.scheduler != nil {
			job.Running = scheduler.IsRunning(name)
		}
		writeJSON(w, http.StatusOK, job)

	case http.MethodDelete:
		if err := store.DeleteJob(r.Context(), name); err != nil {
			httpError(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		methodNotAllowed(w)
	}
}

func (h *CronHandler) setEnabled(w http.ResponseWriter, r *http.Request, name string, enabled bool) {
	store := h.resolveStore(r)
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if err := store.SetEnabled(r.Context(), name, enabled); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": enabled})
}

func (h *CronHandler) run(w http.ResponseWriter, r *http.Request, name string) {
	store := h.resolveStore(r)
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	job, err := store.GetJob(r.Context(), name)
	if err != nil {
		httpError(w, err.Error(), http.StatusNotFound)
		return
	}
	if job == nil {
		httpError(w, "cron job not found", http.StatusNotFound)
		return
	}
	if !job.Enabled {
		httpError(w, "cron job is disabled", http.StatusBadRequest)
		return
	}
	if err := store.RecordManualTrigger(r.Context(), name); err != nil {
		httpInternalError(w, "operation failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "triggered"})
}

func (h *CronHandler) logs(w http.ResponseWriter, r *http.Request, name string) {
	store := h.resolveStore(r)
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	last := 20
	if s := r.URL.Query().Get("last"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			last = n
		}
	}
	last = clampInt(last, 1, 1000)
	logs, err := store.GetLogs(r.Context(), name, last)
	if err != nil {
		httpInternalError(w, "operation failed", err)
		return
	}
	writeJSON(w, http.StatusOK, logs)
}

// liveLogs streams the live log file for a running cron job via SSE.
func (h *CronHandler) liveLogs(w http.ResponseWriter, r *http.Request, name string) {
	scheduler := h.resolveScheduler(r)
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if scheduler == nil {
		serviceUnavailable(w, r, "cron", "scheduler not available")
		return
	}

	logPath := scheduler.LogFilePath(name)
	if logPath == "" {
		httpError(w, "log streaming not available", http.StatusNotImplemented)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		httpError(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()

	// Tail the log file, sending new content as SSE events
	var lastSize int64
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			info, err := os.Stat(logPath)
			if err != nil {
				// File doesn't exist yet or job not running
				if !scheduler.IsRunning(name) {
					// Job finished — send done event and close
					fmt.Fprintf(w, "event: done\ndata: {}\n\n") //nolint:errcheck
					flusher.Flush()
					return
				}
				continue
			}

			currentSize := info.Size()
			if currentSize <= lastSize {
				if !scheduler.IsRunning(name) {
					fmt.Fprintf(w, "event: done\ndata: {}\n\n") //nolint:errcheck
					flusher.Flush()
					return
				}
				continue
			}

			// Read new content
			f, openErr := os.Open(logPath) //nolint:gosec
			if openErr != nil {
				continue
			}
			if _, seekErr := f.Seek(lastSize, io.SeekStart); seekErr != nil {
				_ = f.Close()
				continue
			}
			newData, readErr := io.ReadAll(f)
			_ = f.Close()
			if readErr != nil || len(newData) == 0 {
				continue
			}
			lastSize = currentSize

			// Send as SSE data
			fmt.Fprintf(w, "data: %s\n\n", strings.ReplaceAll(string(newData), "\n", "\ndata: ")) //nolint:errcheck
			flusher.Flush()
		}
	}
}
