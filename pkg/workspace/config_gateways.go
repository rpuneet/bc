package workspace

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GatewaysConfig configures external messaging platform integrations.
//
// JSON keys follow a "platform" or "platform:label" convention. Plain
// "telegram" is a single Telegram bot (backward compat). Keys like
// "telegram:trade_research" register additional bots — parsed into the
// Telegrams map keyed by label.
type GatewaysConfig struct {
	// Telegram is the single default Telegram bot (key "telegram").
	// Deprecated: prefer Telegrams map for multi-bot setups.
	Telegram *TelegramGatewayConfig `json:"-"`
	Discord  *DiscordGatewayConfig  `json:"discord,omitempty"`
	Slack    *SlackGatewayConfig    `json:"slack,omitempty"`
	// Telegrams holds zero or more Telegram bots keyed by label.
	// A plain "telegram" key is stored under label "".
	Telegrams map[string]*TelegramGatewayConfig `json:"-"`
	// GitHubs holds zero or more GitHub webhook configs keyed by label.
	// A plain "github" key is stored under label "".
	GitHubs map[string]*GitHubGatewayConfig `json:"-"`
	// Webhooks holds zero or more generic webhook configs keyed by label.
	// A plain "webhook" key is stored under label "".
	Webhooks map[string]*WebhookGatewayConfig `json:"-"`
	// RSSFeeds holds zero or more RSS/Atom feed configs keyed by label.
	// A plain "rss" key is stored under label "".
	RSSFeeds map[string]*RSSGatewayConfig `json:"-"`
	// GitLabs holds zero or more GitLab webhook configs keyed by label.
	GitLabs map[string]*GitLabGatewayConfig `json:"-"`
	// Jiras holds zero or more Jira webhook configs keyed by label.
	Jiras map[string]*JiraGatewayConfig `json:"-"`
	// Linears holds zero or more Linear webhook configs keyed by label.
	Linears map[string]*LinearGatewayConfig `json:"-"`
	// Sentries holds zero or more Sentry webhook configs keyed by label.
	Sentries map[string]*SentryGatewayConfig `json:"-"`
	// Stripes holds zero or more Stripe webhook configs keyed by label.
	Stripes map[string]*StripeGatewayConfig `json:"-"`
	// Bitbuckets holds zero or more Bitbucket webhook configs keyed by label.
	Bitbuckets map[string]*BitbucketGatewayConfig `json:"-"`
	// PagerDuties holds zero or more PagerDuty webhook configs keyed by label.
	PagerDuties map[string]*PagerDutyGatewayConfig `json:"-"`
	// Datadogs holds zero or more Datadog webhook configs keyed by label.
	Datadogs map[string]*DatadogGatewayConfig `json:"-"`
	// Grafanas holds zero or more Grafana webhook configs keyed by label.
	Grafanas map[string]*GrafanaGatewayConfig `json:"-"`
	// Vercels holds zero or more Vercel webhook configs keyed by label.
	Vercels map[string]*VercelGatewayConfig `json:"-"`
	// Netlifys holds zero or more Netlify webhook configs keyed by label.
	Netlifys map[string]*NetlifyGatewayConfig `json:"-"`
	// Notions holds zero or more Notion poll configs keyed by label.
	Notions map[string]*NotionGatewayConfig `json:"-"`
	// WhatsApps holds zero or more WhatsApp webhook configs keyed by label.
	WhatsApps map[string]*WhatsAppGatewayConfig `json:"-"`
	// Signals holds zero or more Signal poll configs keyed by label.
	Signals map[string]*SignalGatewayConfig `json:"-"`
	// Matrices holds zero or more Matrix poll configs keyed by label.
	Matrices map[string]*MatrixGatewayConfig `json:"-"`
	// MSTeams holds zero or more MS Teams webhook configs keyed by label.
	MSTeams map[string]*MSTeamsGatewayConfig `json:"-"`
	// GoogleChats holds zero or more Google Chat webhook configs keyed by label.
	GoogleChats map[string]*GoogleChatGatewayConfig `json:"-"`
	// Lines holds zero or more LINE webhook configs keyed by label.
	Lines map[string]*LineGatewayConfig `json:"-"`
	// Feishus holds zero or more Feishu webhook configs keyed by label.
	Feishus map[string]*FeishuGatewayConfig `json:"-"`
	// Mattermosts holds zero or more Mattermost webhook configs keyed by label.
	Mattermosts map[string]*MattermostGatewayConfig `json:"-"`
	// IRCs holds zero or more IRC socket configs keyed by label.
	IRCs map[string]*IRCGatewayConfig `json:"-"`
	// Nostrs holds zero or more Nostr socket configs keyed by label.
	Nostrs map[string]*NostrGatewayConfig `json:"-"`
	// Twitches holds zero or more Twitch webhook configs keyed by label.
	Twitches map[string]*TwitchGatewayConfig `json:"-"`
	// IMessages holds zero or more iMessage poll configs keyed by label.
	IMessages map[string]*IMessageGatewayConfig `json:"-"`
	// MQTTs holds zero or more MQTT socket configs keyed by label.
	MQTTs map[string]*MQTTGatewayConfig `json:"-"`
	// Twitters holds zero or more Twitter poll configs keyed by label.
	Twitters map[string]*TwitterGatewayConfig `json:"-"`
	// Reddits holds zero or more Reddit poll configs keyed by label.
	Reddits map[string]*RedditGatewayConfig `json:"-"`
	// HomeAssistants holds zero or more Home Assistant socket configs keyed by label.
	HomeAssistants map[string]*HomeAssistantGatewayConfig `json:"-"`
}

