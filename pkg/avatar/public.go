package avatar

import (
	"os"
	"strings"
)

// PublicBaseEnv names the environment variable that, when set to a public HTTPS
// base (e.g. "https://bc-infra.com/avatars"), makes PublicURL resolve each
// agent's avatar to a genuinely public URL. It is set only once the per-agent
// PNGs have actually been published there (see cmd/mycel avatar publish and the
// landing/public/avatars deploy). Left unset, PublicURL returns "" and callers
// fall back honestly — never a faked or unreachable public link.
const PublicBaseEnv = "MYCEL_AVATAR_PUBLIC_BASE"

// PublicURL returns the public, internet-reachable URL of an agent's avatar PNG,
// or "" when no public base is configured. The URL is <base>/<name>.png; it is
// the source of truth for Slack icon_url (Slack fetches icons from the public
// internet, so a loopback daemon URL will not do).
func PublicURL(name string) string {
	base := strings.TrimSpace(os.Getenv(PublicBaseEnv))
	if base == "" || name == "" {
		return ""
	}
	return strings.TrimRight(base, "/") + "/" + name + ".png"
}
