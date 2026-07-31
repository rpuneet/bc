// Package app defines the plugin contract for external integrations
// ("apps"). Each app is a self-contained plugin package that describes
// itself via a Descriptor and builds a live gateway.NotificationAdapter
// from a connected Instance. See docs/design/apps-plugin.md.
package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/rpuneet/mycel/pkg/gateway"
)

// AuthKind declares how an app authenticates.
type AuthKind string

const (
	// AuthToken means the user pastes an API key / bot token.
	AuthToken AuthKind = "token"
	// AuthOAuth means a browser flow (device flow or localhost callback).
	AuthOAuth AuthKind = "oauth"
	// AuthQR means scan-to-pair with a persistent session (WhatsApp).
	AuthQR AuthKind = "qr"
	// AuthWebhookSecret means an inbound webhook with a shared secret.
	AuthWebhookSecret AuthKind = "webhook-secret"
	// AuthNone means no credentials (RSS).
	AuthNone AuthKind = "none"
)

// FieldSpec describes one config or credential field.
type FieldSpec struct {
	Key         string // "bot_token"
	Label       string
	Placeholder string
	Secret      bool // stored only in the vault, never in prefs JSON
	Required    bool
}

// Descriptor is the app's static self-description. It drives the
// connect-app UI (labels, fields, docs) and the config schema — no
// per-app UI code.
type Descriptor struct {
	ID     string // "slack"
	Label  string // "Slack"
	Auth   AuthKind
	Fields []FieldSpec
	Docs   []string // setup instructions rendered in the connect flow
	Multi  bool     // allows labeled instances ("telegram:alerts")
}

// Instance is one connected app: descriptor ID + instance name +
// resolved config.
type Instance struct {
	Secrets SecretSource      // resolves Secret fields from the vault
	Config  map[string]string // non-secret fields only
	App     string            // descriptor ID
	Name    string            // "slack" or "telegram:alerts"
	Enabled bool
}

// RequiredSecret resolves a secret field that must be present and
// non-empty for the app to build.
func (i Instance) RequiredSecret(key string) (string, error) {
	if i.Secrets == nil {
		return "", fmt.Errorf("app %s: no secret source for required field %q", i.Name, key)
	}
	v, err := i.Secrets.Get(key)
	if err != nil {
		return "", fmt.Errorf("app %s: required secret %q: %w", i.Name, key, err)
	}
	if v == "" {
		return "", fmt.Errorf("app %s: required secret %q is empty", i.Name, key)
	}
	return v, nil
}

// OptionalSecret resolves a secret field, returning "" when unset.
func (i Instance) OptionalSecret(key string) string {
	if i.Secrets == nil {
		return ""
	}
	v, err := i.Secrets.Get(key)
	if err != nil {
		return ""
	}
	return v
}

// Env gives stateful apps a home (WhatsApp session DB, caches).
type Env struct {
	StateDir string // <state>/apps/<instance-name>/
}

// Plugin builds a live adapter from an instance.
type Plugin interface {
	Describe() Descriptor
	Build(inst Instance, env Env) (gateway.NotificationAdapter, error)
}

// PairInfo reports QR pairing progress.
type PairInfo struct {
	State     string `json:"state"` // "idle", "qr_ready", "connected", "error"
	QRDataURL string `json:"qr_data_url,omitempty"`
	Phone     string `json:"phone,omitempty"`
	Error     string `json:"error,omitempty"`
}

// QRPairer is implemented by built adapters whose app pairs by QR
// (WhatsApp). Pairing state lives on the running adapter, so the
// capability is asserted on the adapter returned by Build, not on the
// plugin.
type QRPairer interface {
	StartPairing(ctx context.Context) (PairInfo, error)
	PairStatus() PairInfo
}

// SecretSource resolves an instance's secret fields by field key.
type SecretSource interface {
	Get(key string) (string, error)
}

// SecretValueStore resolves vault values by secret name. *secret.Store
// and secret.LayeredStore implement it. Declared here as a minimal
// interface so pkg/app does not import pkg/secret (which imports
// pkg/home — pkg/home holds the Apps config and imports
// pkg/app, so a direct dependency would cycle).
type SecretValueStore interface {
	GetValue(name string) (string, error)
}

// VaultSecrets resolves secret fields from the encrypted vault under
// "app:<instance-name>:<field-key>".
type VaultSecrets struct {
	Store    SecretValueStore
	Instance string
}

// Get returns the decrypted value for a field key.
func (v VaultSecrets) Get(key string) (string, error) {
	return v.Store.GetValue(SecretName(v.Instance, key))
}

// SecretName returns the vault key for an instance's secret field.
func SecretName(instance, key string) string {
	return "app:" + instance + ":" + key
}

// EnvKey returns the conventional agent env-var name for an instance's
// field: UPPER(app)_UPPER(field), with the instance label appended for
// labeled instances ("telegram:alerts" + "bot_token" →
// TELEGRAM_BOT_TOKEN_ALERTS).
func EnvKey(instance, fieldKey string) string {
	appID := instance
	label := ""
	if i := strings.Index(instance, ":"); i >= 0 {
		appID, label = instance[:i], instance[i+1:]
	}
	key := envToken(appID) + "_" + envToken(fieldKey)
	if label != "" {
		key += "_" + envToken(label)
	}
	return key
}

// envToken uppercases a name segment for env-var use.
func envToken(s string) string {
	return strings.ToUpper(strings.NewReplacer("-", "_", ".", "_", ":", "_").Replace(s))
}

// MapSecrets is an in-memory SecretSource for tests.
type MapSecrets map[string]string

// Get returns the value for key, or an error when absent.
func (m MapSecrets) Get(key string) (string, error) {
	v, ok := m[key]
	if !ok {
		return "", fmt.Errorf("secret %q not found", key)
	}
	return v, nil
}
