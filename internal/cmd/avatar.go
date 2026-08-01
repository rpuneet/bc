package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/avatar"
)

var (
	avatarOut  string
	avatarSize int
	avatarSVG  bool
)

// agentAvatarCmd generates the deterministic AgentCharacter avatar PNGs (and
// optionally SVGs) for agents into a directory — the deploy step that makes the
// avatars publicly reachable. Point --out at the landing site's public assets
// (landing/public/avatars), commit and deploy landing, then set
// MYCEL_AVATAR_PUBLIC_BASE=https://bc-infra.com/avatars so whoami and the Slack
// gateway hand out the public URL. Same creature the mycel UI draws.
var agentAvatarCmd = &cobra.Command{
	Use:   "avatar [names...]",
	Short: "Generate agent AgentCharacter avatar images (for public hosting)",
	Long: `Generate the deterministic AgentCharacter avatar for one or more agents as
PNG (and optionally SVG) files in an output directory.

With no names, every current agent is rendered (requires a running daemon).
With names, exactly those are rendered offline — no daemon needed, since the
avatar derives purely from the name.

Publish flow (makes avatars fetchable by Slack, which needs a public URL):
  1. mycel agent avatar --out landing/public/avatars
  2. commit landing/public/avatars and deploy the landing site
  3. export MYCEL_AVATAR_PUBLIC_BASE=https://bc-infra.com/avatars
Then whoami.avatar_url and the Slack gateway icon_url resolve to the public PNG.`,
	RunE: runAgentAvatar,
}

func init() {
	agentAvatarCmd.Flags().StringVar(&avatarOut, "out", "landing/public/avatars", "output directory for avatar files")
	agentAvatarCmd.Flags().IntVar(&avatarSize, "size", 256, "avatar dimension in pixels")
	agentAvatarCmd.Flags().BoolVar(&avatarSVG, "svg", false, "also write an .svg alongside each .png")
	agentCmd.AddCommand(agentAvatarCmd)
}

func runAgentAvatar(cmd *cobra.Command, args []string) error {
	names := args
	if len(names) == 0 {
		c, err := newDaemonClient(cmd.Context())
		if err != nil {
			return fmt.Errorf("no names given and daemon unreachable: %w", err)
		}
		list, err := c.Agents.List(cmd.Context())
		if err != nil {
			return fmt.Errorf("list agents: %w", err)
		}
		for _, a := range list {
			names = append(names, a.Name)
		}
		if len(names) == 0 {
			return fmt.Errorf("no agents to render")
		}
	}

	if err := os.MkdirAll(avatarOut, 0o750); err != nil {
		return fmt.Errorf("create out dir: %w", err)
	}

	for _, name := range names {
		if !isValidAgentName(name) {
			return fmt.Errorf("invalid agent name %q", name)
		}
		png, err := avatar.PNG(name, avatarSize)
		if err != nil {
			return fmt.Errorf("render %q: %w", name, err)
		}
		pngPath := filepath.Join(avatarOut, name+".png")
		if err := os.WriteFile(pngPath, png, 0o644); err != nil { //nolint:gosec // public asset, world-readable by design
			return fmt.Errorf("write %s: %w", pngPath, err)
		}
		fmt.Printf("wrote %s (%d bytes)\n", pngPath, len(png))

		if avatarSVG {
			svgPath := filepath.Join(avatarOut, name+".svg")
			if err := os.WriteFile(svgPath, []byte(avatar.SVG(name, avatarSize)), 0o644); err != nil { //nolint:gosec // public asset
				return fmt.Errorf("write %s: %w", svgPath, err)
			}
			fmt.Printf("wrote %s\n", svgPath)
		}
	}

	fmt.Printf("\nRendered %d avatar(s) to %s\n", len(names), avatarOut)
	fmt.Println("Next: commit & deploy the landing site, then set")
	fmt.Println("  export MYCEL_AVATAR_PUBLIC_BASE=https://bc-infra.com/avatars")
	return nil
}