// UnmarshalJSON parses gateway config, routing "telegram:*" keys into the
// Telegrams map and keeping Discord/Slack on their typed fields.
func (g *GatewaysConfig) UnmarshalJSON(data []byte) error {
	// Decode known typed fields via an alias to avoid recursion.
	type Alias struct {
		Discord *DiscordGatewayConfig `json:"discord,omitempty"`
		Slack   *SlackGatewayConfig   `json:"slack,omitempty"`
	}
	var alias Alias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	g.Discord = alias.Discord
	g.Slack = alias.Slack

	// Decode the full map to pick up telegram keys.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	g.Telegrams = make(map[string]*TelegramGatewayConfig)
	g.GitHubs = make(map[string]*GitHubGatewayConfig)
	g.Webhooks = make(map[string]*WebhookGatewayConfig)
	g.RSSFeeds = make(map[string]*RSSGatewayConfig)
	g.GitLabs = make(map[string]*GitLabGatewayConfig)
	g.Jiras = make(map[string]*JiraGatewayConfig)
	g.Linears = make(map[string]*LinearGatewayConfig)
	g.Sentries = make(map[string]*SentryGatewayConfig)
	g.Stripes = make(map[string]*StripeGatewayConfig)
	g.Bitbuckets = make(map[string]*BitbucketGatewayConfig)
	g.PagerDuties = make(map[string]*PagerDutyGatewayConfig)
	g.Datadogs = make(map[string]*DatadogGatewayConfig)
	g.Grafanas = make(map[string]*GrafanaGatewayConfig)
	g.Vercels = make(map[string]*VercelGatewayConfig)
	g.Netlifys = make(map[string]*NetlifyGatewayConfig)
	g.Notions = make(map[string]*NotionGatewayConfig)
	g.WhatsApps = make(map[string]*WhatsAppGatewayConfig)
	g.Signals = make(map[string]*SignalGatewayConfig)
	g.Matrices = make(map[string]*MatrixGatewayConfig)
	g.MSTeams = make(map[string]*MSTeamsGatewayConfig)
	g.GoogleChats = make(map[string]*GoogleChatGatewayConfig)
	g.Lines = make(map[string]*LineGatewayConfig)
	g.Feishus = make(map[string]*FeishuGatewayConfig)
	g.Mattermosts = make(map[string]*MattermostGatewayConfig)
	g.IRCs = make(map[string]*IRCGatewayConfig)
	g.Nostrs = make(map[string]*NostrGatewayConfig)
	g.Twitches = make(map[string]*TwitchGatewayConfig)
	g.IMessages = make(map[string]*IMessageGatewayConfig)
	g.MQTTs = make(map[string]*MQTTGatewayConfig)
	g.Twitters = make(map[string]*TwitterGatewayConfig)
	g.Reddits = make(map[string]*RedditGatewayConfig)
	g.HomeAssistants = make(map[string]*HomeAssistantGatewayConfig)
	for key, val := range raw {
		switch {
		case key == "telegram":
			var tc TelegramGatewayConfig
			if err := json.Unmarshal(val, &tc); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Telegram = &tc
			g.Telegrams[""] = &tc
		case strings.HasPrefix(key, "telegram:"):
			label := strings.TrimPrefix(key, "telegram:")
			var tc TelegramGatewayConfig
			if err := json.Unmarshal(val, &tc); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Telegrams[label] = &tc
		case key == "github":
			var gc GitHubGatewayConfig
			if err := json.Unmarshal(val, &gc); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.GitHubs[""] = &gc
		case strings.HasPrefix(key, "github:"):
			label := strings.TrimPrefix(key, "github:")
			var gc GitHubGatewayConfig
			if err := json.Unmarshal(val, &gc); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.GitHubs[label] = &gc
		case key == "webhook":
			var wc WebhookGatewayConfig
			if err := json.Unmarshal(val, &wc); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Webhooks[""] = &wc
		case strings.HasPrefix(key, "webhook:"):
			label := strings.TrimPrefix(key, "webhook:")
			var wc WebhookGatewayConfig
			if err := json.Unmarshal(val, &wc); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Webhooks[label] = &wc
		case key == "rss":
			var rc RSSGatewayConfig
			if err := json.Unmarshal(val, &rc); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.RSSFeeds[""] = &rc
		case strings.HasPrefix(key, "rss:"):
			label := strings.TrimPrefix(key, "rss:")
			var rc RSSGatewayConfig
			if err := json.Unmarshal(val, &rc); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.RSSFeeds[label] = &rc
		case key == "gitlab" || strings.HasPrefix(key, "gitlab:"):
			label := strings.TrimPrefix(key, "gitlab:")
			if key == "gitlab" {
				label = ""
			}
			var c GitLabGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.GitLabs[label] = &c
		case key == "jira" || strings.HasPrefix(key, "jira:"):
			label := strings.TrimPrefix(key, "jira:")
			if key == "jira" {
				label = ""
			}
			var c JiraGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Jiras[label] = &c
		case key == "linear" || strings.HasPrefix(key, "linear:"):
			label := strings.TrimPrefix(key, "linear:")
			if key == "linear" {
				label = ""
			}
			var c LinearGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Linears[label] = &c
		case key == "sentry" || strings.HasPrefix(key, "sentry:"):
			label := strings.TrimPrefix(key, "sentry:")
			if key == "sentry" {
				label = ""
			}
			var c SentryGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Sentries[label] = &c
		case key == "stripe" || strings.HasPrefix(key, "stripe:"):
			label := strings.TrimPrefix(key, "stripe:")
			if key == "stripe" {
				label = ""
			}
			var c StripeGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Stripes[label] = &c
		case key == "bitbucket" || strings.HasPrefix(key, "bitbucket:"):
			label := strings.TrimPrefix(key, "bitbucket:")
			if key == "bitbucket" {
				label = ""
			}
			var c BitbucketGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Bitbuckets[label] = &c
		case key == "pagerduty" || strings.HasPrefix(key, "pagerduty:"):
			label := strings.TrimPrefix(key, "pagerduty:")
			if key == "pagerduty" {
				label = ""
			}
			var c PagerDutyGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.PagerDuties[label] = &c
		case key == "datadog" || strings.HasPrefix(key, "datadog:"):
			label := strings.TrimPrefix(key, "datadog:")
			if key == "datadog" {
				label = ""
			}
			var c DatadogGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Datadogs[label] = &c
		case key == "grafana" || strings.HasPrefix(key, "grafana:"):
			label := strings.TrimPrefix(key, "grafana:")
			if key == "grafana" {
				label = ""
			}
			var c GrafanaGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Grafanas[label] = &c
		case key == "vercel" || strings.HasPrefix(key, "vercel:"):
			label := strings.TrimPrefix(key, "vercel:")
			if key == "vercel" {
				label = ""
			}
			var c VercelGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Vercels[label] = &c
		case key == "netlify" || strings.HasPrefix(key, "netlify:"):
			label := strings.TrimPrefix(key, "netlify:")
			if key == "netlify" {
				label = ""
			}
			var c NetlifyGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Netlifys[label] = &c
		case key == "notion" || strings.HasPrefix(key, "notion:"):
			label := strings.TrimPrefix(key, "notion:")
			if key == "notion" {
				label = ""
			}
			var c NotionGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Notions[label] = &c
		case key == "whatsapp" || strings.HasPrefix(key, "whatsapp:"):
			label := strings.TrimPrefix(key, "whatsapp:")
			if key == "whatsapp" {
				label = ""
			}
			var c WhatsAppGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.WhatsApps[label] = &c
		case key == "signal" || strings.HasPrefix(key, "signal:"):
			label := strings.TrimPrefix(key, "signal:")
			if key == "signal" {
				label = ""
			}
			var c SignalGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Signals[label] = &c
		case key == "matrix" || strings.HasPrefix(key, "matrix:"):
			label := strings.TrimPrefix(key, "matrix:")
			if key == "matrix" {
				label = ""
			}
			var c MatrixGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Matrices[label] = &c
		case key == "msteams" || strings.HasPrefix(key, "msteams:"):
			label := strings.TrimPrefix(key, "msteams:")
			if key == "msteams" {
				label = ""
			}
			var c MSTeamsGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.MSTeams[label] = &c
		case key == "googlechat" || strings.HasPrefix(key, "googlechat:"):
			label := strings.TrimPrefix(key, "googlechat:")
			if key == "googlechat" {
				label = ""
			}
			var c GoogleChatGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.GoogleChats[label] = &c
		case key == "line" || strings.HasPrefix(key, "line:"):
			label := strings.TrimPrefix(key, "line:")
			if key == "line" {
				label = ""
			}
			var c LineGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Lines[label] = &c
		case key == "feishu" || strings.HasPrefix(key, "feishu:"):
			label := strings.TrimPrefix(key, "feishu:")
			if key == "feishu" {
				label = ""
			}
			var c FeishuGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Feishus[label] = &c
		case key == "mattermost" || strings.HasPrefix(key, "mattermost:"):
			label := strings.TrimPrefix(key, "mattermost:")
			if key == "mattermost" {
				label = ""
			}
			var c MattermostGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Mattermosts[label] = &c
		case key == "irc" || strings.HasPrefix(key, "irc:"):
			label := strings.TrimPrefix(key, "irc:")
			if key == "irc" {
				label = ""
			}
			var c IRCGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.IRCs[label] = &c
		case key == "nostr" || strings.HasPrefix(key, "nostr:"):
			label := strings.TrimPrefix(key, "nostr:")
			if key == "nostr" {
				label = ""
			}
			var c NostrGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Nostrs[label] = &c
		case key == "twitch" || strings.HasPrefix(key, "twitch:"):
			label := strings.TrimPrefix(key, "twitch:")
			if key == "twitch" {
				label = ""
			}
			var c TwitchGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Twitches[label] = &c
		case key == "imessage" || strings.HasPrefix(key, "imessage:"):
			label := strings.TrimPrefix(key, "imessage:")
			if key == "imessage" {
				label = ""
			}
			var c IMessageGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.IMessages[label] = &c
		case key == "mqtt" || strings.HasPrefix(key, "mqtt:"):
			label := strings.TrimPrefix(key, "mqtt:")
			if key == "mqtt" {
				label = ""
			}
			var c MQTTGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.MQTTs[label] = &c
		case key == "twitter" || strings.HasPrefix(key, "twitter:"):
			label := strings.TrimPrefix(key, "twitter:")
			if key == "twitter" {
				label = ""
			}
			var c TwitterGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Twitters[label] = &c
		case key == "reddit" || strings.HasPrefix(key, "reddit:"):
			label := strings.TrimPrefix(key, "reddit:")
			if key == "reddit" {
				label = ""
			}
			var c RedditGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Reddits[label] = &c
		case key == "homeassistant" || strings.HasPrefix(key, "homeassistant:"):
			label := strings.TrimPrefix(key, "homeassistant:")
			if key == "homeassistant" {
				label = ""
			}
			var c HomeAssistantGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.HomeAssistants[label] = &c
		}
	}
	return nil
}

