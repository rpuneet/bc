package events

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultMaxLines is the max number of lines kept in the JSONL file.
	DefaultMaxLines = 10000
	// rotationTrimLines is how many lines we keep after rotation (trim oldest).
	rotationTrimLines = 5000
)

// SSEEvent is a single persisted SSE event with its broadcast timestamp.
type SSEEvent struct {
	Data      any       `json:"data"`
	Timestamp time.Time `json:"ts"`
	Type      string    `json:"type"`
}

// JSONLWriter appends SSE events to a JSONL file with line-count rotation.
// It is safe for concurrent use.
type JSONLWriter struct {
	path     string
	maxLines int
	mu       sync.Mutex
}

// NewJSONLWriter creates a writer that appends to the given path.
// maxLines controls when rotation triggers (0 = DefaultMaxLines).
func NewJSONLWriter(path string, maxLines int) *JSONLWriter {
	if maxLines <= 0 {
		maxLines = DefaultMaxLines
	}
	return &JSONLWriter{
		path:     path,
		maxLines: maxLines,
	}
}

// Write appends a single SSE event to the JSONL file.
func (w *JSONLWriter) Write(eventType string, data any) error {
	evt := SSEEvent{
		Type:      eventType,
		Data:      data,
		Timestamp: time.Now().UTC(),
	}
	line, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal SSE event: %w", err)
	}
	line = append(line, '\n')

	w.mu.Lock()
	defer w.mu.Unlock()

	f, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open JSONL file: %w", err)
	}
	if _, err := f.Write(line); err != nil {
		_ = f.Close() //nolint:errcheck // best-effort close on write error
		return fmt.Errorf("write JSONL line: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close JSONL file: %w", err)
	}

	// Check if rotation is needed (count lines)
	count, countErr := w.lineCount()
	if countErr == nil && count > w.maxLines {
		_ = w.rotate() //nolint:errcheck // best-effort rotation
	}

	return nil
}

// ReadLast returns the last n events from the JSONL file, oldest first.
func (w *JSONLWriter) ReadLast(n int) ([]SSEEvent, int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	lines, err := w.readAllLines()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, nil
		}
		return nil, 0, err
	}

	total := len(lines)
	if n <= 0 || n > total {
		n = total
	}

	// Take the last n lines
	start := total - n
	result := make([]SSEEvent, 0, n)
	for _, line := range lines[start:] {
		var evt SSEEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			continue // skip malformed lines
		}
		result = append(result, evt)
	}
	return result, total, nil
}

// ReadPage returns a page of events with offset/limit, oldest first.
// Returns events, total count, and any error.
func (w *JSONLWriter) ReadPage(limit, offset int) ([]SSEEvent, int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	lines, err := w.readAllLines()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, nil
		}
		return nil, 0, err
	}

	total := len(lines)
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return nil, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}

	result := make([]SSEEvent, 0, end-offset)
	for _, line := range lines[offset:end] {
		var evt SSEEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}
		result = append(result, evt)
	}
	return result, total, nil
}

// lineCount returns the number of lines in the file.
// Caller must hold w.mu.
func (w *JSONLWriter) lineCount() (int, error) {
	f, err := os.Open(w.path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }() //nolint:errcheck

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}

// readAllLines reads all non-empty lines from the file.
// Caller must hold w.mu.
func (w *JSONLWriter) readAllLines() ([][]byte, error) {
	f, err := os.Open(w.path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }() //nolint:errcheck

	var lines [][]byte
	scanner := bufio.NewScanner(f)
	// Allow large lines (1MB)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		b := scanner.Bytes()
		if len(b) == 0 {
			continue
		}
		cp := make([]byte, len(b))
		copy(cp, b)
		lines = append(lines, cp)
	}
	return lines, scanner.Err()
}

// rotate keeps only the newest rotationTrimLines lines.
// Caller must hold w.mu.
func (w *JSONLWriter) rotate() error {
	lines, err := w.readAllLines()
	if err != nil {
		return err
	}
	if len(lines) <= rotationTrimLines {
		return nil
	}

	// Keep only the last rotationTrimLines lines
	keep := lines[len(lines)-rotationTrimLines:]

	tmp := w.path + ".tmp"
	f, err := os.Create(tmp) //nolint:gosec // controlled repo path
	if err != nil {
		return fmt.Errorf("create tmp for rotation: %w", err)
	}

	bw := bufio.NewWriter(f)
	for _, line := range keep {
		if _, err := bw.Write(line); err != nil {
			_ = f.Close() //nolint:errcheck
			_ = os.Remove(tmp)
			return fmt.Errorf("write rotated line: %w", err)
		}
		if err := bw.WriteByte('\n'); err != nil {
			_ = f.Close() //nolint:errcheck
			_ = os.Remove(tmp)
			return fmt.Errorf("write newline: %w", err)
		}
	}
	if err := bw.Flush(); err != nil {
		_ = f.Close() //nolint:errcheck
		_ = os.Remove(tmp)
		return fmt.Errorf("flush rotated file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close rotated file: %w", err)
	}

	return os.Rename(tmp, w.path)
}

// taskIDRegexp matches "Task #123" in tool response strings.
var taskIDRegexp = regexp.MustCompile(`Task\s+#(\d+)`)

