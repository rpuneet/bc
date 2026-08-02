package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestAdapter builds an adapter wired to a fake GitHub API server.
func newTestAdapter(t *testing.T, token string, handler http.HandlerFunc) *Adapter {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	a := NewNamed("github:test", "", token)
	a.apiBase = srv.URL
	return a
}

func TestCommentPostsToCorrectURLWithBearerToken(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	a := newTestAdapter(t, "gh-token-123", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		var payload struct {
			Body string `json:"body"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		gotBody = payload.Body
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	})

	if err := a.Comment(context.Background(), "rpuneet", "mycel", 42, "nice PR"); err != nil {
		t.Fatalf("Comment() error = %v", err)
	}
	if gotPath != "/repos/rpuneet/mycel/issues/42/comments" {
		t.Errorf("path = %q, want /repos/rpuneet/mycel/issues/42/comments", gotPath)
	}
	if gotAuth != "Bearer gh-token-123" {
		t.Errorf("Authorization = %q, want Bearer gh-token-123", gotAuth)
	}
	if gotBody != "nice PR" {
		t.Errorf("body = %q, want %q", gotBody, "nice PR")
	}
}

func TestCommentMissingTokenReturnsSignInError(t *testing.T) {
	a := NewNamed("github:test", "", "")
	err := a.Comment(context.Background(), "rpuneet", "mycel", 1, "hi")
	if err == nil {
		t.Fatal("expected error for missing token")
	}
	if !strings.Contains(err.Error(), "sign in to GitHub first") {
		t.Errorf("error = %q, want mention of signing in", err.Error())
	}
}

func TestCommentAPIErrorSurfaced(t *testing.T) {
	a := newTestAdapter(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Resource not accessible by integration"}`))
	})

	err := a.Comment(context.Background(), "rpuneet", "mycel", 1, "hi")
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "Resource not accessible") {
		t.Errorf("error = %q, want it to surface status + body", err.Error())
	}
}

func TestCommentValidatesArguments(t *testing.T) {
	a := NewNamed("github:test", "", "tok")
	if err := a.Comment(context.Background(), "", "mycel", 1, "hi"); err == nil {
		t.Error("expected error for missing owner")
	}
	if err := a.Comment(context.Background(), "rpuneet", "mycel", 0, "hi"); err == nil {
		t.Error("expected error for non-positive number")
	}
	if err := a.Comment(context.Background(), "rpuneet", "mycel", 1, ""); err == nil {
		t.Error("expected error for empty body")
	}
}

func TestSetStatusPostsToCorrectURL(t *testing.T) {
	var gotPath string
	var gotPayload map[string]string
	a := newTestAdapter(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotPayload)
		w.WriteHeader(http.StatusCreated)
	})

	err := a.SetStatus(context.Background(), "rpuneet", "mycel", "abc123", "success", "all good", "https://ci.example.com/1", "mycel-ci")
	if err != nil {
		t.Fatalf("SetStatus() error = %v", err)
	}
	if gotPath != "/repos/rpuneet/mycel/statuses/abc123" {
		t.Errorf("path = %q, want /repos/rpuneet/mycel/statuses/abc123", gotPath)
	}
	if gotPayload["state"] != "success" || gotPayload["description"] != "all good" || gotPayload["context"] != "mycel-ci" {
		t.Errorf("payload = %+v, missing expected fields", gotPayload)
	}
}

func TestSetStatusRejectsInvalidState(t *testing.T) {
	a := NewNamed("github:test", "", "tok")
	if err := a.SetStatus(context.Background(), "rpuneet", "mycel", "sha", "bogus", "", "", ""); err == nil {
		t.Error("expected error for invalid state")
	}
}

func TestSendParsesIssueRefAndComments(t *testing.T) {
	var gotPath string
	a := newTestAdapter(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusCreated)
	})

	if err := a.Send(context.Background(), "rpuneet/mycel#7", "agent", "hello from an agent"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if gotPath != "/repos/rpuneet/mycel/issues/7/comments" {
		t.Errorf("path = %q, want /repos/rpuneet/mycel/issues/7/comments", gotPath)
	}
}

func TestSendRejectsMalformedTarget(t *testing.T) {
	a := NewNamed("github:test", "", "tok")
	for _, ref := range []string{"nofragment", "owner#1", "owner/repo#notanumber", "owner/repo#0"} {
		if err := a.Send(context.Background(), ref, "agent", "hi"); err == nil {
			t.Errorf("Send(%q) expected error, got nil", ref)
		}
	}
}
