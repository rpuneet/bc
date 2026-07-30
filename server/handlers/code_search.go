// code_search.go — ripgrep-backed `/api/code/search` endpoint.
//
// Emits a compact JSON envelope the Code tab's search panel can render
// directly:
//
//	{
//	  "matches":    [{path, line, col, text, before:[...], after:[...]}],
//	  "truncated":  bool,
//	  "elapsed_ms": int
//	}
//
// The handler is read-only and sandboxed to the resolved worktree root.
// Case sensitivity, regex vs literal, and a single optional subdir are
// passed through; everything else (globs, context tuning, type filters)
// can be layered on later without changing the response shape.
package handlers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rpuneet/mycel/pkg/files"
	"github.com/rpuneet/mycel/pkg/log"
)

// searchMaxDefault caps the number of match records a single request
// returns. The Code tab should paginate above this; we error-bound the
// response size rather than streaming to keep the client simple.
const searchMaxDefault = 500

// searchMaxCeiling is a hard upper bound regardless of ?max=. Prevents
// an accidental &max=1000000 from tying up rg for minutes.
const searchMaxCeiling = 2000

// searchMaxQueryLen caps the raw `q` parameter length. Guards against
// pathologically long queries that inflate the argv and slow rg's
// regex compile; ordinary queries are well under this ceiling.
const searchMaxQueryLen = 1024

// searchTimeout bounds a single rg invocation. Long searches either hit
// this and return truncated=true, or complete well under it in practice.
const searchTimeout = 10 * time.Second

// searchMatch mirrors the public JSON shape. Field order matches the API
// contract — do not reorder without bumping a client.
type searchMatch struct {
	Path   string   `json:"path"`
	Text   string   `json:"text"`
	Before []string `json:"before,omitempty"`
	After  []string `json:"after,omitempty"`
	Line   int      `json:"line"`
	Col    int      `json:"col"`
}

type searchResponse struct {
	Matches   []searchMatch `json:"matches"`
	ElapsedMs int64         `json:"elapsed_ms"`
	Truncated bool          `json:"truncated"`
}

func (h *CodeHandler) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		httpError(w, "q required", http.StatusBadRequest)
		return
	}
	if len(q) > searchMaxQueryLen {
		httpError(w, "q too long", http.StatusBadRequest)
		return
	}

	_, wtRoot, _, err := h.resolveWorktreeRoot(r)
	if err != nil {
		httpError(w, err.Error(), http.StatusNotFound)
		return
	}
	if wtRoot == "" {
		httpError(w, "repo not resolved", http.StatusNotFound)
		return
	}

	searchRoot := wtRoot
	if sub := r.URL.Query().Get("path"); sub != "" {
		joined, joinErr := files.SafeJoin(wtRoot, sub)
		if joinErr != nil {
			httpError(w, "invalid path", http.StatusBadRequest)
			return
		}
		searchRoot = joined
	}

	max := searchMaxDefault
	if raw := r.URL.Query().Get("max"); raw != "" {
		n, parseErr := strconv.Atoi(raw)
		if parseErr != nil || n <= 0 {
			httpError(w, "invalid max", http.StatusBadRequest)
			return
		}
		if n > searchMaxCeiling {
			n = searchMaxCeiling
		}
		max = n
	}

	caseI := truthy(r.URL.Query().Get("case"))
	regex := truthy(r.URL.Query().Get("regex"))

	args := []string{
		"--json",
		"--max-filesize", "10M",
		"--no-messages",
		// Default rg behavior already hides dotfiles; these globs are
		// belt-and-braces for .git/ and .bc/ which we never want leaking
		// into a Code-tab search result.
		"-g", "!.git/",
		"-g", "!.bc/",
	}
	if caseI {
		args = append(args, "-i")
	}
	if !regex {
		args = append(args, "--fixed-strings")
	}
	// Context lines default to 1 before + 1 after so the UI can show a
	// peek without a second round-trip.
	args = append(args, "--before-context", "1", "--after-context", "1")
	args = append(args, "--", q, searchRoot)

	ctx, cancel := context.WithTimeout(r.Context(), searchTimeout)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(ctx, "rg", args...) //nolint:gosec // args are built from server-side constants + validated query
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Warn("code search: stdout pipe", "error", err)
		httpError(w, "search unavailable", http.StatusInternalServerError)
		return
	}
	// Capture stderr so we can distinguish "no matches" (rg exit 1,
	// empty stderr) from real failures (rg exit 2, non-empty stderr).
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	if startErr := cmd.Start(); startErr != nil {
		if errors.Is(startErr, exec.ErrNotFound) {
			log.Warn("code search: rg not installed on PATH")
			httpError(w, "ripgrep not installed on server", http.StatusNotImplemented)
			return
		}
		log.Warn("code search: rg start", "error", startErr)
		httpError(w, "search unavailable", http.StatusInternalServerError)
		return
	}

	matches, truncated, dropped, parseErr := parseRipgrepJSON(stdout, wtRoot, max)
	// Drain the rest of stdout so rg can exit cleanly when we hit max.
	waitErr := cmd.Wait()

	if ctx.Err() != nil {
		// Timed out — return whatever we have, flagged as truncated.
		truncated = true
	}

	// rg exit 2 means a real error (bad regex, unreadable tree, etc.).
	// Exit 1 is "no matches" — expected. Surface real errors as
	// truncated + warn so the UI doesn't silently show zero matches
	// on a broken invocation.
	if exitErr := (*exec.ExitError)(nil); errors.As(waitErr, &exitErr) && exitErr.ExitCode() >= 2 {
		truncated = true
		log.Warn("code search: rg exit",
			"code", exitErr.ExitCode(),
			"stderr", strings.TrimSpace(stderrBuf.String()))
	}

	if parseErr != nil && !errors.Is(parseErr, context.DeadlineExceeded) {
		log.Warn("code search: parse", "error", parseErr)
	}
	if dropped > 0 {
		// Malformed JSON from rg usually means buffer truncation or
		// stderr bleeding into stdout. Flag as truncated so the UI
		// warns the user rather than silently showing partial results.
		truncated = true
		log.Warn("code search: dropped malformed rg JSON lines", "count", dropped)
	}

	writeJSON(w, http.StatusOK, searchResponse{
		Matches:   matches,
		Truncated: truncated,
		ElapsedMs: time.Since(start).Milliseconds(),
	})
}

