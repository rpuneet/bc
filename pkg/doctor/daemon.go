// daemon.go — health check against the running bcd daemon.
//
// Queries /api/health and surfaces any degraded services (services that
// failed to initialize at daemon boot and were silently left nil — see
// server/build_services.go). A daemon that is not running is reported
// informationally, never as a failure, so `mycel doctor` stays useful
// offline.
package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/rpuneet/mycel/pkg/client"
)

// daemonHealthTimeout bounds the /api/health roundtrip so doctor never
// hangs on a wedged daemon.
const daemonHealthTimeout = 3 * time.Second

// CheckDaemon queries the running daemon's /api/health endpoint and
// reports degraded services. The daemon address is auto-discovered the
// same way the CLI does (BC_DAEMON_ADDR env > daemon.addr file > default).
func CheckDaemon(ctx context.Context) CategoryReport {
	return CheckDaemonAt(ctx, client.New("").BaseURL)
}

// CheckDaemonAt is CheckDaemon against an explicit daemon base URL.
// Exposed for tests.
func CheckDaemonAt(ctx context.Context, baseURL string) CategoryReport {
	cat := CategoryReport{Name: "Daemon"}

	ctx, cancel := context.WithTimeout(ctx, daemonHealthTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/health", nil)
	if err != nil {
		cat.Items = append(cat.Items, Item{
			Name:     "daemon",
			Message:  fmt.Sprintf("invalid daemon address %q: %v", baseURL, err),
			Severity: SeverityWarn,
		})
		return cat
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Daemon down is not a workspace-health failure — skip gracefully.
		cat.Items = append(cat.Items, Item{
			Name:     "daemon",
			Message:  fmt.Sprintf("not reachable at %s — degraded-service check skipped", baseURL),
			Severity: SeverityOK,
		})
		return cat
	}
	defer func() { _ = resp.Body.Close() }()

	var health struct {
		Degraded map[string]string `json:"degraded"`
		Status   string            `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		cat.Items = append(cat.Items, Item{
			Name:     "daemon",
			Message:  fmt.Sprintf("unparseable /api/health response: %v", err),
			Severity: SeverityWarn,
		})
		return cat
	}

	if health.Status == "unhealthy" {
		cat.Items = append(cat.Items, Item{
			Name:     "daemon",
			Message:  "reachable but unhealthy (database check failed)",
			Severity: SeverityFail,
			Fix:      "check bcd logs, then restart with 'mycel down && mycel up'",
		})
		return cat
	}

	if len(health.Degraded) == 0 {
		cat.Items = append(cat.Items, Item{
			Name:     "services",
			Message:  "all daemon services healthy",
			Severity: SeverityOK,
		})
		return cat
	}

	names := make([]string, 0, len(health.Degraded))
	for name := range health.Degraded {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		cat.Items = append(cat.Items, Item{
			Name:     "service: " + name,
			Message:  health.Degraded[name],
			Severity: SeverityWarn,
			Fix:      "resolve the reason above, then restart the daemon ('mycel down && mycel up')",
		})
	}
	return cat
}
