package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/client"
	"github.com/rpuneet/mycel/pkg/ui"
)

var channelCmd = &cobra.Command{
	Use:     "channel",
	Aliases: []string{"ch"},
	Short:   "Manage communication channels",
	Long: `Manage channels for broadcasting messages to groups of agents.

Channels are named groups of agent members. Messages sent to a channel are
delivered to all member tmux sessions.

Examples:
  mycel channel list                      # List all channels
  mycel channel create workers            # Create a channel named "workers"
  mycel channel show workers              # Show channel details
  mycel channel add workers worker-01     # Add member to channel
  mycel channel add workers --agent w-01  # Add member via --agent flag
  mycel channel send workers "run tests"  # Send to all members
  mycel channel history workers --last 20 # Show last 20 messages
  mycel channel react workers 5 👍        # React to message
  mycel channel edit workers --desc "..."  # Edit channel description
  mycel channel remove workers worker-01  # Remove a member
  mycel channel delete workers            # Delete the channel
  mycel channel status                    # Overview of all channels

Agent Commands (require MYCEL_AGENT_ID):
  mycel channel join workers              # Join a channel (current agent)
  mycel channel leave workers             # Leave a channel (current agent)

Default Channels:
  #eng       Engineering team (all engineer agents)
  #pr        Pull request reviews and notifications
  #standup   Daily standup updates
  #leads     Tech leads and managers

Message Format:
  Messages are delivered as system reminders to agent sessions.
  Use @agent-name to mention specific agents in messages.

See Also:
  mycel agent send       Send message to single agent
  mycel agent broadcast  Send to all agents
  mycel status           View agents and their channels`,
}

var channelCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new channel",
	Args:  cobra.ExactArgs(1),
	RunE:  runChannelCreate,
}

var channelAddCmd = &cobra.Command{
	Use:   "add <channel> [member...]",
	Short: "Add members to a channel",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runChannelAdd,
}

var channelRemoveCmd = &cobra.Command{
	Use:   "remove <channel> [member]",
	Short: "Remove a member from a channel",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runChannelRemove,
}

var channelSendCmd = &cobra.Command{
	Use:   "send <channel> <message>",
	Short: "Send a message to all channel members",
	Args:  cobra.MinimumNArgs(2),
	RunE:  runChannelSend,
}

var channelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all channels",
	RunE:  runChannelList,
}

var channelDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a channel",
	Args:  cobra.ExactArgs(1),
	RunE:  runChannelDelete,
}

var channelJoinCmd = &cobra.Command{
	Use:   "join <channel>",
	Short: "Join a channel (for agents)",
	Long:  `Add yourself to a channel. This command must be run from within an agent session.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runChannelJoin,
}

var channelLeaveCmd = &cobra.Command{
	Use:   "leave <channel>",
	Short: "Leave a channel (for agents)",
	Long:  `Remove yourself from a channel. This command must be run from within an agent session.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runChannelLeave,
}

var channelHistoryCmd = &cobra.Command{
	Use:   "history <channel>",
	Short: "Show channel message history",
	Long: `Display the history of messages sent to a channel.

Examples:
  mycel channel history eng                       # Last 50 messages (default)
  mycel channel history eng --limit 10            # Last 10 messages
  mycel channel history eng --since 1h            # Messages from last hour
  mycel channel history eng --agent agent-core    # Messages from agent-core only
  mycel channel history eng --from 2026-03-01     # Messages from date
  mycel channel history eng --from 2026-03-01 --to 2026-03-05  # Date range
  mycel channel history eng --limit 20 --offset 20  # Page 2 of 20`,
	Args: cobra.ExactArgs(1),
	RunE: runChannelHistory,
}

