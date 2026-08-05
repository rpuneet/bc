package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// GitHubRelease represents the shape of a GitHub release response.
type GitHubRelease struct {
	TagName     string `json:"tag_name"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
}

const (
	releaseCacheTTL = 1 * time.Hour
	ghRepo          = "rpuneet/mycel"
)

// ghAPIURL is the upstream releases endpoint. Overridable in tests.
var ghAPIURL = "https://api.github.com/repos/" + ghRepo + "/releases/latest"

// cachedRelease holds a release with its cache metadata.
type cachedRelease struct {
	release   *GitHubRelease
	fetchedAt time.Time
	status    string // "ok", "rate_limited", "error"
}

// ReleaseHandler serves the latest GitHub release with server-side caching
// to avoid hitting GitHub's unauthenticated rate limit from the browser.
type ReleaseHandler struct {
	cache   *cachedRelease
	httpCli *http.Client
	mu      sync.RWMutex
	sf      singleflight.Group
}

// NewReleaseHandler creates a new ReleaseHandler with a default HTTP client.
func NewReleaseHandler() *ReleaseHandler {
	return &ReleaseHandler{
		httpCli: &http.Client{Timeout: 30 * time.Second},
	}
}

// Register routes the release handler to /api/releases/latest.
func (h *ReleaseHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/releases/latest", h.handle)
}

func (h *ReleaseHandler) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	release, status := h.getRelease(r.Context())
	if release == nil {
		// Return the status even when there's no release, so the UI knows
		// the difference between "rate limited" and "up to date" (or error).
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": status,
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"tag_name":     release.TagName,
		"html_url":     release.HTMLURL,
		"published_at": release.PublishedAt,
		"status":       status,
	})
}

func (h *ReleaseHandler) getRelease(ctx context.Context) (*GitHubRelease, string) {
	h.mu.RLock()
	cache := h.cache
	h.mu.RUnlock()

	now := time.Now()
	if cache != nil && now.Sub(cache.fetchedAt) < releaseCacheTTL {
		return cache.release, cache.status
	}

	// Coalesce concurrent refreshes so a thundering herd of About views
	// issues one GitHub request per TTL window (#3620 CodeRabbit).
	v, _, _ := h.sf.Do("latest", func() (any, error) { //nolint:errcheck // closure never returns an error
		h.mu.RLock()
		c := h.cache
		h.mu.RUnlock()
		if c != nil && time.Since(c.fetchedAt) < releaseCacheTTL {
			return c, nil
		}
		release, status := h.fetchRelease(ctx)
		entry := &cachedRelease{
			release:   release,
			fetchedAt: time.Now(),
			status:    status,
		}
		h.mu.Lock()
		h.cache = entry
		h.mu.Unlock()
		return entry, nil
	})
	entry := v.(*cachedRelease)
	return entry.release, entry.status
}

func (h *ReleaseHandler) fetchRelease(ctx context.Context) (*GitHubRelease, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ghAPIURL, nil)
	if err != nil {
		return nil, "error"
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := h.httpCli.Do(req)
	if err != nil {
		return nil, "error"
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == 429 {
		// Rate limited — return nil with explicit status so UI knows it's not "up to date"
		return nil, "rate_limited"
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "error"
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, "error"
	}
	return &release, "ok"
}