// MarshalJSON serializes the gateway config, emitting "telegram:label"
// keys for each entry in Telegrams.
func (g GatewaysConfig) MarshalJSON() ([]byte, error) {
	m := make(map[string]any)
	for label, tc := range g.Telegrams {
		if label == "" {
			m["telegram"] = tc
		} else {
			m["telegram:"+label] = tc
		}
	}
	// Backward compat: if Telegram is set but not in Telegrams, emit it.
	if g.Telegram != nil {
		if _, ok := g.Telegrams[""]; !ok {
			m["telegram"] = g.Telegram
		}
	}
	for label, gc := range g.GitHubs {
		if label == "" {
			m["github"] = gc
		} else {
			m["github:"+label] = gc
		}
	}
	for label, wc := range g.Webhooks {
		if label == "" {
			m["webhook"] = wc
		} else {
			m["webhook:"+label] = wc
		}
	}
	for label, rc := range g.RSSFeeds {
		if label == "" {
			m["rss"] = rc
		} else {
			m["rss:"+label] = rc
		}
	}
	for label, c := range g.GitLabs {
		if label == "" {
			m["gitlab"] = c
		} else {
			m["gitlab:"+label] = c
		}
	}
	for label, c := range g.Jiras {
		if label == "" {
			m["jira"] = c
		} else {
			m["jira:"+label] = c
		}
	}
	for label, c := range g.Linears {
		if label == "" {
			m["linear"] = c
		} else {
			m["linear:"+label] = c
		}
	}
	for label, c := range g.Sentries {
		if label == "" {
			m["sentry"] = c
		} else {
			m["sentry:"+label] = c
		}
	}
	for label, c := range g.Stripes {
		if label == "" {
			m["stripe"] = c
		} else {
			m["stripe:"+label] = c
		}
	}
	for label, c := range g.Bitbuckets {
		if label == "" {
			m["bitbucket"] = c
		} else {
			m["bitbucket:"+label] = c
		}
	}
	for label, c := range g.PagerDuties {
		if label == "" {
			m["pagerduty"] = c
		} else {
			m["pagerduty:"+label] = c
		}
	}
	for label, c := range g.Datadogs {
		if label == "" {
			m["datadog"] = c
		} else {
			m["datadog:"+label] = c
		}
	}
	for label, c := range g.Grafanas {
		if label == "" {
			m["grafana"] = c
		} else {
			m["grafana:"+label] = c
		}
	}
	for label, c := range g.Vercels {
		if label == "" {
			m["vercel"] = c
		} else {
			m["vercel:"+label] = c
		}
	}
	for label, c := range g.Netlifys {
		if label == "" {
			m["netlify"] = c
		} else {
			m["netlify:"+label] = c
		}
	}
	for label, c := range g.Notions {
		if label == "" {
			m["notion"] = c
		} else {
			m["notion:"+label] = c
		}
	}
	for label, c := range g.WhatsApps {
		if label == "" {
			m["whatsapp"] = c
		} else {
			m["whatsapp:"+label] = c
		}
	}
	for label, c := range g.Signals {
		if label == "" {
			m["signal"] = c
		} else {
			m["signal:"+label] = c
		}
	}
	for label, c := range g.Matrices {
		if label == "" {
			m["matrix"] = c
		} else {
			m["matrix:"+label] = c
		}
	}
	for label, c := range g.MSTeams {
		if label == "" {
			m["msteams"] = c
		} else {
			m["msteams:"+label] = c
		}
	}
	for label, c := range g.GoogleChats {
		if label == "" {
			m["googlechat"] = c
		} else {
			m["googlechat:"+label] = c
		}
	}
	for label, c := range g.Lines {
		if label == "" {
			m["line"] = c
		} else {
			m["line:"+label] = c
		}
	}
	for label, c := range g.Feishus {
		if label == "" {
			m["feishu"] = c
		} else {
			m["feishu:"+label] = c
		}
	}
	for label, c := range g.Mattermosts {
		if label == "" {
			m["mattermost"] = c
		} else {
			m["mattermost:"+label] = c
		}
	}
	for label, c := range g.IRCs {
		if label == "" {
			m["irc"] = c
		} else {
			m["irc:"+label] = c
		}
	}
	for label, c := range g.Nostrs {
		if label == "" {
			m["nostr"] = c
		} else {
			m["nostr:"+label] = c
		}
	}
	for label, c := range g.Twitches {
		if label == "" {
			m["twitch"] = c
		} else {
			m["twitch:"+label] = c
		}
	}
	for label, c := range g.IMessages {
		if label == "" {
			m["imessage"] = c
		} else {
			m["imessage:"+label] = c
		}
	}
	for label, c := range g.MQTTs {
		if label == "" {
			m["mqtt"] = c
		} else {
			m["mqtt:"+label] = c
		}
	}
	for label, c := range g.Twitters {
		if label == "" {
			m["twitter"] = c
		} else {
			m["twitter:"+label] = c
		}
	}
	for label, c := range g.Reddits {
		if label == "" {
			m["reddit"] = c
		} else {
			m["reddit:"+label] = c
		}
	}
	for label, c := range g.HomeAssistants {
		if label == "" {
			m["homeassistant"] = c
		} else {
			m["homeassistant:"+label] = c
		}
	}
	if g.Discord != nil {
		m["discord"] = g.Discord
	}
	if g.Slack != nil {
		m["slack"] = g.Slack
	}
	return json.Marshal(m)
}