var channelReactCmd = &cobra.Command{
	Use:   "react <channel> <message-index> <emoji>",
	Short: "React to a channel message",
	Long: `Add an emoji reaction to a message in a channel.

The message-index is shown in 'mycel channel history' output.
Use common emoji like 👍, 👎, ❤️, 🎉, 👀, 🚀 or any emoji.

Examples:
  mycel channel react engineering 5 👍
  mycel channel react general 0 🎉`,
	Args: cobra.ExactArgs(3),
	RunE: runChannelReact,
}

var channelShowCmd = &cobra.Command{
	Use:   "show <channel>",
	Short: "Show channel details",
	Long: `Display detailed information about a channel including members,
description, and message history statistics.

Examples:
  mycel channel show engineering    # Show engineering channel details
  mycel channel show standup --json # Output as JSON`,
	Args: cobra.ExactArgs(1),
	RunE: runChannelShow,
}

var channelDescCmd = &cobra.Command{
	Use:   "desc <channel> <description>",
	Short: "Set channel description",
	Long:  `Set or update the description for a channel.`,
	Args:  cobra.MinimumNArgs(2),
	RunE:  runChannelDesc,
}

var channelStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show channel overview with activity details",
	Long: `List all channels with detailed status columns.

Columns: Name, Members, Messages, Last Message (preview), Last Activity.

Examples:
  mycel channel status           # Show all channels with details
  mycel channel status --json    # JSON output`,
	RunE: runChannelStatus,
}

var channelEditCmd = &cobra.Command{
	Use:   "edit <channel>",
	Short: "Edit channel description/settings",
	Long: `Edit a channel's description or settings.

Examples:
  mycel channel edit eng --desc "Engineering discussion"`,
	Args: cobra.ExactArgs(1),
	RunE: runChannelEdit,
}

var channelCreateDesc string
var channelEditDesc string

// Channel add/remove --agent flags
var (
	channelAddAgent    string
	channelRemoveAgent string
)

// Channel history flags
var (
	channelHistoryLimit  int
	channelHistoryLast   int
	channelHistoryOffset int
	channelHistorySince  string
	channelHistoryAgent  string
	channelHistoryFrom   string
	channelHistoryTo     string
)

func init() {
	channelCreateCmd.Flags().StringVar(&channelCreateDesc, "desc", "", "Channel description")
	channelEditCmd.Flags().StringVar(&channelEditDesc, "desc", "", "New channel description")

	// --agent flag for add/remove (alternative to positional args)
	channelAddCmd.Flags().StringVar(&channelAddAgent, "agent", "", "Agent to add to channel")
	channelRemoveCmd.Flags().StringVar(&channelRemoveAgent, "agent", "", "Agent to remove from channel")

	channelHistoryCmd.Flags().IntVar(&channelHistoryLimit, "limit", 50, "Maximum number of messages to show")
	channelHistoryCmd.Flags().IntVar(&channelHistoryLast, "last", 0, "Show last N messages (alias for --limit)")
	channelHistoryCmd.Flags().IntVar(&channelHistoryOffset, "offset", 0, "Number of messages to skip")
	channelHistoryCmd.Flags().StringVar(&channelHistorySince, "since", "", "Show messages since duration (e.g., 1h, 30m)")
	channelHistoryCmd.Flags().StringVar(&channelHistoryAgent, "agent", "", "Filter messages by sender agent")
	channelHistoryCmd.Flags().StringVar(&channelHistoryFrom, "from", "", "Show messages from timestamp (RFC3339 or 2006-01-02)")
	channelHistoryCmd.Flags().StringVar(&channelHistoryTo, "to", "", "Show messages until timestamp (RFC3339 or 2006-01-02)")

	// Add shell completion for channel name arguments
	channelAddCmd.ValidArgsFunction = CompleteChannelNames
	channelRemoveCmd.ValidArgsFunction = CompleteChannelNames
	channelSendCmd.ValidArgsFunction = CompleteChannelNames
	channelDeleteCmd.ValidArgsFunction = CompleteChannelNames
	channelJoinCmd.ValidArgsFunction = CompleteChannelNames
	channelLeaveCmd.ValidArgsFunction = CompleteChannelNames
	channelHistoryCmd.ValidArgsFunction = CompleteChannelNames
	channelReactCmd.ValidArgsFunction = CompleteChannelNames
	channelShowCmd.ValidArgsFunction = CompleteChannelNames
	channelDescCmd.ValidArgsFunction = CompleteChannelNames
	channelEditCmd.ValidArgsFunction = CompleteChannelNames

	channelListCmd.Flags().Bool("json", false, "Output as JSON")

	channelCmd.AddCommand(channelCreateCmd)
	channelCmd.AddCommand(channelAddCmd)
	channelCmd.AddCommand(channelRemoveCmd)
	channelCmd.AddCommand(channelSendCmd)
	channelCmd.AddCommand(channelListCmd)
	channelCmd.AddCommand(channelDeleteCmd)
	channelCmd.AddCommand(channelJoinCmd)
	channelCmd.AddCommand(channelLeaveCmd)
	channelCmd.AddCommand(channelHistoryCmd)
	channelCmd.AddCommand(channelReactCmd)
	channelCmd.AddCommand(channelShowCmd)
	channelCmd.AddCommand(channelDescCmd)
	channelCmd.AddCommand(channelStatusCmd)
	channelCmd.AddCommand(channelEditCmd)
	rootCmd.AddCommand(channelCmd)
}

