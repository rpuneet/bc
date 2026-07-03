## mycel channel

Manage communication channels

### Synopsis

Manage channels for broadcasting messages to groups of agents.

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

Agent Commands (require BC_AGENT_ID):
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
  mycel status           View agents and their channels

### Options

```
  -h, --help   help for channel
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel](mycel.md)	 - A simpler, more controllable agent orchestrator
* [mycel channel add](mycel_channel_add.md)	 - Add members to a channel
* [mycel channel create](mycel_channel_create.md)	 - Create a new channel
* [mycel channel delete](mycel_channel_delete.md)	 - Delete a channel
* [mycel channel desc](mycel_channel_desc.md)	 - Set channel description
* [mycel channel edit](mycel_channel_edit.md)	 - Edit channel description/settings
* [mycel channel history](mycel_channel_history.md)	 - Show channel message history
* [mycel channel join](mycel_channel_join.md)	 - Join a channel (for agents)
* [mycel channel leave](mycel_channel_leave.md)	 - Leave a channel (for agents)
* [mycel channel list](mycel_channel_list.md)	 - List all channels
* [mycel channel react](mycel_channel_react.md)	 - React to a channel message
* [mycel channel remove](mycel_channel_remove.md)	 - Remove a member from a channel
* [mycel channel send](mycel_channel_send.md)	 - Send a message to all channel members
* [mycel channel show](mycel_channel_show.md)	 - Show channel details
* [mycel channel status](mycel_channel_status.md)	 - Show channel overview with activity details