func truthy(v string) bool {
	switch strings.ToLower(v) {
	case "", "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// rgEvent models just enough of `rg --json` to extract match text +
// context. Other event types (begin, end, summary) are ignored.
type rgEvent struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		Lines struct {
			Text string `json:"text"`
		} `json:"lines"`
		Submatches []struct {
			Start int `json:"start"`
		} `json:"submatches"`
		LineNumber int `json:"line_number"`
	} `json:"data"`
}

// parseRipgrepJSON reads one JSON event per line. It pairs each `match`
// with the immediately surrounding `context` events (rg emits context
// inline around each match for the -C/-A/-B flags). Returns truncated=
// true when max is hit, and dropped=count of malformed lines that were
// skipped (callers should treat non-zero as a signal that the response
// is incomplete even if scan itself didn't error).
func parseRipgrepJSON(r interface {
	Read(p []byte) (n int, err error)
}, root string, max int) (matches []searchMatch, truncated bool, dropped int, err error) {
	scanner := bufio.NewScanner(r)
	// rg can emit long lines (e.g., minified files) — bump the buffer.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	matches = make([]searchMatch, 0, 32)
	var pending *searchMatch
	flush := func() {
		if pending != nil {
			matches = append(matches, *pending)
			pending = nil
		}
	}

	for scanner.Scan() {
		var ev rgEvent
		if unmarshalErr := json.Unmarshal(scanner.Bytes(), &ev); unmarshalErr != nil {
			dropped++
			continue
		}
		switch ev.Type {
		case "match":
			flush()
			rel, relErr := filepath.Rel(root, ev.Data.Path.Text)
			if relErr != nil {
				rel = ev.Data.Path.Text
			}
			col := 1
			if len(ev.Data.Submatches) > 0 {
				col = ev.Data.Submatches[0].Start + 1
			}
			pending = &searchMatch{
				Path: rel,
				Line: ev.Data.LineNumber,
				Col:  col,
				Text: strings.TrimRight(ev.Data.Lines.Text, "\n"),
			}
			// Truncate only when appending this match would push us
			// PAST max. At exactly max we still let it through and
			// keep reading to check whether another match would follow
			// — that is the only way to report truncation accurately.
			if len(matches)+1 > max {
				pending = nil
				truncated = true
				return matches, truncated, dropped, nil
			}
		case "context":
			if pending == nil {
				// Context for a prior flushed match — attach to last.
				if n := len(matches); n > 0 {
					matches[n-1].After = append(matches[n-1].After, strings.TrimRight(ev.Data.Lines.Text, "\n"))
				}
				continue
			}
			line := strings.TrimRight(ev.Data.Lines.Text, "\n")
			if ev.Data.LineNumber < pending.Line {
				pending.Before = append(pending.Before, line)
			} else {
				pending.After = append(pending.After, line)
			}
		}
	}
	flush()
	if scanErr := scanner.Err(); scanErr != nil {
		return matches, truncated, dropped, fmt.Errorf("scan: %w", scanErr)
	}
	return matches, truncated, dropped, nil
}
