package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/rpuneet/mycel/pkg/events"
)

// EventHandler handles /api/logs (historical event log).
type EventHandler struct {
	store  events.EventStore
	writer *events.JSONLWriter
}

// NewEventHandler creates an EventHandler.
func NewEventHandler(store events.EventStore) *EventHandler {
	return &EventHandler{store: store}
}

// SetWriter attaches a JSONLWriter for the /api/events/history endpoint.
func (h *EventHandler) SetWriter(w *events.JSONLWriter) {
	h.writer = w
}

// Register mounts event log routes on mux.
func (h *EventHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/logs", h.logs)
	mux.HandleFunc("/api/logs/", h.byAgent)
	mux.HandleFunc("/api/events/history", h.history)
	mux.HandleFunc("/api/tasks/current", h.currentTasks)
}

func (h *EventHandler) logs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.list(w, r)
	case http.MethodPost:
		h.appendEvent(w, r)
	default:
		methodNotAllowed(w)
	}
}

func (h *EventHandler) list(w http.ResponseWriter, r *http.Request) {
	store := h.store
	tail := 100
	if s := r.URL.Query().Get("tail"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			tail = n
		}
	}
	tail = clampInt(tail, 1, 10000)
	if store == nil {
		writeJSON(w, http.StatusOK, []events.Event{})
		return
	}
	evts, err := store.ReadLast(tail)
	if err != nil {
		httpInternalError(w, "read events", err)
		return
	}
	if evts == nil {
		evts = []events.Event{}
	}
	writeJSON(w, http.StatusOK, evts)
}

func (h *EventHandler) byAgent(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/logs/")
	if name == "" {
		httpError(w, "agent name required", http.StatusBadRequest)
		return
	}
	store := h.store
	if store == nil {
		writeJSON(w, http.StatusOK, []events.Event{})
		return
	}

	// Default preserves the historical DefaultReadLimit window; a limit param
	// bounds the page and before=<id> pages older events (newest first).
	limit := events.DefaultReadLimit
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	limit = clampInt(limit, 1, events.DefaultReadLimit)
	var before int64
	if s := r.URL.Query().Get("before"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
			before = n
		}
	}

	evts, err := store.ReadByAgentPage(name, limit, before)
	if err != nil {
		httpInternalError(w, "read events", err)
		return
	}
	if evts == nil {
		evts = []events.Event{}
	}
	writeJSON(w, http.StatusOK, evts)
}

func (h *EventHandler) appendEvent(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var ev events.Event
	if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
		httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	store := h.store
	if store == nil {
		httpError(w, "event store not configured", http.StatusInternalServerError)
		return
	}
	if err := store.Append(ev); err != nil {
		httpInternalError(w, "append event", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// currentTasks serves GET /api/tasks/current
// Returns the current task list derived from TaskCreate/TaskUpdate SSE events.
func (h *EventHandler) currentTasks(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writer := h.writer
	if writer == nil {
		httpError(w, "event history not configured", http.StatusServiceUnavailable)
		return
	}

	tasks, err := writer.CurrentTasks()
	if err != nil {
		httpInternalError(w, "read current tasks", err)
		return
	}

	writeJSON(w, http.StatusOK, tasks)
}

// history serves GET /api/events/history?limit=100&offset=0
// Returns paginated SSE events from the JSONL persistence file.
func (h *EventHandler) history(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writer := h.writer
	if writer == nil {
		httpError(w, "event history not configured", http.StatusServiceUnavailable)
		return
	}

	limit := 100
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	limit = clampInt(limit, 1, 10000)

	offset := 0
	if s := r.URL.Query().Get("offset"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			offset = n
		}
	}

	evts, total, err := writer.ReadPage(limit, offset)
	if err != nil {
		httpInternalError(w, "read event history", err)
		return
	}
	if evts == nil {
		evts = []events.SSEEvent{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"events": evts,
		"total":  total,
	})
}
