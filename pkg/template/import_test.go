package template

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseImportDoc_Valid(t *testing.T) {
	data := []byte(`{"name":"foo","description":"desc","mcps":["mycel"],"system_prompt":"hello"}`)
	tmpl, prompt, err := ParseImportDoc(data)
	if err != nil {
		t.Fatalf("ParseImportDoc: %v", err)
	}
	if tmpl.Name != "foo" || tmpl.Description != "desc" {
		t.Errorf("unexpected template: %+v", tmpl)
	}
	if len(tmpl.MCPs) != 1 || tmpl.MCPs[0] != "mycel" {
		t.Errorf("MCPs = %v", tmpl.MCPs)
	}
	if prompt != "hello" {
		t.Errorf("prompt = %q", prompt)
	}
}

func TestParseImportDoc_MissingName(t *testing.T) {
	data := []byte(`{"description":"desc"}`)
	if _, _, err := ParseImportDoc(data); err == nil {
		t.Fatalf("expected error for missing name")
	}
}

func TestParseImportDoc_InvalidJSON(t *testing.T) {
	if _, _, err := ParseImportDoc([]byte("not json")); err == nil {
		t.Fatalf("expected error for invalid JSON")
	}
}

func TestParseImportDoc_ScopeIsStripped(t *testing.T) {
	data := []byte(`{"name":"foo","scope":"workspace"}`)
	tmpl, _, err := ParseImportDoc(data)
	if err != nil {
		t.Fatalf("ParseImportDoc: %v", err)
	}
	if tmpl.Scope != "" {
		t.Errorf("scope should be stripped from import docs, got %q", tmpl.Scope)
	}
}

func TestFetchImportDoc_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"name":"remote","description":"from remote","system_prompt":"prompt text"}`))
	}))
	defer srv.Close()

	tmpl, prompt, err := FetchImportDoc(context.Background(), nil, srv.URL)
	if err != nil {
		t.Fatalf("FetchImportDoc: %v", err)
	}
	if tmpl.Name != "remote" || tmpl.Description != "from remote" {
		t.Errorf("unexpected template: %+v", tmpl)
	}
	if prompt != "prompt text" {
		t.Errorf("prompt = %q", prompt)
	}
}

func TestFetchImportDoc_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	if _, _, err := FetchImportDoc(context.Background(), nil, srv.URL); err == nil {
		t.Fatalf("expected error for HTTP 404")
	}
}

// A URL scheme that is not http(s) has no business reaching an HTTP client.
func TestFetchImportDocRejectsANonHTTPScheme(t *testing.T) {
	for _, raw := range []string{
		"file:///etc/passwd",
		"gopher://example.com/1",
		"ftp://example.com/t.json",
	} {
		if _, _, err := FetchImportDoc(context.Background(), nil, raw); err == nil {
			t.Errorf("FetchImportDoc(%q) = nil error, want a refusal", raw)
		}
	}
}

func TestFetchImportDocRejectsAURLWithNoHost(t *testing.T) {
	if _, _, err := FetchImportDoc(context.Background(), nil, "http://"); err == nil {
		t.Error("FetchImportDoc with no host = nil error, want a refusal")
	}
}