// TaskItem represents a task extracted from SSE event history.
type TaskItem struct {
	ID          string   `json:"id"`
	Subject     string   `json:"subject"`
	Status      string   `json:"status"`
	Owner       string   `json:"owner,omitempty"`
	Description string   `json:"description,omitempty"`
	BlockedBy   []string `json:"blockedBy,omitempty"`
}

// CurrentTasks scans the JSONL event history for TaskCreate/TaskUpdate
// events and builds the current task state. It returns all non-deleted tasks.
func (w *JSONLWriter) CurrentTasks() ([]TaskItem, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	lines, err := w.readAllLines()
	if err != nil {
		if os.IsNotExist(err) {
			return []TaskItem{}, nil
		}
		return nil, err
	}

	// tasks keyed by id, preserving insertion order via separate slice
	tasks := map[string]*TaskItem{}
	var order []string

	for _, line := range lines {
		var evt SSEEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}

		// Only process agent.hook events
		if evt.Type != "agent.hook" {
			continue
		}

		data, ok := evt.Data.(map[string]any)
		if !ok {
			continue
		}

		eventName, _ := data["event"].(string)
		toolName, _ := data["tool_name"].(string)

		// TaskCreate: extract from PostToolUse events
		if eventName == "PostToolUse" && strings.Contains(toolName, "TaskCreate") {
			task := extractTaskCreate(data)
			if task != nil {
				if _, exists := tasks[task.ID]; !exists {
					order = append(order, task.ID)
				}
				tasks[task.ID] = task
			}
		}

		// TaskUpdate: extract from Pre/PostToolUse events
		if (eventName == "PreToolUse" || eventName == "PostToolUse") && strings.Contains(toolName, "TaskUpdate") {
			id, status, blockedBy := extractTaskUpdate(data)
			if id != "" && status != "" {
				if t, exists := tasks[id]; exists {
					t.Status = status
					if len(blockedBy) > 0 {
						t.BlockedBy = append(t.BlockedBy, blockedBy...)
					}
				}
			}
		}
	}

	// Collect non-deleted tasks in insertion order
	result := make([]TaskItem, 0, len(order))
	for _, id := range order {
		if t := tasks[id]; t != nil && t.Status != "deleted" {
			result = append(result, *t)
		}
	}
	return result, nil
}

func extractTaskCreate(data map[string]any) *TaskItem {
	inp, _ := data["tool_input"].(map[string]any)
	resp, _ := data["tool_response"].(map[string]any)
	if inp == nil {
		return nil
	}

	id := "task-" + fmt.Sprintf("%d", time.Now().UnixMilli())

	// First, try to parse numeric ID from string response like "Task #125 created successfully: ..."
	if respStr, ok := data["tool_response"].(string); ok && respStr != "" {
		if m := taskIDRegexp.FindStringSubmatch(respStr); len(m) > 1 {
			id = m[1]
		} else {
			// Try parsing string response as JSON
			var parsed map[string]any
			if json.Unmarshal([]byte(respStr), &parsed) == nil {
				if s, ok := parsed["id"].(string); ok && s != "" {
					id = s
				}
			}
		}
	}

	// If still a fallback ID, try structured response fields
	if strings.HasPrefix(id, "task-") && resp != nil {
		if s, ok := resp["id"].(string); ok && s != "" {
			id = s
		} else if s, ok := resp["task_id"].(string); ok && s != "" {
			id = s
		}
	}

	subject := "Untitled task"
	if s, ok := inp["subject"].(string); ok && s != "" {
		subject = s
	} else if s, ok := inp["description"].(string); ok && s != "" {
		subject = s
	} else if s, ok := inp["title"].(string); ok && s != "" {
		subject = s
	}

	description, _ := inp["description"].(string)
	owner, _ := data["agent"].(string)

	return &TaskItem{
		ID:          id,
		Subject:     subject,
		Status:      "pending",
		Owner:       owner,
		Description: description,
	}
}

func extractTaskUpdate(data map[string]any) (string, string, []string) {
	inp, _ := data["tool_input"].(map[string]any)
	if inp == nil {
		return "", "", nil
	}

	var id string
	if s, ok := inp["taskId"].(string); ok {
		id = s
	} else if s, ok := inp["task_id"].(string); ok {
		id = s
	} else if s, ok := inp["id"].(string); ok {
		id = s
	}
	if id == "" {
		return "", "", nil
	}

	rawStatus, _ := inp["status"].(string)
	if rawStatus == "" {
		return "", "", nil
	}

	statusMap := map[string]string{
		"pending":     "pending",
		"in_progress": "in_progress",
		"in-progress": "in_progress",
		"inProgress":  "in_progress",
		"completed":   "completed",
		"done":        "completed",
		"deleted":     "deleted",
		"cancelled":   "deleted", //nolint:misspell // intentional alias for British spelling
		"canceled":    "deleted",
	}

	status, ok := statusMap[rawStatus]
	if !ok {
		status = "pending"
	}

	var blockedBy []string
	if arr, ok := inp["addBlockedBy"].([]any); ok {
		for _, v := range arr {
			if s, ok := v.(string); ok {
				blockedBy = append(blockedBy, s)
			}
		}
	}

	return id, status, blockedBy
}
