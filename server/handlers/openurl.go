package handlers

import (
	"context"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"time"
)

// OpenURLHandler exposes POST /api/system/open-url, the one reliable way for
// the desktop app to hand an external link to the OS browser.
//
// Why this exists: the Wails webview boots a tiny page that navigates to the
// in-process daemon's http://127.0.0.1 origin. Wails only injects
// window.runtime (and BrowserOpenURL) into pages served through its own asset
// scheme — never into an external http:// origin — so the frontend helper's
// window.open / <a target="_blank"> fallbacks are no-ops inside the macOS
// WKWebView. This endpoint lets the web UI ask the daemon to open the link
// with the host's default browser instead.
//
// Security model: loopback-only (the daemon binds loopback by default) and
// http/https only — the URL is exec'd as a single argv element to a fixed
// platform opener, never through a shell.
type OpenURLHandler struct{}

// NewOpenURLHandler constructs an OpenURLHandler.
func NewOpenURLHandler() *OpenURLHandler { return &OpenURLHandler{} }

// Register mounts POST /api/system/open-url.
func (h *OpenURLHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/system/open-url", h.open)
}

// realOpenURL opens url with the platform's default browser. The URL is passed
// as a single argument to a fixed opener binary — never interpreted by a shell.
func realOpenURL(ctx context.Context, rawURL string) error {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name, args = "open", []string{rawURL}
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", rawURL}
	default:
		name, args = "xdg-open", []string{rawURL}
	}
	// The launch must OUTLIVE the HTTP request: r.Context() is canceled the
	// instant the handler returns 204, which would kill the opener before it
	// hands the URL to the browser. Sever cancellation (keep any values) and
	// give the child a bounded lifetime of its own.
	launchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	// #nosec G204 -- name is a fixed constant; rawURL is validated http/https
	// and passed as a single argv element, so no shell interprets it.
	cmd := exec.CommandContext(launchCtx, name, args...)
	if err := cmd.Start(); err != nil {
		cancel()
		return err
	}
	// Reap the child so it doesn't linger as a zombie; cancel frees the
	// timeout context once the opener exits (it does so near-instantly).
	go func() {
		_ = cmd.Wait()
		cancel()
	}()
	return nil
}

// openURLFunc is the injectable opener so tests can stub the OS call.
var openURLFunc = realOpenURL

// open handles POST /api/system/open-url. Body: {"url":"https://…"}.
func (h *OpenURLHandler) open(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !checkLoopback(w, r) {
		return
	}

	var req struct {
		URL string `json:"url"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		httpError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	u, err := url.Parse(req.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		httpError(w, "invalid url: only http and https URLs are allowed", http.StatusBadRequest)
		return
	}

	if err := openURLFunc(r.Context(), u.String()); err != nil {
		httpError(w, "failed to open url", http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
