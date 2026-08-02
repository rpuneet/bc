package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func openURLMux() http.Handler {
	mux := http.NewServeMux()
	NewOpenURLHandler().Register(mux)
	return mux
}

// TestOpenURLValidHTTP asserts a valid http(s) URL returns 204 and invokes the
// stubbed opener with the exact URL.
func TestOpenURLValidHTTP(t *testing.T) {
	orig := openURLFunc
	t.Cleanup(func() { openURLFunc = orig })

	var got string
	openURLFunc = func(_ context.Context, u string) error {
		got = u
		return nil
	}

	const want = "https://github.com/login/device"
	rec := httptest.NewRecorder()
	openURLMux().ServeHTTP(rec, loopbackPost("/api/system/open-url", `{"url":"`+want+`"}`))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if got != want {
		t.Fatalf("opener got %q, want %q", got, want)
	}
}

// TestOpenURLRejectsNonHTTPScheme asserts a non-http(s) URL is rejected with
// 400 before the opener runs.
func TestOpenURLRejectsNonHTTPScheme(t *testing.T) {
	orig := openURLFunc
	t.Cleanup(func() { openURLFunc = orig })

	ran := false
	openURLFunc = func(_ context.Context, _ string) error {
		ran = true
		return nil
	}

	for _, bad := range []string{
		`{"url":"file:///etc/passwd"}`,
		`{"url":"javascript:alert(1)"}`,
		`{"url":"ftp://example.com"}`,
		`{"url":"not a url"}`,
		`{"url":""}`,
	} {
		rec := httptest.NewRecorder()
		openURLMux().ServeHTTP(rec, loopbackPost("/api/system/open-url", bad))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want %d", bad, rec.Code, http.StatusBadRequest)
		}
	}
	if ran {
		t.Fatal("opener ran for a rejected URL")
	}
}

// TestOpenURLRejectsNonLoopback asserts a non-loopback remote address is
// rejected with 403 before the opener runs.
func TestOpenURLRejectsNonLoopback(t *testing.T) {
	orig := openURLFunc
	t.Cleanup(func() { openURLFunc = orig })

	ran := false
	openURLFunc = func(_ context.Context, _ string) error {
		ran = true
		return nil
	}

	req := httptest.NewRequest(http.MethodPost, "/api/system/open-url", nil)
	req.RemoteAddr = "203.0.113.7:44321"

	rec := httptest.NewRecorder()
	openURLMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if ran {
		t.Fatal("opener ran for a non-loopback caller")
	}
}
