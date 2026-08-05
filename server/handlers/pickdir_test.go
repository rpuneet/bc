package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func pickDirMux() http.Handler {
	mux := http.NewServeMux()
	NewPickDirectoryHandler().Register(mux)
	return mux
}

func TestPickDirectoryReturnsPath(t *testing.T) {
	orig := pickDirectoryFunc
	t.Cleanup(func() { pickDirectoryFunc = orig })

	pickDirectoryFunc = func(_ context.Context) (string, error) {
		return "/Users/me/Projects", nil
	}

	rec := httptest.NewRecorder()
	pickDirMux().ServeHTTP(rec, loopbackPost("/api/system/pick-directory", `{}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["path"] != "/Users/me/Projects" {
		t.Fatalf("path = %q, want /Users/me/Projects", body["path"])
	}
}

func TestPickDirectoryCanceledIsNoContent(t *testing.T) {
	orig := pickDirectoryFunc
	t.Cleanup(func() { pickDirectoryFunc = orig })

	pickDirectoryFunc = func(_ context.Context) (string, error) {
		return "", ErrPickCanceled
	}

	rec := httptest.NewRecorder()
	pickDirMux().ServeHTTP(rec, loopbackPost("/api/system/pick-directory", `{}`))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestPickDirectoryRejectsNonLoopback(t *testing.T) {
	orig := pickDirectoryFunc
	t.Cleanup(func() { pickDirectoryFunc = orig })

	ran := false
	pickDirectoryFunc = func(_ context.Context) (string, error) {
		ran = true
		return "/tmp", nil
	}

	req := httptest.NewRequest(http.MethodPost, "/api/system/pick-directory", nil)
	req.RemoteAddr = "203.0.113.7:44321"
	rec := httptest.NewRecorder()
	pickDirMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if ran {
		t.Fatal("picker ran for a non-loopback caller")
	}
}

func TestPickDirectoryDialogFailure(t *testing.T) {
	orig := pickDirectoryFunc
	t.Cleanup(func() { pickDirectoryFunc = orig })

	pickDirectoryFunc = func(_ context.Context) (string, error) {
		return "", errors.New("no folder dialog available")
	}

	rec := httptest.NewRecorder()
	pickDirMux().ServeHTTP(rec, loopbackPost("/api/system/pick-directory", `{}`))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}
