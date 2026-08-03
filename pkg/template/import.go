package template

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ImportDoc is the on-the-wire JSON shape accepted by template import
// (the `mycel template import` CLI command and the marketplace install
// endpoint). It mirrors Template plus a plain-text SystemPrompt field,
// since the on-disk Store keeps the prompt in a sibling .md file rather
// than inside the template's JSON.
type ImportDoc struct { //nolint:govet // embedding Template; reordering would obscure the JSON shape
	Template
	SystemPrompt string `json:"system_prompt,omitempty"`
}

// maxImportBytes caps how much of a remote or local import document is
// read, guarding against runaway files/responses.
const maxImportBytes = 1 << 20 // 1 MiB

// importFetchTimeout bounds a remote import. Without one, a server that accepts
// the connection and then says nothing holds the import open indefinitely.
const importFetchTimeout = 20 * time.Second

// checkImportScheme rejects anything but http(s). A bare path is a local file,
// which the caller handles before reaching here, and any other scheme —
// file://, gopher://, and the rest — has no business being handed to an HTTP
// client.
func checkImportScheme(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse %s: %w", rawURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("template import needs an http or https URL, or a local file path; got scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("template import URL %q names no host", rawURL)
	}
	return nil
}

// ParseImportDoc decodes raw JSON bytes describing a template into a
// Template plus its system prompt text.
func ParseImportDoc(data []byte) (Template, string, error) {
	var doc ImportDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return Template{}, "", fmt.Errorf("parse template import document: %w", err)
	}
	if doc.Name == "" {
		return Template{}, "", fmt.Errorf(`template import document is missing required "name" field`)
	}
	// Scope is runtime-only and must never be sourced from an import doc.
	doc.Scope = ""
	return doc.Template, doc.SystemPrompt, nil
}

// FetchImportDoc fetches an import document from rawURL and parses it.
// client may be nil, in which case a client with a timeout is used.
//
// This is for a URL a person typed: `mycel template import <url>`. It is
// deliberately not reachable from an HTTP request, because a URL chosen by a
// request would let the caller aim the daemon at loopback services or a cloud
// metadata endpoint. The marketplace install path resolves templates from the
// local store only, and dispatches remote ones to an agent to import here.
func FetchImportDoc(ctx context.Context, client *http.Client, rawURL string) (Template, string, error) {
	if err := checkImportScheme(rawURL); err != nil {
		return Template{}, "", err
	}
	if client == nil {
		client = &http.Client{Timeout: importFetchTimeout}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Template{}, "", fmt.Errorf("build request for %s: %w", rawURL, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return Template{}, "", fmt.Errorf("GET %s: %w", rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return Template{}, "", fmt.Errorf("GET %s: HTTP %d", rawURL, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImportBytes))
	if err != nil {
		return Template{}, "", fmt.Errorf("read response from %s: %w", rawURL, err)
	}
	return ParseImportDoc(data)
}
