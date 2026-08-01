package handlers_test

import (
	"bytes"
	"io"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/server"
)

var pngMagic = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}

// TestAgentAvatarEndpoint verifies GET /api/agents/{name}/avatar.png returns a
// deterministic PNG and avatar.svg returns SVG — the agent's AgentCharacter,
// rendered server-side and cacheable.
func TestAgentAvatarEndpoint(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".mycel")
	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	// PNG — valid magic, correct content type, and byte-for-byte deterministic
	// across calls (no agent record needed; it derives from the name).
	get := func(path string) (status int, contentType string, body []byte) {
		t.Helper()
		resp, err := http.Get(ts.URL + path) //nolint:noctx // test
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ = io.ReadAll(resp.Body) //nolint:errcheck // test read
		return resp.StatusCode, resp.Header.Get("Content-Type"), body
	}

	status, ct, png1 := get("/api/agents/zen-zebra/avatar.png")
	if status != http.StatusOK {
		t.Fatalf("avatar.png status = %d, want 200", status)
	}
	if ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
	if !bytes.HasPrefix(png1, pngMagic) {
		t.Fatalf("not a PNG: % x", png1)
	}
	_, _, png2 := get("/api/agents/zen-zebra/avatar.png")
	if !bytes.Equal(png1, png2) {
		t.Error("avatar.png not deterministic across calls")
	}
	_, _, other := get("/api/agents/pi/avatar.png")
	if bytes.Equal(png1, other) {
		t.Error("distinct agents returned identical avatars")
	}

	// SVG variant.
	statusSVG, ctSVG, svg := get("/api/agents/zen-zebra/avatar.svg")
	if statusSVG != http.StatusOK {
		t.Fatalf("avatar.svg status = %d, want 200", statusSVG)
	}
	if ctSVG != "image/svg+xml" {
		t.Errorf("Content-Type = %q, want image/svg+xml", ctSVG)
	}
	if !bytes.HasPrefix(svg, []byte("<svg")) {
		t.Errorf("avatar.svg body not an SVG: %.40s", svg)
	}
}
