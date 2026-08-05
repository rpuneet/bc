package client

import "context"

// SystemInfo is the lightweight workspace/host summary from GET /api/system/info.
type SystemInfo struct {
	Hostname     string `json:"hostname"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	Workspace    string `json:"workspace"`
	HasWorkspace bool   `json:"has_workspace"`
}

// SystemInfo returns daemon workspace/host info (CWD-free).
func (s *StatsClient) SystemInfo(ctx context.Context) (*SystemInfo, error) {
	var info SystemInfo
	if err := s.client.get(ctx, "/api/system/info", &info); err != nil {
		return nil, err
	}
	return &info, nil
}
