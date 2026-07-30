package rss

import (
	"testing"

	"github.com/rpuneet/mycel/pkg/app"
)

func TestPluginDescribe(t *testing.T) {
	d := plugin{}.Describe()
	if d.ID != "rss" || d.Label == "" {
		t.Errorf("Describe() = ID %q, Label %q; want rss, non-empty", d.ID, d.Label)
	}
	if d.Auth != app.AuthNone {
		t.Errorf("Auth = %q, want %q", d.Auth, app.AuthNone)
	}
	if !d.Multi {
		t.Error("Multi = false, want true")
	}
	for i, f := range d.Fields {
		if f.Key == "" {
			t.Errorf("Fields[%d] has empty key", i)
		}
	}
	if len(d.Docs) == 0 {
		t.Error("Docs is empty")
	}
}

func TestPluginBuild(t *testing.T) {
	tests := []struct {
		cfg     map[string]string
		name    string
		wantErr bool
	}{
		{name: "url only", cfg: map[string]string{"url": "https://example.com/feed.xml"}},
		{name: "url and interval", cfg: map[string]string{"url": "https://example.com/feed.xml", "interval": "60"}},
		{name: "missing url", cfg: map[string]string{}, wantErr: true},
		{name: "bad interval", cfg: map[string]string{"url": "https://example.com/feed.xml", "interval": "soon"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := app.Instance{App: "rss", Name: "rss:blog", Config: tt.cfg, Secrets: app.MapSecrets{}}
			adapter, err := plugin{}.Build(inst, app.Env{})
			if (err != nil) != tt.wantErr {
				t.Fatalf("Build error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if adapter.Name() != "rss:blog" {
				t.Errorf("Name() = %q, want rss:blog", adapter.Name())
			}
		})
	}
}