func runChannelList(cmd *cobra.Command, _ []string) error {
	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	channels, err := c.Channels.List(cmd.Context())
	if err != nil {
		return err
	}

	jsonOutput, err := cmd.Flags().GetBool("json")
	if err != nil {
		return err
	}
	if jsonOutput {
		// Build enhanced channel list with member counts and descriptions for TUI
		type ChannelSummary struct {
			Name        string   `json:"name"`
			Description string   `json:"description,omitempty"`
			Members     []string `json:"members"`
			MemberCount int      `json:"member_count"`
		}
		summaries := make([]ChannelSummary, 0, len(channels))
		for _, ch := range channels {
			summaries = append(summaries, ChannelSummary{
				Name:        ch.Name,
				Members:     ch.Members,
				MemberCount: ch.MemberCount,
				Description: ch.Description,
			})
		}
		response := struct {
			Channels []ChannelSummary `json:"channels"`
		}{Channels: summaries}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(response)
	}

	if len(channels) == 0 {
		ui.Warning("No channels defined")
		ui.BlankLine()
		ui.Info("Run 'mycel channel create <name>' to create a channel")
		ui.Info("Or run 'mycel up' to create default channels")
		return nil
	}

	// Use pkg/ui table for consistent formatting
	table := ui.NewTable("CHANNEL", "MEMBERS", "DESCRIPTION")

	for _, ch := range channels {
		memberCount := fmt.Sprintf("(%d)", ch.MemberCount)
		desc := ""
		if ch.Description != "" {
			desc = truncateMessage(ch.Description, 30)
		}
		table.AddRow(ch.Name, memberCount, desc)
	}

	table.Print()
	return nil
}

