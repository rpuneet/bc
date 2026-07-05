package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rpuneet/mycel/pkg/deps"
)

// DepsHandler exposes the optional dependencies manager (see pkg/deps).
//
// Security: Start/Stop can spawn arbitrary docker commands, so we require
// the request to originate from loopback unless MYCEL_REMOTE=1 is explicitly
// set by the operator.
type DepsHandler struct {
	registry *deps.Registry
}

// NewDepsHandler constructs a DepsHandler around an existing registry.
func NewDepsHandler(registry *deps.Registry) *DepsHandler {
	return &DepsHandler{registry: registry}
}

// Register mounts the deps routes on mux.
//
// Routes:
//
//	GET  /api/deps                 -> list deps + status
//	GET  /api/deps/{id}            -> one dep detail
//	GET  /api/deps/{id}/status     -> same as detail (compat)
//	POST /api/deps/{id}/start      -> start (loopback-only)
//	POST /api/deps/{id}/stop       -> stop  (loopback-only)
//	GET  /api/deps/{id}/logs       -> SSE stream of log tails
func (h *DepsHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/deps", h.list)
	mux.HandleFunc("/api/deps/", h.item)
}

// depView is the JSON shape returned by GET endpoints.
type depView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	State       string `json:"state"`
	Error       string `json:"error,omitempty"`
	Deprecated  bool   `json:"deprecated"`
}

func viewOfDep(ctx context.Context, d deps.Dependency) depView {
	v := depView{
		ID:          d.ID(),
		Name:        d.DisplayName(),
		Description: d.Description(),
		Deprecated:  d.Deprecated(),
	}
	st, err := d.Status(ctx)
	v.State = string(st)
	if err != nil {
		v.Error = err.Error()
	}
	return v
}

// list handles GET /api/deps.
func (h *DepsHandler) list(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if h.registry == nil {
		writeJSON(w, http.StatusOK, map[string]any{"deps": []depView{}})
		return
	}
	list := h.registry.List()
	out := make([]depView, 0, len(list))
	for _, d := range list {
		out = append(out, viewOfDep(r.Context(), d))
	}
	writeJSON(w, http.StatusOK, map[string]any{"deps": out})
}

// item routes /api/deps/{id}[/action].
func (h *DepsHandler) item(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/deps/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}
	id, tail, _ := strings.Cut(rest, "/")
	if id == "" {
		http.NotFound(w, r)
		return
	}

	dep, err := h.resolve(id)
	if err != nil {
		if errors.Is(err, deps.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		httpInternalError(w, "resolve dep", err)
		return
	}

	switch tail {
	case "":
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		writeJSON(w, http.StatusOK, viewOfDep(r.Context(), dep))
	case "status":
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		writeJSON(w, http.StatusOK, viewOfDep(r.Context(), dep))
	case "start":
		h.start(w, r, dep)
	case "stop":
		h.stop(w, r, dep)
	case "logs":
		h.logs(w, r, dep)
	default:
		http.NotFound(w, r)
	}
}

func (h *DepsHandler) resolve(id string) (deps.Dependency, error) {
	if h.registry == nil {
		return nil, deps.ErrNotFound
	}
	return h.registry.Get(id)
}

// start handles POST /api/deps/{id}/start.
func (h *DepsHandler) start(w http.ResponseWriter, r *http.Request, d deps.Dependency) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !checkLoopback(w, r) {
		return
	}
	if d.Deprecated() {
		httpError(w, "dependency deprecated", http.StatusConflict)
		return
	}
	if err := d.Start(r.Context()); err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, viewOfDep(r.Context(), d))
}

// stop handles POST /api/deps/{id}/stop.
func (h *DepsHandler) stop(w http.ResponseWriter, r *http.Request, d deps.Dependency) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !checkLoopback(w, r) {
		return
	}
	if err := d.Stop(r.Context()); err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, viewOfDep(r.Context(), d))
}

// logs handles GET /api/deps/{id}/logs as a simple SSE stream.
//
// Each tick we fetch `tail` lines via dep.Logs and emit the new lines (by
// index) as individual SSE messages. When the tail stabilizes, we keep the
// stream open but emit nothing until new lines appear. The stream closes
// when the client disconnects.
func (h *DepsHandler) logs(w http.ResponseWriter, r *http.Request, d deps.Dependency) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	tail := 200
	if s := r.URL.Query().Get("tail"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			tail = clampInt(n, 1, 2000)
		}
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		httpError(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	writeEvent := func(event, data string) bool {
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	var seen int
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	emit := func() bool {
		lines, err := d.Logs(r.Context(), tail)
		if err != nil {
			payload, _ := json.Marshal(map[string]string{"error": err.Error()})
			return writeEvent("error", string(payload))
		}
		// For the first tick emit whatever tail we have; on subsequent
		// ticks only emit lines we haven't seen yet. We key "seen" on
		// the line count rather than content to keep the implementation
		// simple and correct for append-only logs.
		start := seen
		if len(lines) < start {
			// Rotation / truncation — resync.
			start = 0
		}
		for i := start; i < len(lines); i++ {
			payload, _ := json.Marshal(map[string]string{"line": lines[i]})
			if !writeEvent("log", string(payload)) {
				return false
			}
		}
		seen = len(lines)
		return true
	}

	if !emit() {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !emit() {
				return
			}
		}
	}
}

// checkLoopback enforces that mutating dep endpoints only accept requests
// from 127.0.0.1 / ::1 unless MYCEL_REMOTE=1 is set.
func checkLoopback(w http.ResponseWriter, r *http.Request) bool {
	if os.Getenv("MYCEL_REMOTE") == "1" {
		return true
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr may not have a port when synthesized by tests.
		host = r.RemoteAddr
	}
	if host == "" {
		httpError(w, "forbidden", http.StatusForbidden)
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		httpError(w, "forbidden", http.StatusForbidden)
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	httpError(w, "forbidden: loopback required (set MYCEL_REMOTE=1 to override)", http.StatusForbidden)
	return false
}
