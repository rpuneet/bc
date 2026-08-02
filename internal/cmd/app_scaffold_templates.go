package cmd

import "text/template"

// appScaffoldPluginTmpl generates pkg/gateway/<name>/plugin.go — the
// app.Plugin implementation (Describe + Build) that registers itself
// with the default app registry.
var appScaffoldPluginTmpl = template.Must(template.New("plugin").Parse(`package {{.Pkg}}

import (
	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for {{.Label}}.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "{{.Name}}",
		Label: "{{.Label}}",
		// TODO: pick the auth kind that matches {{.Label}}'s API
		// (app.AuthToken, app.AuthOAuth, app.AuthQR, app.AuthNone).
		Auth:  app.AuthWebhookSecret,
		Multi: {{.Multi}},
		Fields: []app.FieldSpec{
			// TODO: replace with {{.Label}}'s real config/credential fields.
			{Key: "api_key", Label: "API Key", Placeholder: "your-api-key", Secret: true, Required: true},
			{Key: "workspace", Label: "Workspace", Placeholder: "my-workspace"},
		},
		Docs: []string{
			// TODO: replace with real setup instructions.
			"POST JSON to /hooks/{{.Name}} on your mycel server.",
			"TODO: describe how to obtain the API key and configure the webhook.",
		},
	}
}

func (plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	// TODO: resolve any additional config/secrets needed to build the adapter.
	apiKey, err := inst.RequiredSecret("api_key")
	if err != nil {
		return nil, err
	}
	return New(inst.Name, apiKey), nil
}
`))

// appScaffoldAdapterTmpl generates pkg/gateway/<name>/<name>.go — the
// gateway.NotificationAdapter implementation with TODO stubs for the
// adapter's actual wire-up.
var appScaffoldAdapterTmpl = template.Must(template.New("adapter").Parse(`// Package {{.Pkg}} implements a gateway.NotificationAdapter for {{.Label}}.
package {{.Pkg}}

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rpuneet/mycel/pkg/gateway"
)

// Adapter implements gateway.NotificationAdapter for {{.Label}}.
type Adapter struct {
	lastMessageAt time.Time
	handler       func(gateway.Notification)
	name          string
	apiKey        string
	lastError     string
	mu            sync.Mutex
	connected     bool
	messageCount  atomic.Int64
}

var _ gateway.NotificationAdapter = (*Adapter)(nil)

// New creates a new {{.Label}} adapter.
func New(name, apiKey string) *Adapter {
	return &Adapter{name: name, apiKey: apiKey}
}

// Name returns the adapter identifier.
func (a *Adapter) Name() string { return a.name }

// Type returns the connection pattern for this adapter.
// TODO: change to gateway.AdapterSocket or gateway.AdapterPoll if
// {{.Label}} is not webhook-based.
func (a *Adapter) Type() gateway.AdapterType { return gateway.AdapterWebhook }

// Start connects to {{.Label}} and begins receiving notifications.
// TODO: for socket/poll adapters, connect and start the receive loop
// here, calling handler for each inbound event and blocking until ctx
// is canceled. Webhook adapters can just store the handler.
func (a *Adapter) Start(_ context.Context, handler func(gateway.Notification)) error {
	a.handler = handler
	return nil
}

// Stop gracefully disconnects from {{.Label}}.
// TODO: close any open connections/goroutines started in Start.
func (a *Adapter) Stop() error { return nil }

// HTTPHandler returns an http.Handler for webhook-based adapters, or
// nil for socket/poll adapters.
// TODO: implement the actual webhook handler (verify signature/secret,
// parse the payload, call a.handler with a gateway.Notification).
func (a *Adapter) HTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not implemented", http.StatusNotImplemented)
	})
}

// Channels returns discovered channels/groups the adapter has access to.
// TODO: return real channels once {{.Label}} exposes them.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	return []gateway.ChannelInfo{
		{ID: a.name, Name: a.name, Platform: "{{.Name}}"},
	}
}

// Status returns the adapter's connection state for the web UI.
func (a *Adapter) Status() gateway.AdapterStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return gateway.AdapterStatus{
		Connected:     a.connected,
		LastMessageAt: a.lastMessageAt,
		Error:         a.lastError,
		MessageCount:  a.messageCount.Load(),
	}
}
`))
