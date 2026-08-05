package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/client"
	"github.com/rpuneet/mycel/pkg/notify"
	"github.com/rpuneet/mycel/pkg/ui"
)

func newNotifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "notify",
		Aliases: []string{"n"},
		Short:   "Manage channel subscriptions and gateway notifications",
		Long: `Manage agent subscriptions to gateway channels (Slack, Telegram, Discord).

Channels deliver external app messages to subscribed agents via tmux send-keys.
Agents respond using the platform's own MCP tools.

Examples:
  mycel notify status                               # Show gateway connection status
  mycel notify list                                  # List all subscriptions
  mycel notify subscribe slack:eng eng-01            # Subscribe agent to channel
  mycel notify unsubscribe slack:eng eng-01          # Unsubscribe agent
  mycel notify activity slack:eng                    # Show delivery activity log
  mycel notify prune --dry-run                       # Preview leftover catch-all copies
  mycel notify prune --yes                           # Delete matching leftovers`,
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show gateway connection status and subscriptions",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := client.New("")
			ctx := context.Background()

			subs, err := c.Notify.ListSubscriptions(ctx)
			if err != nil {
				return fmt.Errorf("status: %w", err)
			}

			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(subs)
			}

			if len(subs) == 0 {
				fmt.Println("No subscriptions configured.")
				fmt.Println("Use 'mycel notify subscribe <channel> <agent>' to add one.")
				return nil
			}

			// Group by channel
			byChannel := map[string][]client.Subscription{}
			for _, sub := range subs {
				byChannel[sub.Channel] = append(byChannel[sub.Channel], sub)
			}

			for ch, chSubs := range byChannel {
				fmt.Printf("  %s\n", ui.CyanText(ch))
				for _, sub := range chSubs {
					mention := ""
					if sub.MentionOnly {
						mention = " (@mention only)"
					}
					fmt.Printf("    → %s%s\n", sub.Agent, mention)
				}
			}
			return nil
		},
	}
	statusCmd.Flags().Bool("json", false, "Output as JSON")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all agent subscriptions",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := client.New("")
			ctx := context.Background()

			subs, err := c.Notify.ListSubscriptions(ctx)
			if err != nil {
				return fmt.Errorf("list subscriptions: %w", err)
			}

			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(subs)
			}

			if len(subs) == 0 {
				fmt.Println("No subscriptions.")
				return nil
			}

			for _, sub := range subs {
				mention := ""
				if sub.MentionOnly {
					mention = ui.CyanText(" (@mention only)")
				}
				fmt.Printf("  %-25s → %s%s\n", sub.Channel, sub.Agent, mention)
			}
			return nil
		},
	}
	listCmd.Flags().Bool("json", false, "Output as JSON")

	subscribeCmd := &cobra.Command{
		Use:   "subscribe <channel> <agent>",
		Short: "Subscribe an agent to a channel",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			channel, agent := args[0], args[1]
			mentionOnly, _ := cmd.Flags().GetBool("mention-only")

			c := client.New("")
			ctx := context.Background()
			if err := c.Notify.Subscribe(ctx, channel, agent, mentionOnly); err != nil {
				return fmt.Errorf("subscribe: %w", err)
			}

			mention := ""
			if mentionOnly {
				mention = " (@mention only)"
			}
			fmt.Printf("Subscribed %s to %s%s\n", agent, channel, mention)
			return nil
		},
	}
	subscribeCmd.Flags().Bool("mention-only", false, "Only deliver messages that @mention this agent")

	unsubscribeCmd := &cobra.Command{
		Use:   "unsubscribe <channel> <agent>",
		Short: "Unsubscribe an agent from a channel",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			channel, agent := args[0], args[1]
			c := client.New("")
			ctx := context.Background()
			if err := c.Notify.Unsubscribe(ctx, channel, agent); err != nil {
				return fmt.Errorf("unsubscribe: %w", err)
			}
			fmt.Printf("Unsubscribed %s from %s\n", agent, channel)
			return nil
		},
	}

	activityCmd := &cobra.Command{
		Use:   "activity <channel>",
		Short: "Show delivery activity for a channel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			channel := args[0]
			limit, _ := cmd.Flags().GetInt("limit")

			c := client.New("")
			ctx := context.Background()
			entries, err := c.Notify.Activity(ctx, channel, limit)
			if err != nil {
				return fmt.Errorf("activity: %w", err)
			}

			if len(entries) == 0 {
				fmt.Printf("No delivery activity for %s\n", channel)
				return nil
			}

			for _, e := range entries {
				status := e.Status
				switch e.Status {
				case "delivered":
					status = ui.GreenText("delivered")
				case "failed":
					status = ui.RedText("failed")
				case "pending":
					status = ui.YellowText("pending")
				}
				preview := e.Preview
				if len(preview) > 60 {
					preview = preview[:60] + "..."
				}
				fmt.Printf("  %s  %-10s → %-15s  %s\n", e.LoggedAt.Format("15:04:05"), status, e.Agent, preview)
			}
			return nil
		},
	}
	activityCmd.Flags().Int("limit", 20, "Number of recent entries to show")

	pruneCmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove leftover catch-all auto-copied subscriptions",
		Long: `List (and optionally delete) per-channel subscriptions that look like
copies of a platform catch-all ("{platform}:general") row.

Before #3464, catch-all delivery wrote permanent rows onto every real channel.
Those rows have no provenance, so prune uses a heuristic: same agent and
mention_only as an existing catch-all subscription. Deliberate subscriptions
that happen to match are included — review the list before confirming.

Examples:
  mycel notify prune --dry-run
  mycel notify prune --platform gmail
  mycel notify prune --yes`,
		RunE: runNotifyPrune,
	}
	pruneCmd.Flags().Bool("dry-run", false, "List candidates without deleting")
	pruneCmd.Flags().Bool("yes", false, "Delete without interactive confirmation")
	pruneCmd.Flags().String("platform", "", "Only consider channels for this platform (e.g. gmail)")
	pruneCmd.Flags().Bool("json", false, "Output candidates as JSON")

	cmd.AddCommand(statusCmd, listCmd, subscribeCmd, unsubscribeCmd, activityCmd, pruneCmd)
	return cmd
}