func runChannelCreate(cmd *cobra.Command, args []string) error {
	name := strings.TrimSpace(args[0])
	if name == "" {
		return fmt.Errorf("channel name cannot be empty")
	}
	// Validate channel name to prevent log injection and special character issues
	if !validIdentifier(name) {
		return fmt.Errorf("channel name %q contains invalid characters (use letters, numbers, dash, underscore)", name)
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	ch, err := c.Channels.Create(cmd.Context(), name, channelCreateDesc)
	if err != nil {
		return err
	}

	if ch.Description != "" {
		fmt.Printf("Created channel %q with description: %s\n", ch.Name, ch.Description)
	} else {
		fmt.Printf("Created channel %q\n", ch.Name)
	}
	return nil
}

func runChannelAdd(cmd *cobra.Command, args []string) error {
	channelName := args[0]
	// Validate channel name to prevent log injection
	if !validIdentifier(channelName) {
		return fmt.Errorf("channel name %q contains invalid characters (use letters, numbers, dash, underscore)", channelName)
	}

	// Collect members from positional args and --agent flag
	members := args[1:]
	if channelAddAgent != "" {
		members = append(members, channelAddAgent)
	}
	if len(members) == 0 {
		return fmt.Errorf("at least one member is required (use positional args or --agent)")
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	added := 0
	for _, member := range members {
		if err := c.Channels.AddMember(cmd.Context(), channelName, member); err != nil {
			fmt.Printf("  Warning: %v\n", err)
			continue
		}
		added++
	}

	fmt.Printf("Added %d member(s) to channel %q\n", added, channelName)
	return nil
}

func runChannelRemove(cmd *cobra.Command, args []string) error {
	channelName := args[0]
	// Validate channel name to prevent log injection
	if !validIdentifier(channelName) {
		return fmt.Errorf("channel name %q contains invalid characters (use letters, numbers, dash, underscore)", channelName)
	}

	// Get member from positional arg or --agent flag
	var member string
	if len(args) > 1 {
		member = args[1]
	} else if channelRemoveAgent != "" {
		member = channelRemoveAgent
	} else {
		return fmt.Errorf("member is required (use positional arg or --agent)")
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	if err := c.Channels.RemoveMember(cmd.Context(), channelName, member); err != nil {
		return err
	}

	fmt.Printf("Removed %q from channel %q\n", member, channelName)
	return nil
}

func runChannelSend(cmd *cobra.Command, args []string) error {
	channelName := args[0]
	// Validate channel name to prevent log injection
	if !validIdentifier(channelName) {
		return fmt.Errorf("channel name %q contains invalid characters (use letters, numbers, dash, underscore)", channelName)
	}
	message := strings.Join(args[1:], " ")

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	sender := getUserSenderCtx(cmd.Context())
	if _, err := c.Channels.Send(cmd.Context(), channelName, sender, message); err != nil {
		return err
	}

	fmt.Printf("Sent message to #%s\n", channelName)
	return nil
}

func runChannelDelete(cmd *cobra.Command, args []string) error {
	name := args[0]
	// Validate channel name to prevent log injection
	if !validIdentifier(name) {
		return fmt.Errorf("channel name %q contains invalid characters (use letters, numbers, dash, underscore)", name)
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	if err := c.Channels.Delete(cmd.Context(), name); err != nil {
		return err
	}

	fmt.Printf("Deleted channel %q\n", name)
	return nil
}

func runChannelJoin(cmd *cobra.Command, args []string) error {
	agentID := os.Getenv("MYCEL_AGENT_ID")
	if agentID == "" {
		return errorAgentNotRunning(fmt.Sprintf("mycel channel join %s", args[0]))
	}

	channelName := args[0]
	// Validate channel name to prevent log injection
	if !validIdentifier(channelName) {
		return fmt.Errorf("channel name %q contains invalid characters (use letters, numbers, dash, underscore)", channelName)
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	if err := c.Channels.AddMember(cmd.Context(), channelName, agentID); err != nil {
		return err
	}

	fmt.Printf("Joined channel %q\n", channelName)
	return nil
}

func runChannelLeave(cmd *cobra.Command, args []string) error {
	agentID := os.Getenv("MYCEL_AGENT_ID")
	if agentID == "" {
		return errorAgentNotRunning(fmt.Sprintf("mycel channel leave %s", args[0]))
	}

	channelName := args[0]
	// Validate channel name to prevent log injection
	if !validIdentifier(channelName) {
		return fmt.Errorf("channel name %q contains invalid characters (use letters, numbers, dash, underscore)", channelName)
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	if err := c.Channels.RemoveMember(cmd.Context(), channelName, agentID); err != nil {
		return err
	}

	fmt.Printf("Left channel %q\n", channelName)
	return nil
}

func runChannelHistory(cmd *cobra.Command, args []string) error {
	channelName := args[0]
	// Validate channel name to prevent log injection
	if !validIdentifier(channelName) {
		return fmt.Errorf("channel name %q contains invalid characters (use letters, numbers, dash, underscore)", channelName)
	}

	// --last overrides --limit when explicitly set
	limit := channelHistoryLimit
	if channelHistoryLast > 0 {
		limit = channelHistoryLast
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	msgs, err := c.Channels.History(cmd.Context(), channelName, limit, channelHistoryOffset, channelHistoryAgent)
	if err != nil {
		return err
	}

	// Filter by --since if provided
	if channelHistorySince != "" {
		cutoff, parseErr := parseSinceDuration(channelHistorySince)
		if parseErr != nil {
			return parseErr
		}
		filtered := msgs[:0]
		for _, msg := range msgs {
			if !msg.CreatedAt.Before(cutoff) {
				filtered = append(filtered, msg)
			}
		}
		msgs = filtered
	}

	// Filter by --from timestamp
	if channelHistoryFrom != "" {
		fromTime, parseErr := parseTimestamp(channelHistoryFrom)
		if parseErr != nil {
			return fmt.Errorf("invalid --from timestamp: %w", parseErr)
		}
		filtered := msgs[:0]
		for _, msg := range msgs {
			if !msg.CreatedAt.Before(fromTime) {
				filtered = append(filtered, msg)
			}
		}
		msgs = filtered
	}

	// Filter by --to timestamp
	if channelHistoryTo != "" {
		toTime, parseErr := parseTimestamp(channelHistoryTo)
		if parseErr != nil {
			return fmt.Errorf("invalid --to timestamp: %w", parseErr)
		}
		filtered := msgs[:0]
		for _, msg := range msgs {
			if msg.CreatedAt.Before(toTime) {
				filtered = append(filtered, msg)
			}
		}
		msgs = filtered
	}

	jsonOutput, err := cmd.Flags().GetBool("json")
	if err != nil {
		return err
	}
	if jsonOutput {
		// Wrap in object for TUI compatibility
		response := struct {
			Channel  string               `json:"channel"`
			Messages []client.MessageInfo `json:"messages"`
		}{Channel: channelName, Messages: msgs}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(response)
	}

	if len(msgs) == 0 {
		fmt.Printf("No message history for channel %q\n", channelName)
		return nil
	}

	fmt.Printf("Message history for #%s:\n", channelName)
	fmt.Println(strings.Repeat("-", 60))
	for i, msg := range msgs {
		timeStr := msg.CreatedAt.Local().Format("2006-01-02 15:04:05")
		if msg.Sender != "" {
			fmt.Printf("[%d] [%s] %s: %s\n", i, timeStr, msg.Sender, msg.Content)
		} else {
			fmt.Printf("[%d] [%s] %s\n", i, timeStr, msg.Content)
		}
		// Show reactions if any
		if len(msg.Reactions) > 0 {
			var reactionStrs []string
			for emoji, users := range msg.Reactions {
				reactionStrs = append(reactionStrs, fmt.Sprintf("%s %d", emoji, len(users)))
			}
			fmt.Printf("    Reactions: %s\n", strings.Join(reactionStrs, " "))
		}
	}

	return nil
}

func runChannelReact(cmd *cobra.Command, args []string) error {
	channelName := args[0]
	// Validate channel name to prevent log injection
	if !validIdentifier(channelName) {
		return fmt.Errorf("channel name %q contains invalid characters (use letters, numbers, dash, underscore)", channelName)
	}
	msgIndex, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("invalid message index %q: %w", args[1], err)
	}
	emoji := args[2]

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	// Fetch history to get the actual message ID at the given index
	msgs, err := c.Channels.History(cmd.Context(), channelName, 1000, 0, "")
	if err != nil {
		return err
	}
	if msgIndex < 1 || msgIndex > len(msgs) {
		return fmt.Errorf("message index %d out of range (1-%d)", msgIndex, len(msgs))
	}
	msgID := int(msgs[msgIndex-1].ID)

	added, err := c.Channels.React(cmd.Context(), channelName, msgID, emoji, getUserSenderCtx(cmd.Context()))
	if err != nil {
		return err
	}

	if added {
		fmt.Printf("Added %s reaction to message %d in #%s\n", emoji, msgIndex, channelName)
	} else {
		fmt.Printf("Removed %s reaction from message %d in #%s\n", emoji, msgIndex, channelName)
	}
	return nil
}

// ChannelInfo represents detailed channel information for JSON output.
type ChannelInfo struct {
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	Members      []string `json:"members"`
	MemberCount  int      `json:"member_count"`
	HistoryCount int      `json:"history_count"`
}

func runChannelShow(cmd *cobra.Command, args []string) error {
	channelName := args[0]
	// Validate channel name to prevent log injection
	if !validIdentifier(channelName) {
		return fmt.Errorf("channel name %q contains invalid characters (use letters, numbers, dash, underscore)", channelName)
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	ch, err := c.Channels.Get(cmd.Context(), channelName)
	if err != nil {
		return fmt.Errorf("channel %q not found (use 'mycel channel list' to see available channels): %w", channelName, err)
	}

	msgs, _ := c.Channels.History(cmd.Context(), channelName, 5, 0, "")

	jsonOutput, err := cmd.Flags().GetBool("json")
	if err != nil {
		return err
	}

	if jsonOutput {
		info := ChannelInfo{
			Name:         ch.Name,
			Description:  ch.Description,
			Members:      ch.Members,
			MemberCount:  ch.MemberCount,
			HistoryCount: ch.MessageCount,
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(info)
	}

	// Text output
	fmt.Printf("Channel: #%s\n", ch.Name)
	fmt.Println(strings.Repeat("-", 40))

	if ch.Description != "" {
		fmt.Printf("Description: %s\n", ch.Description)
	}

	fmt.Printf("Members (%d):\n", ch.MemberCount)
	if len(ch.Members) == 0 {
		fmt.Println("  (none)")
	} else {
		for _, m := range ch.Members {
			fmt.Printf("  • %s\n", m)
		}
	}

	fmt.Printf("\nMessage History: %d messages\n", ch.MessageCount)

	if len(msgs) > 0 {
		fmt.Println("\nRecent Messages (last 5):")
		for _, msg := range msgs {
			content := strings.ReplaceAll(msg.Content, "\n", " ")
			fmt.Printf("  [%s] %s: %s\n", msg.CreatedAt.Local().Format("15:04"), msg.Sender, truncateMessage(content, 50))
		}
	}

	return nil
}

func runChannelDesc(cmd *cobra.Command, args []string) error {
	channelName := strings.TrimSpace(args[0])
	if channelName == "" {
		return fmt.Errorf("channel name cannot be empty")
	}
	// Validate channel name to prevent log injection
	if !validIdentifier(channelName) {
		return fmt.Errorf("channel name %q contains invalid characters (use letters, numbers, dash, underscore)", channelName)
	}

	// Join description from remaining arguments
	description := strings.TrimSpace(strings.Join(args[1:], " "))
	if description == "" {
		return fmt.Errorf("description cannot be empty")
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	if _, err := c.Channels.Update(cmd.Context(), channelName, description); err != nil {
		return fmt.Errorf("failed to set description: %w", err)
	}

	fmt.Printf("Updated description for channel %q: %s\n", channelName, description)
	return nil
}

func runChannelEdit(cmd *cobra.Command, args []string) error {
	channelName := args[0]
	if !validIdentifier(channelName) {
		return fmt.Errorf("channel name %q contains invalid characters (use letters, numbers, dash, underscore)", channelName)
	}

	if channelEditDesc == "" {
		return fmt.Errorf("at least one setting is required (e.g. --desc)")
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	if _, err := c.Channels.Update(cmd.Context(), channelName, channelEditDesc); err != nil {
		return fmt.Errorf("failed to update channel: %w", err)
	}

	fmt.Printf("Updated channel %q\n", channelName)
	return nil
}

func runChannelStatus(cmd *cobra.Command, args []string) error {
	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	channels, err := c.Channels.List(cmd.Context())
	if err != nil {
		return err
	}

	jsonOutput, err := cmd.Flags().GetBool("json")
	if err != nil {
		return err
	}

	type ChannelStatus struct {
		Name         string `json:"name"`
		Description  string `json:"description,omitempty"`
		LastMessage  string `json:"last_message,omitempty"`
		LastSender   string `json:"last_sender,omitempty"`
		LastActivity string `json:"last_activity,omitempty"`
		MemberCount  int    `json:"member_count"`
		MessageCount int    `json:"message_count"`
	}

	statuses := make([]ChannelStatus, 0, len(channels))
	for _, ch := range channels {
		cs := ChannelStatus{
			Name:         ch.Name,
			Description:  ch.Description,
			MemberCount:  ch.MemberCount,
			MessageCount: ch.MessageCount,
		}

		// Fetch last message for this channel
		msgs, histErr := c.Channels.History(cmd.Context(), ch.Name, 1, 0, "")
		if histErr == nil && len(msgs) > 0 {
			last := msgs[0]
			cs.LastSender = last.Sender
			cs.LastMessage = truncateMessage(strings.ReplaceAll(last.Content, "\n", " "), 40)
			cs.LastActivity = last.CreatedAt.UTC().Format(time.RFC3339)
		}

		statuses = append(statuses, cs)
	}

	if jsonOutput {
		response := struct {
			Channels []ChannelStatus `json:"channels"`
		}{Channels: statuses}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(response)
	}

	if len(statuses) == 0 {
		ui.Warning("No channels defined")
		return nil
	}

	table := ui.NewTable("CHANNEL", "MEMBERS", "MESSAGES", "LAST SENDER", "LAST MESSAGE", "LAST ACTIVITY")
	for _, cs := range statuses {
		activity := ""
		if cs.LastActivity != "" {
			if t, parseErr := time.Parse(time.RFC3339, cs.LastActivity); parseErr == nil {
				activity = t.Format("Jan 02 15:04")
			}
		}
		table.AddRow(
			cs.Name,
			fmt.Sprintf("%d", cs.MemberCount),
			fmt.Sprintf("%d", cs.MessageCount),
			cs.LastSender,
			cs.LastMessage,
			activity,
		)
	}
	table.Print()
	return nil
}

// parseTimestamp parses a timestamp string in RFC3339 or date-only format.
func parseTimestamp(s string) (time.Time, error) {
	// Try RFC3339 first
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	// Try date-only format
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("expected RFC3339 (2006-01-02T15:04:05Z) or date (2006-01-02), got %q", s)
}

// defaultNickname is the fallback sender name when no user nickname is configured.
const defaultNickname = "@bc"

// getUserSenderCtx returns the sender identity for channel messages.
// If running as an agent, returns MYCEL_AGENT_ID.
// Otherwise, queries the daemon settings API for user.nickname.
func getUserSenderCtx(ctx context.Context) string {
	// Check if running as an agent
	if agentID := os.Getenv("MYCEL_AGENT_ID"); agentID != "" {
		return agentID
	}

	// Try to get nickname from daemon settings API
	c, err := newDaemonClient(ctx)
	if err == nil {
		raw, settingsErr := c.Settings.Get(ctx)
		if settingsErr == nil {
			var settings struct {
				User struct {
					Nickname string `json:"nickname"`
				} `json:"user"`
			}
			if jsonErr := json.Unmarshal(raw, &settings); jsonErr == nil && settings.User.Nickname != "" {
				return settings.User.Nickname
			}
		}
	}

	// Fallback to default nickname
	return defaultNickname
}
