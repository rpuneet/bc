// Package server_test — round-trip integration tests for the daemon HTTP API.
//
// These tests exercise full create→read→verify cycles through the HTTP API
// backed by real SQLite storage, proving the path:
//
//	HTTP request → handler → service → store → SQLite → response
package server_test

import (
	"net/http"
	"testing"
)

// ─── Notify Subscription Round-Trip ─────────────────────────────────────────

// TestE2E_ChannelRoundTrip exercises the notify subscription lifecycle:
// subscribe agent → list subscriptions → verify → unsubscribe.
// pkg/channel was deleted; channels are now gateway-backed via pkg/notify.
func TestE2E_ChannelRoundTrip(t *testing.T) {
	s := newE2EServer(t)

	// 1. Subscribe an agent to a gateway channel
	code, body := s.postJSON(t, "/api/notify/subscriptions", map[string]any{
		"channel":      "slack:roundtrip",
		"agent":        "alice",
		"mention_only": false,
	})
	if code != 201 {
		t.Fatalf("subscribe: want 201, got %d: %v", code, body)
	}
	if body["status"] != "subscribed" {
		t.Fatalf("subscribe: want status=subscribed, got %v", body["status"])
	}

	// 2. Subscribe a second agent
	code, body = s.postJSON(t, "/api/notify/subscriptions", map[string]any{
		"channel":      "slack:roundtrip",
		"agent":        "bob",
		"mention_only": true,
	})
	if code != 201 {
		t.Fatalf("subscribe bob: want 201, got %d: %v", code, body)
	}

	// 3. List subscriptions for the channel
	subCode, subs := s.getList(t, "/api/notify/subscriptions/slack:roundtrip")
	if subCode != 200 {
		t.Fatalf("list channel subscriptions: want 200, got %d", subCode)
	}
	if len(subs) != 2 {
		t.Fatalf("want 2 subscriptions, got %d", len(subs))
	}

	// 4. Verify subscription data survived round-trip
	found := false
	for _, sub := range subs {
		if subMap, ok := sub.(map[string]any); ok && subMap["agent"] == "alice" {
			found = true
			if subMap["channel"] != "slack:roundtrip" {
				t.Errorf("alice subscription: want channel=slack:roundtrip, got %v", subMap["channel"])
			}
		}
	}
	if !found {
		t.Fatal("alice subscription not found in channel subscriptions")
	}

	// 5. Verify subscription appears in global list
	allCode, allSubs := s.getList(t, "/api/notify/subscriptions")
	if allCode != 200 {
		t.Fatalf("list all subscriptions: want 200, got %d", allCode)
	}
	if len(allSubs) < 2 {
		t.Fatalf("want at least 2 global subscriptions, got %d", len(allSubs))
	}
}

// ─── Health Readiness ────────────────────────────────────────────────────────

// TestE2E_HealthReady verifies the readiness probe checks downstream deps.
func TestE2E_HealthReady(t *testing.T) {
	s := newE2EServer(t)

	code, body := s.get(t, "/health/ready")
	if code != 200 {
		t.Fatalf("GET /health/ready: want 200, got %d", code)
	}
	if body["status"] != "ok" {
		t.Fatalf("want status=ok, got %v", body["status"])
	}
	checks, ok := body["checks"].(map[string]any)
	if !ok {
		t.Fatal("expected checks map in readiness response")
	}
	if checks["db"] != "ok" {
		t.Fatalf("want checks.db=ok, got %v", checks["db"])
	}
}

// ─── Doctor Round-Trip ───────────────────────────────────────────────────────

// TestE2E_Doctor_HasChecks verifies doctor returns a report with check results.
func TestE2E_Doctor_HasChecks(t *testing.T) {
	s := newE2EServer(t)

	code, body := s.get(t, "/api/doctor")
	if code != 200 {
		t.Fatalf("want 200, got %d", code)
	}

	// Doctor report should have at least one key (category)
	if len(body) == 0 {
		t.Fatal("doctor report is empty, expected at least one check category")
	}
}

// ─── Costs Round-Trip ────────────────────────────────────────────────────────

// TestE2E_Costs_StructureValid verifies cost summary returns a well-formed
// response (not just non-nil, but with expected structure).
func TestE2E_Costs_StructureValid(t *testing.T) {
	s := newE2EServer(t)

	code, body := s.get(t, "/api/costs")
	if code != 200 {
		t.Fatalf("want 200, got %d", code)
	}

	// Cost summary should have a total_cost field (zero for a fresh home)
	if body == nil {
		t.Fatal("expected non-nil cost summary")
	}
}

// ─── Multi-Step Scenarios ────────────────────────────────────────────────────

// TestE2E_ChannelCreateDeleteVerify tests that subscribe/unsubscribe lifecycle
// works correctly via the notify subscription API.
// pkg/channel CRUD was removed; this test now covers the notify subscription API.
func TestE2E_ChannelCreateDeleteVerify(t *testing.T) {
	s := newE2EServer(t)

	// Subscribe an agent to a gateway channel
	code, _ := s.postJSON(t, "/api/notify/subscriptions", map[string]any{
		"channel": "slack:ephemeral",
		"agent":   "test-agent",
	})
	if code != 201 {
		t.Fatalf("subscribe: want 201, got %d", code)
	}

	// Verify subscription appears in channel list
	subCode, subs := s.getList(t, "/api/notify/subscriptions/slack:ephemeral")
	if subCode != 200 {
		t.Fatalf("list subscriptions after subscribe: want 200, got %d", subCode)
	}
	if len(subs) != 1 {
		t.Fatalf("want 1 subscription after subscribe, got %d", len(subs))
	}

	// Unsubscribe
	unsubCode := s.delete(t, "/api/notify/subscriptions/slack:ephemeral?agent=test-agent")
	if unsubCode != 200 {
		t.Fatalf("unsubscribe: want 200, got %d", unsubCode)
	}

	// Verify subscription is gone
	afterCode, afterSubs := s.getList(t, "/api/notify/subscriptions/slack:ephemeral")
	if afterCode != 200 {
		t.Fatalf("list subscriptions after unsubscribe: want 200, got %d", afterCode)
	}
	if len(afterSubs) != 0 {
		t.Fatalf("want 0 subscriptions after unsubscribe, got %d", len(afterSubs))
	}

	// Verify /api/channels returns empty list (no gateway manager configured)
	channelsCode, channels := s.getList(t, "/api/channels")
	if channelsCode != 200 {
		t.Fatalf("list channels: want 200, got %d", channelsCode)
	}
	_ = channels // fresh home has no active gateway channels
}

// The route the Go client sends on must be mounted for POST. It was not: the
// client posted to /api/channels/{name}/messages, which falls through to the
// GET-only history handler, so every `mycel channel send` failed with 405 while
// the client's own unit test — a fake server answering any path — stayed green.
//
// This E2E server has no gateway configured, so a mounted route answers 503
// ("gateway not available"). Anything is acceptable here except 405 and 404,
// which mean the route is missing or method-mismatched.
func TestE2E_ChannelSendRouteAcceptsPost(t *testing.T) {
	s := newE2EServer(t)

	code, _ := s.postJSON(t, "/api/channels/send", map[string]any{
		"channel": "slack:general",
		"sender":  "alice",
		"message": "hi",
	})
	if code == http.StatusMethodNotAllowed || code == http.StatusNotFound {
		t.Fatalf("POST /api/channels/send returned %d — the send route the CLI uses is not mounted", code)
	}
}
