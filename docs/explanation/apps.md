# Apps: the plugin platform for external integrations

Notifications, gateway credentials, and secrets for external platforms are
one product surface: **Apps**. You connect an app (Slack, GitHub, WhatsApp,
Telegram, …); the app owns its credentials, its transport, its channels, and
its per-agent wiring. Adding a new integration to mycel means writing one
self-contained plugin package — no central switch statements.

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

### Loopback (localhost redirect) flow — Google / Gmail

The generic authorization-code + PKCE flow lives in `pkg/oauth`
(`LoopbackFlow`). On `BeginAuth` the daemon opens a short-lived listener on
`http://127.0.0.1:<port>/oauth/callback` (an ephemeral port), returns the
provider consent URL (`AuthURL`, `Kind == "callback"`), and the web UI opens
it in the user's browser. Google redirects back to the loopback listener with
the `code`; `PollAuth` exchanges it (with the PKCE verifier) for access +
refresh tokens and hands them back as vault secrets. This is fully local — no
hosted redirect, no cloud — which is exactly what Google "Desktop app" OAuth
clients permit.

Gmail wires this flow (`pkg/gateway/gmail`) with the `gmail.readonly` +
`gmail.send` scopes and `access_type=offline` + `prompt=consent` so Google
always returns a refresh token. The connect UI **always** offers both
**Sign in with Gmail** (browser link) and **Advanced** manual paste.

Zero-paste Sign in uses the server Google client from
`GOOGLE_OAUTH_CLIENT_ID` / `GOOGLE_OAUTH_CLIENT_SECRET` (or release
ldflags). When those are unset, paste Client ID + Client Secret under
Advanced and click Sign in anyway — same loopback link, bring-your-own
client — or paste a Refresh Token for the fully manual path. Gmail does
**not** implement `OAuthConfigured`; hiding Sign in when the server
client was missing is what made the link option disappear.

Other OAuth plugins may still implement `OAuthConfigured` when a button
would always fail with no user-supplied alternative:

```go
// OAuthConfigured is an optional capability an OAuthFlow plugin implements to
// report whether its browser flow is usable right now (server-side client
// creds present). false → the catalog reports oauth_available=false.
type OAuthConfigured interface {
    OAuthConfigured() bool
}
```

**Registering the Google "Desktop app" client (owner, one-time):**

1. Google Cloud Console → *APIs & Services* → **Enable** the Gmail API for the
   project.
2. *APIs & Services* → *OAuth consent screen* → add the scopes
   `https://www.googleapis.com/auth/gmail.readonly` and
   `https://www.googleapis.com/auth/gmail.send`; add your Google account as a
   test user (or publish the app).
3. *APIs & Services* → *Credentials* → *Create credentials* → *OAuth client ID*
   → application type **Desktop app**. Desktop clients allow the loopback
   redirect `http://127.0.0.1:<port>/oauth/callback` (any port), so no fixed
   redirect URL needs registering.
4. Export the client on the machine running the daemon:
   `GOOGLE_OAUTH_CLIENT_ID=<id>.apps.googleusercontent.com` and
   `GOOGLE_OAUTH_CLIENT_SECRET=GOCSPX-…`, then restart `mycel up`. Gmail's
   connect modal **Sign in with Gmail** works with zero pasting.

Slack-type apps that need a fixed HTTPS redirect stay on token paste; real
Slack OAuth is deferred to a future hosted-redirect ("mycel cloud").

## Config: generic instances, secrets in the vault

The old `gateways` config section is replaced by `apps` in `prefs.json`:

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
- The per-platform config structs (34 structs + custom marshal) are
  deleted. `PATCH /api/settings {"apps": ...}` merges generically per
  instance key, validated against each app's descriptor.
- Agent sessions receive app credentials as env vars named by convention:
  `UPPER(app)_UPPER(field)`, with the instance label appended for labeled
  instances (`app.EnvKey`: `github` + `token` → `GITHUB_TOKEN`;
  `telegram:alerts` + `bot_token` → `TELEGRAM_BOT_TOKEN_ALERTS`).

### Per-agent vault scoping

Connected-app Secret fields and well-known integration tokens
(`SLACK_BOT_TOKEN`, `TELEGRAM_BOT_TOKEN`, …) are **not** injected into every
agent session. On spawn/restart, mycel looks up the agent's unmuted
notification subscriptions (`slack:general`, `telegram:alerts:ops`, catch-all
`slack:*`, …) and only exports credentials for matching app instances /
platforms (#3686):

| Source | Injected when |
|---|---|
| Role / template `secrets` allowlist | Always (unchanged) |
| Connected-app Secret fields (`app:<instance>:<key>`) | Agent has a subscription whose instance prefix matches the app instance name (`telegram:alerts:ops` → instance `telegram:alerts`; `slack:*` → `slack`) |
| Well-known tokens (`SLACK_BOT_TOKEN`, …) | Agent is subscribed to that platform (`slack:…` / `telegram:…`, including labeled instances) |
| Connected GitHub `api_token` → `GH_TOKEN` / `GITHUB_TOKEN` | Agent subscribed to that github instance, **or** the role allowlist already requested `GITHUB_TOKEN` / `GH_TOKEN` |

Non-secret required fields (feed URLs, homeservers, …) still inject for every
enabled instance via `injectAppEnv` — they are not credentials.

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

`pkg/notify` remains the fan-out layer: subscriptions, delivery, history,
one `Notification` type.

## Built-in plugins

**28 built-in plugins** are registered via side-effect imports in
`pkg/app/builtin/builtin.go` — bitbucket, datadog, discord, github, gitlab,
grafana, imessage, irc, jira, line, linear, matrix, mattermost, mqtt,
netlify, notion, pagerduty, reddit, rss, sentry, signal, slack, stripe,
telegram, twitch, vercel, webhook, whatsapp.

`buildGatewayManager` is entirely data-driven over `prefs.json`'s `apps`
section — there is no hardcoded per-platform chain, no per-platform config
structs, and no handler switches. `/api/apps` serves the catalog, instance
CRUD, and auth flows. `QRPairer` is asserted on the **built adapter**
(pairing state lives on the running adapter), not on the plugin.

## Non-goals

- No hosted OAuth relay.
- No change to notify subscription semantics or delivery.
