package workspace

import (
	"encoding/json"
	"testing"
)

// TestParseConfigWithPerformance tests parsing [performance] section from TOML (#1013)

// TestParseConfigWithTUI tests parsing [tui] section from TOML (#1022)

// TestConfigSaveAndLoadPerformance tests save/load round-trip for performance config (#1013)

func TestGatewaysConfigUnmarshalMultiTelegram(t *testing.T) {
	input := `{
		"telegram:trade": {"bot_token": "tok1", "mode": "polling", "enabled": true},
		"telegram:gateway": {"bot_token": "tok2", "mode": "polling", "enabled": true},
		"slack": {"bot_token": "xoxb", "app_token": "xapp", "mode": "socket", "enabled": true}
	}`

	var gw GatewaysConfig
	if err := json.Unmarshal([]byte(input), &gw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if gw.Telegram != nil {
		t.Error("expected Telegram (plain) to be nil")
	}
	if len(gw.Telegrams) != 2 {
		t.Fatalf("expected 2 Telegrams entries, got %d", len(gw.Telegrams))
	}
	if gw.Telegrams["trade"].BotToken != "tok1" {
		t.Errorf("expected tok1, got %s", gw.Telegrams["trade"].BotToken)
	}
	if gw.Telegrams["gateway"].BotToken != "tok2" {
		t.Errorf("expected tok2, got %s", gw.Telegrams["gateway"].BotToken)
	}
	if gw.Slack == nil || gw.Slack.BotToken != "xoxb" {
		t.Error("Slack not parsed correctly")
	}
}

func TestGatewaysConfigUnmarshalPlainTelegram(t *testing.T) {
	input := `{
		"telegram": {"bot_token": "tok1", "mode": "polling", "enabled": true}
	}`

	var gw GatewaysConfig
	if err := json.Unmarshal([]byte(input), &gw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if gw.Telegram == nil {
		t.Fatal("expected Telegram (plain) to be set")
	}
	if gw.Telegram.BotToken != "tok1" {
		t.Errorf("expected tok1, got %s", gw.Telegram.BotToken)
	}
	// Plain "telegram" should also appear in Telegrams map under empty label.
	if gw.Telegrams[""] == nil || gw.Telegrams[""].BotToken != "tok1" {
		t.Error("plain telegram not in Telegrams map")
	}
}

func TestGatewaysConfigMarshalRoundTrip(t *testing.T) {
	gw := GatewaysConfig{
		Telegrams: map[string]*TelegramGatewayConfig{
			"trade":   {BotToken: "tok1", Mode: "polling", Enabled: true},
			"gateway": {BotToken: "tok2", Mode: "polling", Enabled: true},
		},
		Slack: &SlackGatewayConfig{BotToken: "xoxb", AppToken: "xapp", Mode: "socket", Enabled: true},
	}

	data, err := json.Marshal(gw)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var gw2 GatewaysConfig
	if err := json.Unmarshal(data, &gw2); err != nil {
		t.Fatalf("unmarshal round-trip: %v", err)
	}

	if len(gw2.Telegrams) != 2 {
		t.Fatalf("expected 2, got %d", len(gw2.Telegrams))
	}
	if gw2.Telegrams["trade"].BotToken != "tok1" {
		t.Error("trade bot token mismatch after round-trip")
	}
	if gw2.Slack == nil || gw2.Slack.BotToken != "xoxb" {
		t.Error("slack mismatch after round-trip")
	}
}
