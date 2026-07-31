package client

import (
	"context"
	"encoding/json"
	"net/http"
)

// SettingsClient provides global settings operations via the daemon.
type SettingsClient struct {
	client *Client
}

// Get returns the current global settings.
func (s *SettingsClient) Get(ctx context.Context) (json.RawMessage, error) {
	var settings json.RawMessage
	if err := s.client.get(ctx, "/api/settings", &settings); err != nil {
		return nil, err
	}
	return settings, nil
}

// Update replaces the global settings with the given patch map.
func (s *SettingsClient) Update(ctx context.Context, patch map[string]any) (json.RawMessage, error) {
	var settings json.RawMessage
	if err := s.client.put(ctx, "/api/settings", patch, &settings); err != nil {
		return nil, err
	}
	return settings, nil
}

// Patch updates a specific section of the global settings.
func (s *SettingsClient) Patch(ctx context.Context, section string, data map[string]any) (json.RawMessage, error) {
	var settings json.RawMessage
	if err := s.client.do(ctx, http.MethodPatch, "/api/settings/"+section, data, &settings); err != nil {
		return nil, err
	}
	return settings, nil
}
