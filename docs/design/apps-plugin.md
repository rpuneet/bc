# Apps: the plugin platform for external integrations

Status: approved direction (2026-07-30 owner directive) · Wave 1 of the plugin-era program

Notifications, gateway credentials, and the standalone secrets concept merge
into one product surface: **Apps**. You connect an app (Slack, GitHub,
WhatsApp, Telegram, …); the app owns its credentials, its transport, its
channels, and its per-agent wiring. Adding a new integration to mycel means
writing one self-contained plugin package — no central switch statements.

## Why

Today the pieces exist but aren't a platform:

- `server/build_services.go` constructs every adapter in a ~300-line
  hand-written chain — one block per platform, 28 constructor calls.
- `pkg/workspace/config_gateways.go` declares 34 per-platform config structs
  (981 lines) with custom JSON marshaling, all carrying **plaintext tokens
  inside preferences.json**.
- `pkg/secret` is a clean encrypted vault, but gateway credentials bypass it.
- Platform-specific behavior leaks into handlers
  (`gateways.go: switch platform` for pairing, per-platform PATCH cases).

## The App plugin contract (`pkg/app`)

```go
// AuthKind declares how an app authenticates.
type AuthKind string

const (
    AuthToken         AuthKind = "token"          // paste an API key / bot token
    AuthOAuth         AuthKind = "oauth"          // browser flow (device flow or localhost callback)
    AuthQR            AuthKind = "qr"             // scan-to-pair with persistent session (WhatsApp)
    AuthWebhookSecret AuthKind = "webhook-secret" // inbound webhook with shared secret
    AuthNone          AuthKind = "none"           // no credentials (RSS)
)

// FieldSpec describes one config or credential field.
type FieldSpec struct {
    Key         string // "bot_token"
    Label       string
    Placeholder string
    Secret      bool // stored only in the vault, never in prefs JSON
    Required    bool
}

// Descriptor is the app's static self-description. It drives the connect-app
// UI (labels, fields, docs) and the config schema — no per-app UI code.
type Descriptor struct {
    ID     string // "slack"
    Label  string // "Slack"
    Auth   AuthKind
    Fields []FieldSpec
    Docs   []string // setup instructions rendered in the connect flow
    Multi  bool     // allows labeled instances ("telegram:alerts")
}

// Instance is one connected app: descriptor ID + instance name + resolved config.
type Instance struct {
    App     string            // descriptor ID
    Name    string            // "slack" or "telegram:alerts"
    Enabled bool
    Config  map[string]string // non-secret fields only
    Secrets SecretSource      // resolves Secret fields from the vault
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
```

Registration is decentralized: each plugin package has a `plugin.go` with
`func init() { app.Register(plugin{}) }`, and `pkg/app/builtin/builtin.go`
imports the enabled set for side effects. Adding an app = one new package +
one import line.

Optional capabilities stay runtime-asserted on the built adapter, exactly as
today (`messageSender`, `fileSender`, `reactionSender`, `ChannelIdentity`).
QR pairing joins that list: pairing state (QR channel, session, connection)
lives on the running adapter, so `QRPairer` is asserted on the adapter
returned by `Build`, not on the plugin:

```go
// PairInfo reports QR pairing progress.
type PairInfo struct {
    State     string // "idle", "qr_ready", "connected", "error"
    QRDataURL string
    Phone     string
    Error     string
}

// QRPairer is implemented by built adapters whose app pairs by QR (WhatsApp).
type QRPairer interface {
    StartPairing(ctx context.Context) (PairInfo, error)
    PairStatus() PairInfo
}
```

OAuth remains a plugin-level capability (no live adapter exists before auth
completes):