// TelegramGatewayConfig configures the Telegram gateway adapter.
type TelegramGatewayConfig struct {
	BotToken string `json:"bot_token"`
	Mode     string `json:"mode"`
	Enabled  bool   `json:"enabled"`
}

// DiscordGatewayConfig configures the Discord gateway adapter.
type DiscordGatewayConfig struct {
	BotToken string `json:"bot_token"`
	Enabled  bool   `json:"enabled"`
}

// SlackGatewayConfig configures the Slack gateway adapter.
type SlackGatewayConfig struct {
	BotToken string `json:"bot_token"`
	AppToken string `json:"app_token"`
	Mode     string `json:"mode"`
	Enabled  bool   `json:"enabled"`
}

// GitHubGatewayConfig configures the GitHub webhook gateway adapter.
type GitHubGatewayConfig struct {
	Secret  string `json:"secret"`
	Enabled bool   `json:"enabled"`
}

// WebhookGatewayConfig configures a generic webhook gateway adapter.
type WebhookGatewayConfig struct {
	Secret  string `json:"secret,omitempty"`
	Enabled bool   `json:"enabled"`
}

// RSSGatewayConfig configures an RSS/Atom feed poll adapter.
type RSSGatewayConfig struct {
	URL      string `json:"url"`
	Interval int    `json:"interval"` // seconds, default 300
	Enabled  bool   `json:"enabled"`
}

