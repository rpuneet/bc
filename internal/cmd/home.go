package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/workspace"
)

// runHome is called from runRoot with no arguments in an interactive
// terminal. The web UI is the product's rich surface: make sure the daemon
// is up, then open the dashboard in the browser.
func runHome(cmd *cobra.Command, _ []string) error {
	url := daemonWebURL()

	if !daemonReachable(cmd.Context(), url) {
		fmt.Println("Starting mycel server...")
		self, err := os.Executable()
		if err != nil {
			return fmt.Errorf("locate mycel binary: %w", err)
		}
		// #nosec G204 -- self is our own executable path.
		up := exec.CommandContext(cmd.Context(), self, "up", "-d")
		up.Stdout = os.Stdout
		up.Stderr = os.Stderr
		if err := up.Run(); err != nil {
			return fmt.Errorf("mycel up -d: %w", err)
		}
		// The daemon writes run/daemon.addr once listening; poll briefly.
		for range 20 {
			url = daemonWebURL()
			if daemonReachable(cmd.Context(), url) {
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
	}

	fmt.Printf("mycel is running — dashboard at %s\n", url)
	if err := openBrowser(cmd.Context(), url); err != nil {
		log.Debug("could not open browser", "error", err)
	}
	return nil
}

// daemonWebURL resolves the daemon's base URL from
// ~/.mycel/run/daemon.addr (scheme + host:port, written by `mycel up`),
// falling back to the default port.
func daemonWebURL() string {
	if p, err := workspace.DaemonAddrPath(); err == nil {
		// #nosec G304 -- fixed path under ~/.mycel, not user input.
		if b, readErr := os.ReadFile(p); readErr == nil {
			if addr := strings.TrimSpace(string(b)); addr != "" {
				if !strings.Contains(addr, "://") {
					addr = "http://" + addr
				}
				return addr
			}
		}
	}
	return "http://127.0.0.1:9374"
}

// daemonReachable reports whether a mycel daemon answers at url.
func daemonReachable(ctx context.Context, url string) bool {
	reqCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url+"/api/health", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close() //nolint:errcheck
	return resp.StatusCode == http.StatusOK
}

// openBrowser opens url with the platform's default browser.
func openBrowser(ctx context.Context, url string) error {
	var name string
	args := make([]string, 0, 3)
	switch runtime.GOOS {
	case "darwin":
		name = "open"
	case "windows":
		name, args = "cmd", append(args, "/c", "start")
	default:
		name = "xdg-open"
	}
	args = append(args, url)
	// #nosec G204 -- fixed command names, url is our own daemon address.
	return exec.CommandContext(ctx, name, args...).Start()
}
