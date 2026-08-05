package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetReleaseCoalescesConcurrentFetches(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		time.Sleep(50 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(GitHubRelease{
			TagName:     "v0.4.7",
			HTMLURL:     "https://github.com/rpuneet/mycel/releases/tag/v0.4.7",
			PublishedAt: "2026-08-05T00:00:00Z",
		})
	}))
	t.Cleanup(srv.Close)

	prev := ghAPIURL
	ghAPIURL = srv.URL
	t.Cleanup(func() { ghAPIURL = prev })

	h := NewReleaseHandler()
	h.httpCli = srv.Client()

	const n = 20
	type result struct {
		tag    string
		status string
	}
	out := make(chan result, n)
	for range n {
		go func() {
			rel, status := h.getRelease(t.Context())
			tag := ""
			if rel != nil {
				tag = rel.TagName
			}
			out <- result{tag: tag, status: status}
		}()
	}
	for i := range n {
		r := <-out
		if r.status != "ok" || r.tag != "v0.4.7" {
			t.Fatalf("result[%d] = %+v", i, r)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("upstream calls = %d, want 1 (singleflight)", got)
	}
}