// GitLabGatewayConfig configures the GitLab webhook gateway adapter.
type GitLabGatewayConfig struct {
	Token   string `json:"token"`
	Enabled bool   `json:"enabled"`
}

// JiraGatewayConfig configures the Jira webhook gateway adapter.
type JiraGatewayConfig struct {
	Secret  string `json:"secret,omitempty"`
	Enabled bool   `json:"enabled"`
}

// LinearGatewayConfig configures the Linear webhook gateway adapter.
type LinearGatewayConfig struct {
	Secret  string `json:"secret"`
	Enabled bool   `json:"enabled"`
}

// SentryGatewayConfig configures the Sentry webhook gateway adapter.
type SentryGatewayConfig struct {
	Secret  string `json:"secret"`
	Enabled bool   `json:"enabled"`
}

// StripeGatewayConfig configures the Stripe webhook gateway adapter.
type StripeGatewayConfig struct {
	Secret  string `json:"secret"`
	Enabled bool   `json:"enabled"`
}

// BitbucketGatewayConfig configures the Bitbucket webhook gateway adapter.
type BitbucketGatewayConfig struct {
	Secret  string `json:"secret,omitempty"`
	Enabled bool   `json:"enabled"`
}

// PagerDutyGatewayConfig configures the PagerDuty webhook gateway adapter.
type PagerDutyGatewayConfig struct {
	Secret  string `json:"secret,omitempty"`
	Enabled bool   `json:"enabled"`
}