```go
// OAuthFlow is implemented by plugins whose app can authenticate via a
// browser flow. GitHub ships the device flow (fully local: any OAuth
// app's client ID, no client secret, no redirect URL). Apps that can't
// run locally (Slack requires an HTTPS redirect) stay token-paste until
// a hosted relay exists. Discord is deliberately not OAuth: its adapter
// needs a bot token, which OAuth cannot mint (it only issues user/bearer
// tokens) — a flow yielding an unusable credential would be fake.
type OAuthFlow interface {
    // BeginAuth starts the flow. Device flow returns VerificationURL +
    // UserCode; callback flow returns AuthURL.
    BeginAuth(ctx context.Context, inst Instance) (AuthSession, error)
    // PollAuth reports progress; on success it returns the secrets to
    // persist, keyed by descriptor Secret field keys. Sessions live
    // in-memory on the plugin — a daemon restart aborts pending auths.
    PollAuth(ctx context.Context, session AuthSession) (AuthResult, error)
}
```

The catalog response advertises the capability as `oauth_available` per
app (keyed off the interface assertion, not the descriptor's AuthKind),
and `POST /api/apps/{name}/auth` + `GET .../auth/status?session=<id>`
drive it; on completion the server persists the returned secrets to the
vault and hot-starts the adapter.

## Config: generic instances, secrets in the vault

`preferences.json` `gateways` section is replaced by `apps`:

```json
{
  "apps": {
    "slack":           { "app": "slack",    "enabled": true, "config": { "mode": "socket" } },
    "telegram:alerts": { "app": "telegram", "enabled": true, "config": { "mode": "poll" } }
  }
}
```

- One shape for every app; validation against the descriptor's `Fields`.
- **No secret ever lands in prefs.** Fields marked `Secret` write to the
  vault under `app:<instance>:<key>` (e.g. `app:slack:bot_token`); the
  server resolves them at Build time via `SecretSource`.
- `pkg/workspace/config_gateways.go` (34 structs + custom marshal) is
  deleted. `PATCH /api/settings {"apps": ...}` merges generically.
- One-time migration of the owner's live preferences.json is an ops step on
  the deployment machine, not shipped compatibility code.

## Server wiring

`buildGatewayManager` collapses to:

```go
for name, ic := range cfg.Apps {
    plugin, ok := app.Get(ic.App)
    if !ok { degraded["app:"+name] = "unknown app"; continue }
    inst := app.ResolveInstance(name, ic, vault)
    adapter, err := plugin.Build(inst, app.Env{StateDir: appStateDir(name)})
    ...
    m.Register(adapter)
}
```

HTTP surface (replaces `/api/gateways/*`, keeps `gateway.Manager` runtime):

| Route | Purpose |
|---|---|
| `GET  /api/apps` | catalog (descriptors) + connected instances with status |
| `POST /api/apps/{name}` | connect/update an instance (secret fields → vault) |
| `DELETE /api/apps/{name}` | disconnect + delete vault keys + state dir |
| `POST /api/apps/{name}/auth` | begin OAuth/QR (dispatches on plugin capability — no platform switch) |
| `GET  /api/apps/{name}/auth/status` | poll pairing/auth completion |
| existing channel/subscription routes | unchanged, re-rooted under `/api/apps/{name}/channels` |

`pkg/notify` remains the fan-out layer (subscriptions, delivery, history) —
unchanged in W1 except the merged `Notification` type moves to one
definition.

## Migration order

1. `pkg/app` core: types, registry, instance resolution, vault-backed
   `SecretSource`, tests.
2. Reference plugins proving each AuthKind: **slack** (token, multi-field),
   **telegram** (token, Multi), **webhook** (webhook-secret, Multi),
   **rss** (none), **whatsapp** (qr, stateful Env).
3. Data-driven `buildGatewayManager` + `/api/apps` handlers; delete the
   hardcoded chain, `config_gateways.go`, and per-platform handler switches.
4. Port the remaining adapters (mechanical: descriptor + Build wrapping the
   existing constructor). Which of the 34 survive is an open product call;
   the port order starts with the core set (discord, github, gitlab, signal,
   matrix, mattermost, msteams, googlechat, irc, sentry, pagerduty…).
5. OAuth: GitHub device flow first (fully local), Discord localhost
   callback next. Slack stays token-paste until a hosted relay exists.
6. Web UI (Wave 2): `/apps` home, connect flow driven entirely by
   descriptors, per-agent app wiring in New Agent + agent Config.

## Non-goals (W1)

- No hosted OAuth relay.
- No change to notify subscription semantics or delivery.
- No TUI changes.
