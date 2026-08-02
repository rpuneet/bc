package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// Comment posts a comment on a GitHub issue or pull request. GitHub treats
// PRs as issues for the comments API, so one call serves both.
func (a *Adapter) Comment(ctx context.Context, owner, repo string, number int, body string) error {
	if owner == "" || repo == "" || number <= 0 {
		return fmt.Errorf("github: comment requires owner, repo, and a positive issue/PR number")
	}
	if body == "" {
		return fmt.Errorf("github: comment body is required")
	}
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repo, number)
	return a.postJSON(ctx, path, map[string]string{"body": body})
}

// SetStatus sets a commit status on a specific SHA (POST
// /repos/{owner}/{repo}/statuses/{sha}). state must be one of GitHub's
// accepted values: "error", "failure", "pending", "success". description
// and targetURL are optional and may be passed as "".
func (a *Adapter) SetStatus(ctx context.Context, owner, repo, sha, state, description, targetURL, statusContext string) error {
	if owner == "" || repo == "" || sha == "" {
		return fmt.Errorf("github: status requires owner, repo, and sha")
	}
	switch state {
	case "error", "failure", "pending", "success":
	default:
		return fmt.Errorf("github: invalid status state %q (want error, failure, pending, or success)", state)
	}
	payload := map[string]string{"state": state}
	if description != "" {
		payload["description"] = description
	}
	if targetURL != "" {
		payload["target_url"] = targetURL
	}
	if statusContext != "" {
		payload["context"] = statusContext
	}
	path := fmt.Sprintf("/repos/%s/%s/statuses/%s", owner, repo, sha)
	return a.postJSON(ctx, path, payload)
}

// Send implements the gateway package's outbound messageSender capability
// so a GitHub comment can be posted through the same generic channel-send
// path used by Slack/Telegram/WhatsApp (POST /api/apps/channels/send and
// the send_message MCP tool). Since GitHub has no single "channel" concept,
// the channel id doubles as an issue/PR reference: "owner/repo#123".
func (a *Adapter) Send(ctx context.Context, channelID, _, content string) error {
	owner, repo, number, err := parseIssueRef(channelID)
	if err != nil {
		return err
	}
	return a.Comment(ctx, owner, repo, number, content)
}

// parseIssueRef parses "owner/repo#123" into its parts.
func parseIssueRef(ref string) (owner, repo string, number int, err error) {
	ownerRepo, numStr, ok := strings.Cut(ref, "#")
	if !ok {
		return "", "", 0, fmt.Errorf("github: target %q must be \"owner/repo#number\"", ref)
	}
	owner, repo, ok = strings.Cut(ownerRepo, "/")
	if !ok || owner == "" || repo == "" {
		return "", "", 0, fmt.Errorf("github: target %q must be \"owner/repo#number\"", ref)
	}
	number, err = strconv.Atoi(numStr)
	if err != nil || number <= 0 {
		return "", "", 0, fmt.Errorf("github: target %q has an invalid issue/PR number", ref)
	}
	return owner, repo, number, nil
}

// postJSON POSTs a JSON body to the GitHub REST API using the stored
// api_token as a Bearer credential. It returns a clear error when no token
// is configured, and surfaces GitHub's own error message on non-2xx status.
func (a *Adapter) postJSON(ctx context.Context, path string, payload any) error {
	a.mu.Lock()
	token := a.apiToken
	base := a.apiBase
	a.mu.Unlock()

	if token == "" {
		return fmt.Errorf("github: no API token configured — sign in to GitHub first")
	}
	if base == "" {
		base = apiBaseURL
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("github: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("github: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")

	res, err := a.httpc.Do(req)
	if err != nil {
		return fmt.Errorf("github: request %s: %w", path, err)
	}
	defer res.Body.Close() //nolint:errcheck // read-only body

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(res.Body, 1<<16))
		return fmt.Errorf("github: %s returned %d: %s", path, res.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}
