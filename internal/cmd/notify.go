package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/client"
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
  mycel notify activity slack:eng                    # Show delivery activity log`,
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

	cmd.AddCommand(statusCmd, listCmd, subscribeCmd, unsubscribeCmd, activityCmd)
	return cmd
}

func init() {
	rootCmd.AddCommand(newNotifyCmd())
}