// DatadogGatewayConfig configures the Datadog webhook gateway adapter.
type DatadogGatewayConfig struct {
	Secret  string `json:"secret,omitempty"`
	Enabled bool   `json:"enabled"`
}

// GrafanaGatewayConfig configures the Grafana webhook gateway adapter.
type GrafanaGatewayConfig struct {
	Token   string `json:"token,omitempty"`
	Enabled bool   `json:"enabled"`
}

// VercelGatewayConfig configures the Vercel webhook gateway adapter.
type VercelGatewayConfig struct {
	Secret  string `json:"secret,omitempty"`
	Enabled bool   `json:"enabled"`
}

// NetlifyGatewayConfig configures the Netlify webhook gateway adapter.
type NetlifyGatewayConfig struct {
	Secret  string `json:"secret,omitempty"`
	Enabled bool   `json:"enabled"`
}

// NotionGatewayConfig configures the Notion poll gateway adapter.
type NotionGatewayConfig struct {
	Token    string `json:"token"`
	Interval int    `json:"interval"` // seconds, default 300
	Enabled  bool   `json:"enabled"`
}

// WhatsAppGatewayConfig configures the WhatsApp (Meta Cloud API) webhook adapter.
type WhatsAppGatewayConfig struct {
	VerifyToken         string `json:"verify_token"`
	Enabled             bool   `json:"enabled"`
	IncludeSelfMessages bool   `json:"include_self_messages"`
}