func runNotifyPrune(cmd *cobra.Command, _ []string) error {
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	yes, _ := cmd.Flags().GetBool("yes")
	platform, _ := cmd.Flags().GetString("platform")
	jsonFlag, _ := cmd.Flags().GetBool("json")

	c := client.New("")
	ctx := context.Background()
	subs, err := c.Notify.ListSubscriptions(ctx)
	if err != nil {
		return fmt.Errorf("list subscriptions: %w", err)
	}

	notifySubs := make([]notify.Subscription, len(subs))
	for i, s := range subs {
		notifySubs[i] = notify.Subscription{
			ID:          s.ID,
			Channel:     s.Channel,
			Agent:       s.Agent,
			MentionOnly: s.MentionOnly,
			Muted:       s.Muted,
			CreatedAt:   s.CreatedAt,
		}
	}
	candidates := notify.FilterPruneByPlatform(notify.FindPruneCandidates(notifySubs), platform)

	if jsonFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(candidates); err != nil {
			return err
		}
	} else if len(candidates) == 0 {
		fmt.Println("No catch-all copy candidates found.")
		return nil
	} else {
		fmt.Printf("Found %d candidate subscription(s) that match a catch-all row:\n", len(candidates))
		for _, sub := range candidates {
			mention := ""
			if sub.MentionOnly {
				mention = " (@mention only)"
			}
			fmt.Printf("  %-40s → %s%s\n", sub.Channel, sub.Agent, mention)
		}
		fmt.Println()
		fmt.Println("These may be leftover auto-copies; deliberate subscriptions that")
		fmt.Println("mirror the catch-all settings are also listed. Review carefully.")
	}

	if len(candidates) == 0 || dryRun {
		if dryRun && !jsonFlag && len(candidates) > 0 {
			fmt.Println("Dry run — nothing deleted. Re-run with --yes to remove them.")
		}
		return nil
	}
	if !yes {
		if jsonFlag {
			return fmt.Errorf("refusing to delete without --yes when using --json")
		}
		fmt.Print("\nType 'yes' to delete these subscriptions: ")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			return fmt.Errorf("read confirmation: %w", err)
		}
		if strings.TrimSpace(line) != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	var failed int
	for _, sub := range candidates {
		if err := c.Notify.Unsubscribe(ctx, sub.Channel, sub.Agent); err != nil {
			fmt.Fprintf(os.Stderr, "failed %s → %s: %v\n", sub.Channel, sub.Agent, err)
			failed++
			continue
		}
		if !jsonFlag {
			fmt.Printf("Removed %s → %s\n", sub.Channel, sub.Agent)
		}
	}
	if failed > 0 {
		return fmt.Errorf("removed %d, failed %d", len(candidates)-failed, failed)
	}
	if jsonFlag {
		fmt.Fprintf(os.Stderr, "removed %d subscription(s)\n", len(candidates))
	} else {
		fmt.Printf("Removed %d subscription(s).\n", len(candidates))
	}
	return nil
}

func init() {
	rootCmd.AddCommand(newNotifyCmd())
}
