package home

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestGithubTokenRoundTrip verifies Read/Write/Delete under a temp HOME.
func TestGithubTokenRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if GithubConnected() {
		t.Error("should not be connected initially")
	}
	if tok, err := ReadGithubToken(); err != nil || tok != "" {
		t.Errorf("ReadGithubToken initial = (%q, %v), want (\"\", nil)", tok, err)
	}

	if err := WriteGithubToken("gho_secret"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !GithubConnected() {
		t.Error("should be connected after write")
	}
	tok, err := ReadGithubToken()
	if err != nil || tok != "gho_secret" {
		t.Errorf("Read after write = (%q, %v)", tok, err)
	}

	// File perms should be 0600.
	p, _ := GithubTokenPath() //nolint:errcheck
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perms = %v, want 0600", info.Mode().Perm())
	}

	// Directory should be 0700.
	if dirInfo, err := os.Stat(filepath.Dir(p)); err == nil {
		if dirInfo.Mode().Perm() != 0o700 {
			t.Errorf("dir perms = %v, want 0700", dirInfo.Mode().Perm())
		}
	}

	if err := DeleteGithubToken(); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if GithubConnected() {
		t.Error("should not be connected after delete")
	}
	// Delete again is a no-op.
	if err := DeleteGithubToken(); err != nil {
		t.Errorf("Delete second time: %v", err)
	}
}

func TestListGithubReposNotAuthenticated(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := ListGithubRepos(context.Background(), "")
	if err != ErrGithubNotAuthenticated {
		t.Errorf("err = %v, want ErrGithubNotAuthenticated", err)
	}
}

func TestListGithubReposWithMockAPI(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := WriteGithubToken("testtoken"); err != nil {
		t.Fatalf("write token: %v", err)
	}

	body := `[
		{"full_name":"me/one","name":"one","clone_url":"https://github.com/me/one.git","ssh_url":"git@github.com:me/one.git","html_url":"https://github.com/me/one","default_branch":"main","private":false,"description":"one"},
		{"full_name":"me/two","name":"two","clone_url":"https://github.com/me/two.git","ssh_url":"git@github.com:me/two.git","html_url":"https://github.com/me/two","default_branch":"master","private":true}
	]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/repos" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer testtoken" {
			t.Errorf("auth = %q", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body)) //nolint:errcheck
	}))
	defer srv.Close()
	restore := SetGithubAPIBase(srv.URL)
	defer restore()

	repos, err := ListGithubRepos(context.Background(), "")
	if err != nil {
		t.Fatalf("ListGithubRepos: %v", err)
	}
	if len(repos) != 2 {
		t.Errorf("repos = %d, want 2", len(repos))
	}

	// query filters.
	repos, err = ListGithubRepos(context.Background(), "two")
	if err != nil {
		t.Fatalf("ListGithubRepos filter: %v", err)
	}
	if len(repos) != 1 || repos[0].Name != "two" {
		t.Errorf("filter result = %+v", repos)
	}
}

func TestValidateGithubToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Errorf("path = %q", r.URL.Path)
		}
		auth := r.Header.Get("Authorization")
		if auth == "Bearer goodtoken" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"login":"me"}`)) //nolint:errcheck
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	restore := SetGithubAPIBase(srv.URL)
	defer restore()

	login, err := ValidateGithubToken(context.Background(), "goodtoken")
	if err != nil || login != "me" {
		t.Errorf("Validate good = (%q, %v)", login, err)
	}

	if _, err := ValidateGithubToken(context.Background(), ""); err != ErrGithubNotAuthenticated {
		t.Errorf("empty token err = %v", err)
	}

	if _, err := ValidateGithubToken(context.Background(), "badtoken"); err != ErrGithubNotAuthenticated {
		t.Errorf("bad token err = %v, want ErrGithubNotAuthenticated", err)
	}
}

func TestParseHTMLURL(t *testing.T) {
	owner, name := ParseHTMLURL("https://github.com/foo/bar.git")
	if owner != "foo" || name != "bar" {
		t.Errorf("ParseHTMLURL = (%q, %q)", owner, name)
	}
	owner, name = ParseHTMLURL("https://gitlab.com/a/b")
	if owner != "" || name != "" {
		t.Errorf("non-github should return empty")
	}
}