// SignalGatewayConfig configures the Signal (signal-cli REST) poll adapter.
type SignalGatewayConfig struct {
	APIURL   string `json:"api_url"`
	Interval int    `json:"interval"` // seconds, default 10
	Enabled  bool   `json:"enabled"`
}

// MatrixGatewayConfig configures the Matrix client-server poll adapter.
type MatrixGatewayConfig struct {
	Homeserver string `json:"homeserver"`
	Token      string `json:"token"`
	Interval   int    `json:"interval"` // seconds, default 10
	Enabled    bool   `json:"enabled"`
}

// MSTeamsGatewayConfig configures the Microsoft Teams Bot Framework webhook adapter.
type MSTeamsGatewayConfig struct {
	Secret  string `json:"secret,omitempty"`
	Enabled bool   `json:"enabled"`
}

// GoogleChatGatewayConfig configures the Google Chat webhook adapter.
type GoogleChatGatewayConfig struct {
	Secret  string `json:"secret,omitempty"`
	Enabled bool   `json:"enabled"`
}

// LineGatewayConfig configures the LINE Messaging API webhook adapter.
type LineGatewayConfig struct {
	Secret  string `json:"secret"`
	Enabled bool   `json:"enabled"`
}

// FeishuGatewayConfig configures the Feishu/Lark event subscription webhook adapter.
type FeishuGatewayConfig struct {
	Secret  string `json:"secret,omitempty"`
	Enabled bool   `json:"enabled"`
}

