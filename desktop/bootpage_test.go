package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// The handoff URL is the only channel through which the desktop app can tell the
// UI what version it is: the page is served entirely by the daemon, which may be
// an older one the app attached to rather than started.

func TestHandoffPathCarriesDesktopMarkerAndVersion(t *testing.T) {
	got := handoffPath("0.4.5-dev.12.g1a2b3c4")

	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("handoffPath produced an unparseable URL %q: %v", got, err)
	}
	q := u.Query()
	if q.Get("desktop") != "1" {
		t.Errorf("desktop marker missing from %q — external links would stop opening", got)
	}
	if q.Get("app_version") != "0.4.5-dev.12.g1a2b3c4" {
		t.Errorf("app_version = %q, want the version passed in (from %q)", q.Get("app_version"), got)
	}
}

// TestHandoffPathEncodesVersion: a source build's version is not a bare
// identifier, and an unencoded value would truncate or corrupt the query.
func TestHandoffPathEncodesVersion(t *testing.T) {
	got := handoffPath("0.0.0-dev.0.g0+weird value&desktop=0")

	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("unparseable URL %q: %v", got, err)
	}
	if v := u.Query().Get("app_version"); v != "0.0.0-dev.0.g0+weird value&desktop=0" {
		t.Errorf("app_version round-tripped as %q, want the original", v)
	}
	// The injected desktop=0 must not have become a second value for the marker.
	if got := u.Query()["desktop"]; len(got) != 1 || got[0] != "1" {
		t.Errorf("desktop marker = %v, want exactly [1] — a version must not be able to override it", got)
	}
}

// TestHandoffPathWithoutVersion: `go run ./desktop` and any build without
// ldflags has no version to report, and must not advertise an empty one.
func TestHandoffPathWithoutVersion(t *testing.T) {
	u, err := url.Parse(handoffPath(""))
	if err != nil {
		t.Fatalf("unparseable URL: %v", err)
	}
	if u.Query().Has("app_version") {
		t.Errorf("app_version present for an unset version: %q", u.RawQuery)
	}
	if u.Query().Get("desktop") != "1" {
		t.Error("desktop marker must survive an unset version")
	}
}

// TestBootMiddlewareServesTheHandoffTarget: the boot page is what the webview
// loads, so the version has to actually reach the rendered document.
func TestBootMiddlewareServesTheHandoffTarget(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot) // stands in for the embedded assets
	})
	h := bootMiddleware("http://127.0.0.1:9374", "0.4.5-dev.12.g1a2b3c4")(next)

	for _, path := range []string{"/", "/index.html"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want the boot page", path, rec.Code)
			continue
		}
		body := rec.Body.String()
		if !strings.Contains(body, "0.4.5-dev.12.g1a2b3c4") {
			t.Errorf("GET %s: boot page omits the app version, so the UI can never report it", path)
		}
		if !strings.Contains(body, "http://127.0.0.1:9374") {
			t.Errorf("GET %s: boot page omits the server URL", path)
		}
	}

	// Non-document requests still fall through to the assets.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if rec.Code != http.StatusTeapot {
		t.Errorf("asset request returned %d, want pass-through to the asset server", rec.Code)
	}
}
