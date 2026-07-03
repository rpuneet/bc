package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// githubTokenFile is the path where user's personal access token lives.
// Consistent with other bc secrets: ~/.mycel/github-token with 0600 perms.
const githubTokenFile = "github-token"

// githubAPIBase is the REST API base. Overridable in tests.
var githubAPIBase = "https://api.github.com"

// Repo mirrors the subset of a GitHub repository record we expose to the
// frontend.
type Repo struct {
	FullName      string `json:"full_name"`
	Name          string `json:"name"`
	CloneURL      string `json:"clone_url"`
	SSHURL        string `json:"ssh_url"`
	HTMLURL       string `json:"html_url"`
	DefaultBranch string `json:"default_branch"`
	Description   string `json:"description,omitempty"`
	Private       bool   `json:"private"`
}

// GithubTokenPath returns the filesystem location of the stored token.
// The directory is created as 0700 on first write.
func GithubTokenPath() (string, error) {
	home, err := MycelHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, githubTokenFile), nil
}

// ReadGithubToken returns the stored PAT, or "" if none. Any read error
// other than ENOENT is returned so the caller can distinguish "not set"
// from "read failed".
func ReadGithubToken() (string, error) {
	p, err := GithubTokenPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(p) //nolint:gosec // well-known token path under the mycel home
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// WriteGithubToken persists the PAT to ~/.mycel/github-token with 0600 perms.
// Empty token clears the file.
func WriteGithubToken(token string) error {
	p, err := GithubTokenPath()
	if err != nil {
		return err
	}
	if token == "" {
		if rmErr := os.Remove(p); rmErr != nil && !os.IsNotExist(rmErr) {
			return rmErr
		}
		return nil
	}
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(token), 0o600)
}

// DeleteGithubToken removes the token file, ignoring "not found".
func DeleteGithubToken() error {
	return WriteGithubToken("")
}

// GithubConnected returns true if a non-empty token is on disk.
func GithubConnected() bool {
	t, err := ReadGithubToken()
	return err == nil && t != ""
}

// ListGithubRepos returns up to 100 repos accessible to the authenticated
// user, filtered by optional substring query. Uses /user/repos which
// requires the `repo` scope.
func ListGithubRepos(ctx context.Context, query string) ([]Repo, error) {
	token, err := ReadGithubToken()
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, ErrGithubNotAuthenticated
	}

	u := githubAPIBase + "/user/repos?per_page=100&sort=updated"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck

	body, _ := io.ReadAll(resp.Body) //nolint:errcheck
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("github api: %d %s: %s", resp.StatusCode, resp.Status, strings.TrimSpace(string(body)))
	}

	var raw []struct {
		FullName      string `json:"full_name"`
		Name          string `json:"name"`
		CloneURL      string `json:"clone_url"`
		SSHURL        string `json:"ssh_url"`
		HTMLURL       string `json:"html_url"`
		DefaultBranch string `json:"default_branch"`
		Description   string `json:"description"`
		Private       bool   `json:"private"`
	}
	if uErr := json.Unmarshal(body, &raw); uErr != nil {
		return nil, fmt.Errorf("parse github response: %w", uErr)
	}

	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]Repo, 0, len(raw))
	for _, r := range raw {
		if q != "" && !strings.Contains(strings.ToLower(r.FullName), q) {
			continue
		}
		out = append(out, Repo{
			FullName:      r.FullName,
			Name:          r.Name,
			CloneURL:      r.CloneURL,
			SSHURL:        r.SSHURL,
			HTMLURL:       r.HTMLURL,
			DefaultBranch: r.DefaultBranch,
			Description:   r.Description,
			Private:       r.Private,
		})
	}
	return out, nil
}

// ValidateGithubToken performs a lightweight /user call to verify the token
// works. Returns the GitHub login on success.
func ValidateGithubToken(ctx context.Context, token string) (string, error) {
	if token == "" {
		return "", ErrGithubNotAuthenticated
	}
	u := githubAPIBase + "/user"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() //nolint:errcheck

	body, _ := io.ReadAll(resp.Body) //nolint:errcheck
	if resp.StatusCode == http.StatusUnauthorized {
		return "", ErrGithubNotAuthenticated
	}
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("github api: %d %s: %s", resp.StatusCode, resp.Status, strings.TrimSpace(string(body)))
	}
	var r struct {
		Login string `json:"login"`
	}
	if uErr := json.Unmarshal(body, &r); uErr != nil {
		return "", fmt.Errorf("parse /user: %w", uErr)
	}
	return r.Login, nil
}

// ErrGithubNotAuthenticated indicates no token is configured or it was
// rejected by the API.
var ErrGithubNotAuthenticated = errors.New("github not authenticated")

// SetGithubAPIBase is a test hook for retargeting the API base (e.g. at an
// httptest.Server). Not intended for production use.
func SetGithubAPIBase(base string) func() {
	old := githubAPIBase
	githubAPIBase = strings.TrimRight(base, "/")
	return func() { githubAPIBase = old }
}

// CloneURLFromRepo picks the best clone URL for a Repo: SSH if a credential
// helper appears configured, otherwise HTTPS. Returns the CloneURL field
// if no URL was provided.
func CloneURLFromRepo(repo Repo, preferSSH bool) string {
	if preferSSH && repo.SSHURL != "" {
		return repo.SSHURL
	}
	if repo.CloneURL != "" {
		return repo.CloneURL
	}
	return repo.HTMLURL
}

// ParseHTMLURL extracts owner/name from a github.com HTTPS URL for display
// purposes. Returns ("", "") on non-github hostnames.
func ParseHTMLURL(raw string) (owner, name string) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host != "github.com" {
		return "", ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 {
		return "", ""
	}
	return parts[0], strings.TrimSuffix(parts[1], ".git")
}