// MattermostGatewayConfig configures the Mattermost outgoing webhook adapter.
type MattermostGatewayConfig struct {
	URL     string `json:"url,omitempty"`
	Token   string `json:"token,omitempty"`
	Enabled bool   `json:"enabled"`
}

// IRCGatewayConfig configures the IRC socket adapter.
type IRCGatewayConfig struct {
	Server   string   `json:"server"`
	Channels []string `json:"channels,omitempty"`
	Enabled  bool     `json:"enabled"`
}

// NostrGatewayConfig configures the Nostr relay WebSocket adapter (placeholder).
type NostrGatewayConfig struct {
	RelayURL string `json:"relay_url"`
	Enabled  bool   `json:"enabled"`
}

// TwitchGatewayConfig configures the Twitch EventSub webhook adapter.
type TwitchGatewayConfig struct {
	Secret  string `json:"secret"`
	Enabled bool   `json:"enabled"`
}

// IMessageGatewayConfig configures the iMessage (BlueBubbles) poll adapter.
type IMessageGatewayConfig struct {
	APIURL   string `json:"api_url"`
	Password string `json:"password"`
	Interval int    `json:"interval"` // seconds, default 10
	Enabled  bool   `json:"enabled"`
}

// MQTTGatewayConfig configures the MQTT socket adapter (placeholder).
type MQTTGatewayConfig struct {
	BrokerURL string `json:"broker_url"`
	Topic     string `json:"topic,omitempty"`
	Enabled   bool   `json:"enabled"`
}

// TwitterGatewayConfig configures the Twitter API v2 poll adapter.
type TwitterGatewayConfig struct {
	BearerToken string `json:"bearer_token"`
	UserID      string `json:"user_id"`
	Interval    int    `json:"interval"` // seconds, default 60
	Enabled     bool   `json:"enabled"`
}

// RedditGatewayConfig configures the Reddit API poll adapter.
type RedditGatewayConfig struct {
	Subreddit   string `json:"subreddit"`
	BearerToken string `json:"bearer_token"`
	Interval    int    `json:"interval"` // seconds, default 60
	Enabled     bool   `json:"enabled"`
}

// HomeAssistantGatewayConfig configures the Home Assistant WebSocket adapter (placeholder).
type HomeAssistantGatewayConfig struct {
	URL     string `json:"url"`
	Token   string `json:"token"`
	Enabled bool   `json:"enabled"`
}

// MergeGatewaysPatch deep-merges raw gateway JSON into dst. Each key in
// the patch is applied independently so a patch containing only
// {"discord": {...}} does not wipe existing Slack/Telegram config.
// Shared by the settings PATCH handler and the config overlay (#3239).
func MergeGatewaysPatch(dst *GatewaysConfig, raw json.RawMessage) error {
	// Marshal the existing config to get the current state as a map.
	existing, err := json.Marshal(dst)
	if err != nil {
		return err
	}
	var base map[string]json.RawMessage
	if unmarshalErr := json.Unmarshal(existing, &base); unmarshalErr != nil {
		return unmarshalErr
	}

	// Parse the incoming patch as a map.
	var patch map[string]json.RawMessage
	if unmarshalErr := json.Unmarshal(raw, &patch); unmarshalErr != nil {
		return unmarshalErr
	}

	// Merge: patch keys overwrite base keys, base keys not in patch are kept.
	for k, v := range patch {
		base[k] = v
	}

	// Re-serialize the merged map and unmarshal into the destination.
	merged, err := json.Marshal(base)
	if err != nil {
		return err
	}
	return json.Unmarshal(merged, dst)
}
