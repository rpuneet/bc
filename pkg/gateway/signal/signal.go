// Package signal implements a gateway.NotificationAdapter that polls
// a signal-cli REST API for new messages.
package signal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/log"
)

const defaultInterval = 10 // seconds

// Adapter implements gateway.NotificationAdapter for Signal via signal-cli REST.
type Adapter struct {
	httpClient   *http.Client
	handler      func(gateway.Notification)
	lastFetchAt  time.Time
	name         string
	apiURL       string
	lastError    string
	messageCount atomic.Int64
	mu           sync.Mutex
	interval     int
	connected    bool
}

var _ gateway.NotificationAdapter = (*Adapter)(nil)

// New creates a Signal adapter with the default name.
func New(apiURL string, intervalSeconds int) *Adapter {
	return NewNamed("signal", apiURL, intervalSeconds)
}

// NewNamed creates a named Signal adapter.
func NewNamed(name, apiURL string, intervalSeconds int) *Adapter {
	if intervalSeconds <= 0 {
		intervalSeconds = defaultInterval
	}
	return &Adapter{
		name:       name,
		apiURL:     apiURL,
		interval:   intervalSeconds,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *Adapter) Name() string              { return a.name }
func (a *Adapter) Type() gateway.AdapterType { return gateway.AdapterPoll }
func (a *Adapter) HTTPHandler() http.Handler { return nil }
func (a *Adapter) Stop() error               { return nil }

// Start polls the signal-cli REST API until ctx is canceled.
func (a *Adapter) Start(ctx context.Context, handler func(gateway.Notification)) error {
	a.handler = handler

	a.poll(ctx)

	ticker := time.NewTicker(time.Duration(a.interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			a.poll(ctx)
		}
	}
}

// Channels returns a single channel.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	return []gateway.ChannelInfo{{ID: "messages", Name: "messages", Platform: "signal"}}
}

// Status returns the adapter's connection state.
func (a *Adapter) Status() gateway.AdapterStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return gateway.AdapterStatus{
		Connected:     a.connected,
		LastMessageAt: a.lastFetchAt,
		Error:         a.lastError,
		MessageCount:  a.messageCount.Load(),
	}
}

// poll fetches new messages from the signal-cli REST API.
func (a *Adapter) poll(ctx context.Context) {
	url := fmt.Sprintf("%s/v1/receive", a.apiURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		a.setError(fmt.Sprintf("build request: %v", err))
		return
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		a.setError(fmt.Sprintf("fetch: %v", err))
		return
	}
	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		a.setError(fmt.Sprintf("read body: %v", err))
		return
	}

	a.mu.Lock()
	a.connected = true
	a.lastFetchAt = time.Now()
	a.lastError = ""
	a.mu.Unlock()

	var messages []struct {
		Envelope struct {
			Source      string `json:"source"`
			DataMessage struct {
				Message string `json:"message"`
			} `json:"dataMessage"`
		} `json:"envelope"`
	}
	if err := json.Unmarshal(body, &messages); err != nil {
		return // no messages or unexpected format
	}

	for _, msg := range messages {
		sender := msg.Envelope.Source
		if sender == "" {
			sender = "signal"
		}

		raw, _ := json.Marshal(msg) //nolint:errcheck
		a.messageCount.Add(1)

		if a.handler != nil {
			a.handler(gateway.Notification{
				Channel:   "messages",
				Platform:  "signal",
				Sender:    sender,
				Content:   msg.Envelope.DataMessage.Message,
				Timestamp: time.Now(),
				Raw:       raw,
			})
		}
	}

	log.Debug("signal: poll complete", "adapter", a.name, "messages", len(messages))
}

func (a *Adapter) setError(msg string) {
	a.mu.Lock()
	a.lastError = msg
	a.mu.Unlock()
	log.Warn("signal: "+msg, "adapter", a.name)
}
