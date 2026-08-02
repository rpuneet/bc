/**
 * ConnectApp — the catalog-driven connect flow for Apps.
 *
 * Everything that describes an app — its label, auth kind, config
 * fields, secret flags and setup docs — comes from the backend
 * descriptors (GET /api/apps). This file only keeps presentation-level
 * metadata (icon, brand color, category, one-line description) keyed by
 * app ID. An app with no backend descriptor does not render.
 *
 * The flow reads: pick app → authenticate (auth-kind-specific) →
 * choose agents/channels → done.
 *
 *   • token / webhook-secret / none → config fields; secret fields are
 *     password inputs, stored server-side in the encrypted vault.
 *     Already-configured secrets show a "configured" state and are
 *     replace-only — values are never echoed back.
 *   • qr → QR pairing via POST /api/apps/{name}/auth + status polling.
 *   • multi apps take an optional instance label ("telegram:alerts").
 */

import { useEffect, useRef, useState, useCallback } from "react";
import { createPortal } from "react-dom";
import { api } from "../../api/client";
import type { Agent, AppAuthSession, AppDescriptor, AppInstance } from "../../api/client";
import { AppIcon, presentationFor } from "./PlatformIcons";
import { StatusDot } from "./appStatus";
import { openExternal } from "../../utils/openExternal";

/* ── Presentation-only metadata (backend owns the rest) ──────────────
   App icon/color/category/description metadata lives in PlatformIcons.tsx
   (APP_PRESENTATION / presentationFor) alongside the brand SVG map, so
   every app-icon consumer shares one source of truth instead of each
   view re-deriving its own fallback chain. */

const CATEGORY_ORDER = ["Chat", "Code & DevOps", "Monitoring", "Payments", "Content", "Custom", "Other"];

/** Short auth-kind hint shown on catalog cards. */
function authHint(d: AppDescriptor): { label: string; tone: "accent" | "warning" | "info" | "muted" } {
  if (d.oauth_available && d.auth !== "qr") {
    return { label: "Browser sign-in available", tone: "info" };
  }
  switch (d.auth) {
    case "qr":
      return { label: "Scan QR to pair", tone: "accent" };
    case "webhook-secret":
      return { label: "Webhook · requires public URL", tone: "warning" };
    case "oauth":
      return { label: "Browser sign-in", tone: "info" };
    case "none":
      return { label: "No credentials needed", tone: "muted" };
    default:
      return { label: "Ready to connect", tone: "accent" };
  }
}

function AppGlyph({ appId, size }: { appId: string; size: number }) {
  return (
    <span className="flex items-center justify-center">
      <AppIcon base={appId} size={size} />
    </span>
  );
}

/* ── App chooser — full-screen catalog modal ─────────────────── */

export function AppChooser({ onSelect, onClose }: { onSelect: (appId: string) => void; onClose: () => void }) {
  const [search, setSearch] = useState("");
  const [catalog, setCatalog] = useState<AppDescriptor[]>([]);
  const [instances, setInstances] = useState<AppInstance[]>([]);
  const [loading, setLoading] = useState(true);
  const searchRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    api
      .getApps()
      .then((res) => {
        setCatalog(res.catalog ?? []);
        setInstances(res.instances ?? []);
      })
      .catch(() => { /* empty catalog renders the empty state */ })
      .finally(() => { setLoading(false); });
  }, []);

  // Focus search on mount
  useEffect(() => {
    requestAnimationFrame(() => searchRef.current?.focus());
  }, []);

  // Escape to close
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", handler);
    return () => { document.removeEventListener("keydown", handler); };
  }, [onClose]);

  const instancesByApp = new Map<string, AppInstance[]>();
  for (const inst of instances) {
    const list = instancesByApp.get(inst.app) ?? [];
    list.push(inst);
    instancesByApp.set(inst.app, list);
  }

  const q = search.trim().toLowerCase();
  const filtered = catalog.filter((d) => {
    if (!q) return true;
    const pres = presentationFor(d.id);
    return (
      d.label.toLowerCase().includes(q) ||
      d.id.toLowerCase().includes(q) ||
      pres.description.toLowerCase().includes(q) ||
      pres.category.toLowerCase().includes(q)
    );
  });

  const connectedApps = filtered.filter((d) => (instancesByApp.get(d.id) ?? []).length > 0);
  const availableApps = filtered.filter((d) => (instancesByApp.get(d.id) ?? []).length === 0);

  const renderCard = (d: AppDescriptor) => {
    const pres = presentationFor(d.id);
    const appInstances = instancesByApp.get(d.id) ?? [];
    const isConnected = appInstances.length > 0;
    const hint = authHint(d);
    const hintColor =
      hint.tone === "accent" ? "text-mycel-accent"
        : hint.tone === "warning" ? "text-mycel-warning"
          : hint.tone === "info" ? "text-mycel-info"
            : "text-mycel-muted";

    return (
      <button
        key={d.id}
        type="button"
        data-testid={`app-card-${d.id}`}
        onClick={() => { onSelect(d.id); }}
        className={`
          relative flex flex-col p-4 rounded-lg border text-left
          transition-all duration-150 ease-out group
          border-mycel-border cursor-pointer hover:border-mycel-accent hover:scale-[1.02] hover:shadow-mycel
          ${isConnected ? "border-mycel-success bg-mycel-success-subtle" : ""}
        `}
      >
        {/* Connected badge */}
        {isConnected && (
          <div className="absolute top-2.5 right-2.5">
            <svg width="18" height="18" viewBox="0 0 18 18" fill="none">
              <circle cx="9" cy="9" r="8" fill="var(--mycel-success)" opacity="0.15" />
              <path d="M5.5 9l2.5 2.5 4.5-4.5" stroke="var(--mycel-success)" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          </div>
        )}

        {/* Icon + name */}
        <div className="flex items-center gap-2.5 mb-2">
          <AppGlyph appId={d.id} size={20} />
          <span className="text-sm font-semibold text-mycel-text transition-colors group-hover:text-mycel-accent">
            {d.label}
          </span>
        </div>

        {/* Description */}
        <p className="text-xs text-mycel-muted leading-relaxed flex-1">{pres.description}</p>

        {/* Status tag */}
        <div className="mt-2.5">
          {isConnected ? (
            <span className="inline-flex items-center gap-1 text-[10px] font-medium text-mycel-success">
              <span className="w-1.5 h-1.5 rounded-full bg-mycel-success" />
              Connected
              {appInstances.length > 1 ? ` · ${String(appInstances.length)} instances` : ""}
              {d.multi && <span className="text-mycel-muted font-normal">· add another</span>}
            </span>
          ) : (
            <span className={`inline-flex items-center gap-1 text-[10px] ${hintColor}`}>
              {hint.tone === "accent" && (
                <span className="w-1.5 h-1.5 rounded-full" style={{ backgroundColor: pres.color, opacity: 0.7 }} />
              )}
              {hint.label}
            </span>
          )}
        </div>
      </button>
    );
  };

  return createPortal(
    <div
      className="fixed inset-0 z-50 flex items-center justify-center"
      style={{ animation: "fadeIn 120ms ease-out" }}
    >
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-mycel-overlay backdrop-blur-md"
        onClick={onClose}
      />

      {/* Modal */}
      <div className="relative z-10 bg-mycel-surface-2 border border-mycel-border rounded-lg shadow-mycel-lg flex flex-col w-[calc(100vw-48px)] max-w-[960px] max-h-[calc(100vh-48px)]">
        {/* Header */}
        <div className="px-6 pt-5 pb-4 border-b border-mycel-border shrink-0">
          <div className="flex items-center justify-between mb-4">
            <div>
              <h2 className="text-base font-semibold text-mycel-text tracking-tight">Connect an app</h2>
              <p className="text-xs text-mycel-muted mt-0.5">Choose a service to wire into your agents</p>
            </div>
            <button
              type="button"
              onClick={onClose}
              className="w-8 h-8 flex items-center justify-center rounded-md text-mycel-muted hover:text-mycel-text hover:bg-mycel-surface-hover transition-colors"
              aria-label="Close"
            >
              <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
                <path d="M4 4l8 8M12 4l-8 8" />
              </svg>
            </button>
          </div>

          {/* Search */}
          <div className="relative">
            <svg
              className="absolute left-3 top-1/2 -translate-y-1/2 text-mycel-muted"
              width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.3"
            >
              <circle cx="6" cy="6" r="4" />
              <path d="M9 9l3.5 3.5" strokeLinecap="round" />
            </svg>
            <input
              ref={searchRef}
              type="text"
              value={search}
              onChange={(e) => { setSearch(e.target.value); }}
              placeholder="Search apps..."
              className="w-full pl-9 pr-3 py-2.5 text-sm rounded-md border border-mycel-border bg-mycel-surface text-mycel-text placeholder:text-mycel-muted focus:outline-none focus:ring-1 focus:ring-mycel-accent focus:border-mycel-accent transition-colors"
            />
          </div>
        </div>

        {/* Grid content */}
        <div className="flex-1 overflow-auto px-6 py-5" style={{ scrollbarWidth: "thin", scrollbarColor: "var(--mycel-border) transparent" }}>
          {loading && (
            <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
              {Array.from({ length: 8 }).map((_, i) => (
                <div key={i} className="h-28 rounded-lg bg-mycel-surface-hover animate-pulse" />
              ))}
            </div>
          )}

          {/* Connected section */}
          {connectedApps.length > 0 && (
            <div className="mb-6">
              <h3 className="text-[11px] font-medium text-mycel-success uppercase tracking-[0.08em] mb-3 flex items-center gap-2">
                <span className="w-1.5 h-1.5 rounded-full bg-mycel-success" />
                Connected
              </h3>
              <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
                {connectedApps.map(renderCard)}
              </div>
            </div>
          )}

          {connectedApps.length > 0 && availableApps.length > 0 && (
            <div className="border-t border-mycel-border my-5" />
          )}

          {/* Categorized available apps */}
          {CATEGORY_ORDER.map((cat) => {
            const items = availableApps.filter((d) => presentationFor(d.id).category === cat);
            if (items.length === 0) return null;
            return (
              <div key={cat} className="mb-6 last:mb-0">
                <h3 className="text-[11px] font-medium text-mycel-muted uppercase tracking-[0.08em] mb-3">{cat}</h3>
                <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
                  {items.map(renderCard)}
                </div>
              </div>
            );
          })}

          {!loading && filtered.length === 0 && (
            <div className="flex flex-col items-center justify-center py-16 text-mycel-muted">
              <svg width="32" height="32" viewBox="0 0 32 32" fill="none" stroke="currentColor" strokeWidth="1.2" className="mb-3 opacity-30">
                <circle cx="14" cy="14" r="9" />
                <path d="M20.5 20.5l7 7" strokeLinecap="round" />
              </svg>
              <p className="text-sm font-medium">
                {catalog.length === 0 ? "App catalog unavailable" : `No apps match “${search}”`}
              </p>
            </div>
          )}
        </div>
      </div>
    </div>,
    document.body,
  );
}

/* ── Agent subscription step ─────────────────────────────────── */

function AgentSubscriptionStep({
  instanceName,
  appLabel,
  onDone,
}: {
  instanceName: string;
  appLabel: string;
  onDone: () => void;
}) {
  const isTelegram = instanceName === "telegram" || instanceName.startsWith("telegram:");
  const [agents, setAgents] = useState<Agent[]>([]);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [mentionOnly, setMentionOnly] = useState<Set<string>>(new Set());
  const [channels, setChannels] = useState<string[]>([]);
  const [selectedChannels, setSelectedChannels] = useState<Set<string>>(new Set());
  const knownChannelsRef = useRef<string[]>([]);
  const [saving, setSaving] = useState(false);
  const [loading, setLoading] = useState(true);
  const [refreshingChannels, setRefreshingChannels] = useState(false);

  const loadChannels = useCallback(async () => {
    try {
      const res = await api.getApps();
      const inst = (res.instances ?? []).find((i) => i.name === instanceName);
      const discovered = (inst?.channels ?? []).filter(
        (ch) => ch && !ch.endsWith(":general"),
      );
      setChannels(discovered);
      setSelectedChannels((prev) => {
        const known = knownChannelsRef.current;
        if (prev.size === 0 && known.length === 0) {
          // First load: select everything discovered.
          return new Set(discovered);
        }
        // Keep prior picks that still exist; only auto-select *new* channels
        // so a user deselect + Refresh does not re-check deselected ones.
        const knownSet = new Set(known);
        const next = new Set([...prev].filter((c) => discovered.includes(c)));
        for (const c of discovered) {
          if (!knownSet.has(c)) next.add(c);
        }
        return next;
      });
      knownChannelsRef.current = discovered;
    } catch {
      setChannels([]);
    }
  }, [instanceName]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const list = await api.listAgents();
        if (!cancelled) setAgents(list ?? []);
      } catch {
        if (!cancelled) setAgents([]);
      }
      if (isTelegram) {
        await loadChannels();
      }
      if (!cancelled) setLoading(false);
    })();
    return () => { cancelled = true; };
  }, [instanceName, isTelegram, loadChannels]);

  const toggleAgent = (name: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(name)) {
        next.delete(name);
        setMentionOnly((m) => { const nm = new Set(m); nm.delete(name); return nm; });
      } else {
        next.add(name);
      }
      return next;
    });
  };

  const toggleMention = (name: string) => {
    setMentionOnly((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name); else next.add(name);
      return next;
    });
  };

  const toggleChannel = (channel: string) => {
    setSelectedChannels((prev) => {
      const next = new Set(prev);
      if (next.has(channel)) next.delete(channel); else next.add(channel);
      return next;
    });
  };

  const handleRefreshChannels = async () => {
    setRefreshingChannels(true);
    await loadChannels();
    setRefreshingChannels(false);
  };

  const handleDone = async () => {
    setSaving(true);
    try {
      // Telegram: only subscribe to real discovered channels. Never invent
      // telegram:general — DMs arrive as telegram:<username|chat_id>.
      // Other apps keep the historical <instance>:general default for now.
      const targets = isTelegram
        ? [...selectedChannels]
        : [`${instanceName}:general`];

      if (targets.length > 0 && selected.size > 0) {
        await Promise.all(
          targets.flatMap((channel) =>
            [...selected].map((agent) =>
              api.subscribe(channel, agent, mentionOnly.has(agent)).catch(() => { /* best effort */ }),
            ),
          ),
        );
      }
    } catch { /* best effort */ }
    setSaving(false);
    onDone();
  };

  const stateColor = (state: string) => {
    if (state === "working" || state === "running") return "var(--mycel-success)";
    if (state === "idle") return "var(--mycel-warning)";
    return "var(--mycel-muted)";
  };

  const channelLeaf = (ch: string) => {
    const i = ch.lastIndexOf(":");
    return i >= 0 ? ch.slice(i + 1) : ch;
  };

  return (
    <div>
      <div className="p-4 border-b border-mycel-border">
        <div className="flex items-center gap-2 mb-1">
          <span className="text-[10px] font-semibold text-mycel-accent bg-mycel-accent-subtle px-2 py-0.5 rounded-full uppercase tracking-wider">Step 2</span>
        </div>
        <h3 className="text-base font-semibold text-mycel-text">Add agents to {appLabel}</h3>
        <p className="text-xs text-mycel-muted mt-1">
          {isTelegram
            ? "Select agents and the Telegram chats that should deliver to them."
            : "Select which agents should receive notifications from this app."}
        </p>
      </div>

      <div className="p-4 max-h-[340px] overflow-auto space-y-4">
        {isTelegram && (
          <div>
            <div className="flex items-center justify-between mb-2">
              <span className="text-[11px] font-semibold uppercase tracking-wider text-mycel-muted">Channels</span>
              <button
                type="button"
                onClick={() => { void handleRefreshChannels(); }}
                disabled={refreshingChannels}
                className="text-[11px] text-mycel-accent hover:underline disabled:opacity-50"
              >
                {refreshingChannels ? "Refreshing…" : "Refresh"}
              </button>
            </div>
            {loading ? (
              <div className="text-xs text-mycel-muted bg-mycel-surface-hover border border-mycel-border rounded-md px-3 py-2">
                Loading channels…
              </div>
            ) : channels.length === 0 ? (
              <div className="text-xs text-mycel-muted bg-mycel-surface-hover border border-mycel-border rounded-md px-3 py-2">
                No Telegram chats discovered yet. Message the bot in a DM (or a group),
                then click Refresh. Agents subscribed to a fake <code className="text-mycel-text-2">telegram:general</code> channel
                never receive real traffic.
              </div>
            ) : (
              <div className="space-y-1">
                {channels.map((ch) => (
                  <label
                    key={ch}
                    className="flex items-center gap-3 px-3 py-2 rounded-md hover:bg-mycel-surface-hover cursor-pointer transition-colors"
                  >
                    <input
                      type="checkbox"
                      checked={selectedChannels.has(ch)}
                      onChange={() => { toggleChannel(ch); }}
                      className="shrink-0 accent-[var(--mycel-accent)]"
                    />
                    <span className="text-sm text-mycel-text flex-1 min-w-0 truncate" title={ch}>
                      {channelLeaf(ch)}
                    </span>
                    <span className="text-[10px] text-mycel-muted font-mono truncate max-w-[40%]">{ch}</span>
                  </label>
                ))}
              </div>
            )}
          </div>
        )}

        {loading ? (
          <div className="text-center py-6 text-mycel-muted text-xs">Loading agents...</div>
        ) : agents.length === 0 ? (
          <div className="text-center py-6 text-mycel-muted text-xs">No agents found</div>
        ) : (
          <div>
            {isTelegram && (
              <div className="text-[11px] font-semibold uppercase tracking-wider text-mycel-muted mb-2">Agents</div>
            )}
            <div className="space-y-1">
              {agents.filter((a) => !a.archived_at).map((agent) => (
                <label
                  key={agent.name}
                  className="flex items-center gap-3 px-3 py-2 rounded-md hover:bg-mycel-surface-hover cursor-pointer transition-colors"
                >
                  <input
                    type="checkbox"
                    checked={selected.has(agent.name)}
                    onChange={() => { toggleAgent(agent.name); }}
                    className="shrink-0 accent-[var(--mycel-accent)]"
                  />
                  <span
                    className="shrink-0 w-2 h-2 rounded-full"
                    style={{ backgroundColor: stateColor(agent.state) }}
                    title={agent.state}
                  />
                  <span className="text-sm text-mycel-text flex-1 min-w-0 truncate">{agent.name}</span>
                  {selected.has(agent.name) && (
                    <button
                      type="button"
                      onClick={(e) => { e.preventDefault(); e.stopPropagation(); toggleMention(agent.name); }}
                      className="shrink-0 text-[10px] px-2 py-0.5 rounded-full border transition-colors"
                      style={{
                        borderColor: mentionOnly.has(agent.name) ? "color-mix(in oklab, var(--mycel-accent) 40%, transparent)" : "var(--mycel-border)",
                        color: mentionOnly.has(agent.name) ? "var(--mycel-accent)" : "var(--mycel-muted)",
                        background: mentionOnly.has(agent.name) ? "var(--mycel-accent-subtle)" : "transparent",
                      }}
                      title={mentionOnly.has(agent.name) ? "Mention only: ON" : "Mention only: OFF"}
                    >
                      @mention only
                    </button>
                  )}
                </label>
              ))}
            </div>
          </div>
        )}
      </div>

      <div className="flex justify-between items-center gap-2 p-4 border-t border-mycel-border">
        <span className="text-xs text-mycel-text-2">
          {selected.size} agent{selected.size !== 1 ? "s" : ""}
          {isTelegram ? ` · ${String(selectedChannels.size)} channel${selectedChannels.size !== 1 ? "s" : ""}` : ""} selected
        </span>
        <button
          type="button"
          onClick={() => { void handleDone(); }}
          disabled={saving}
          className="inline-flex items-center h-9 px-3 text-sm text-mycel-accent-fg bg-mycel-accent hover:bg-mycel-accent-hover shadow-mycel-sm rounded-md font-medium transition-colors disabled:opacity-50"
        >
          {saving ? "Saving..." : "Done"}
        </button>
      </div>
    </div>
  );
}

/* ── Linkify URLs in doc strings ─────────────────────────────── */

function linkifyDoc(text: string): React.ReactNode {
  const urlRe = /(https?:\/\/[^\s,)]+)/g;
  const parts = text.split(urlRe);
  if (parts.length === 1) return text;
  return parts.map((part, i) =>
    /^https?:\/\//.test(part) ? (
      <a key={i} href={part} target="_blank" rel="noopener noreferrer" style={{ color: "var(--mycel-accent)", textDecoration: "underline" }}>
        {part.replace(/^https?:\/\//, "")}
      </a>
    ) : (
      <span key={i}>{part}</span>
    ),
  );
}

/** Strip the scheme for human-readable URL labels ("github.com/login/device"). */
function humanURL(url: string): string {
  return url.replace(/^https?:\/\//, "").replace(/\/$/, "");
}

/** Short, display-safe label for an authorization URL: host + path only,
 *  never the query string. OAuth authorize URLs carry client_id,
 *  redirect_uri, scope and state as query params — sprawling, useless-to-
 *  read text that must never be rendered as the button/body copy. The
 *  full URL (query string included) is still what gets opened. */
function shortURLLabel(url: string): string {
  try {
    const u = new URL(url);
    return humanURL(`${u.origin}${u.pathname}`);
  } catch {
    return humanURL(url).split("?")[0] ?? humanURL(url);
  }
}

/** Normalize a user-typed instance label to the id-safe segment after ":". */
export function sanitizeInstanceLabel(label: string): string {
  return label.trim().toLowerCase().replace(/[^a-z0-9_-]+/g, "-").replace(/^-+|-+$/g, "");
}

/* ── Connect wizard (descriptor-driven setup + agent wiring) ─── */

export function ConnectWizard({
  appId,
  onClose,
  onConnected,
}: {
  appId: string;
  onClose: () => void;
  onConnected: () => void;
}) {
  const [descriptor, setDescriptor] = useState<AppDescriptor | null>(null);
  const [instances, setInstances] = useState<AppInstance[]>([]);
  const [catalogLoading, setCatalogLoading] = useState(true);
  const [values, setValues] = useState<Record<string, string>>({});
  const [label, setLabel] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [step, setStep] = useState<"setup" | "agents">("setup");
  const [qrDataUrl, setQrDataUrl] = useState<string | null>(null);
  const [pairState, setPairState] = useState<string>("idle");
  const qrPollRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const qrTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [oauthState, setOauthState] = useState<"idle" | "starting" | "pending" | "error">("idle");
  const [oauthSession, setOauthSession] = useState<AppAuthSession | null>(null);
  const oauthPollRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const oauthTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // The descriptor + this app's connected instances, from the catalog.
  useEffect(() => {
    let cancelled = false;
    api
      .getApps()
      .then((res) => {
        if (cancelled) return;
        const desc = (res.catalog ?? []).find((d) => d.id === appId) ?? null;
        const mine = (res.instances ?? []).filter((i) => i.app === appId);
        setDescriptor(desc);
        setInstances(mine);
        // Seed plain fields from the connected instance so reopening an
        // app doesn't present blank required inputs (secrets stay blank —
        // replace-only semantics).
        const current = mine.find((i) => i.name === appId) ?? mine[0];
        if (current?.config) {
          setValues((prev) => {
            const seeded = { ...prev };
            for (const f of desc?.fields ?? []) {
              const v = current.config?.[f.key];
              if (!f.secret && seeded[f.key] === undefined && v !== undefined && v !== "") {
                seeded[f.key] = String(v);
              }
            }
            return seeded;
          });
        }
      })
      .catch(() => { /* handled by the unknown-app state */ })
      .finally(() => { if (!cancelled) setCatalogLoading(false); });
    return () => { cancelled = true; };
  }, [appId]);

  // Escape to close
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", handler);
    return () => { document.removeEventListener("keydown", handler); };
  }, [onClose]);

  // Cleanup QR/OAuth poll intervals on unmount.
  useEffect(() => {
    return () => {
      if (qrPollRef.current) clearInterval(qrPollRef.current);
      if (qrTimeoutRef.current) clearTimeout(qrTimeoutRef.current);
      if (oauthPollRef.current) clearInterval(oauthPollRef.current);
      if (oauthTimeoutRef.current) clearTimeout(oauthTimeoutRef.current);
    };
  }, []);

  // Instance this connect targets: "<app>" or "<app>:<label>" for multi.
  const cleanLabel = sanitizeInstanceLabel(label);
  const instanceName = descriptor?.multi && cleanLabel !== "" ? `${appId}:${cleanLabel}` : appId;
  const existing = instances.find((i) => i.name === instanceName);
  const isUpdate = Boolean(existing);

  /** True when the vault already holds a value for this secret field. */
  const hasStoredSecret = (key: string): boolean =>
    existing?.config?.[`has_${key}`] === true;

  /** One descriptor field → labeled input (secret fields are password
   *  inputs with replace-only, never-echoed semantics). */
  const renderField = (field: AppDescriptor["fields"][number]) => {
    const configured = field.secret && hasStoredSecret(field.key);
    return (
      <div key={field.key}>
        <label className="block text-sm font-medium text-mycel-text-2 mb-1">
          {field.label}
          {!field.required && <span className="text-mycel-muted ml-1 font-normal">(optional)</span>}
          {configured && (
            <span className="ml-2 inline-flex items-center gap-1 text-[10px] font-medium text-mycel-success">
              <span className="w-1 h-1 rounded-full bg-mycel-success" />
              configured
            </span>
          )}
        </label>
        <input
          type={field.secret ? "password" : "text"}
          value={values[field.key] ?? ""}
          onChange={(e) => { setValues((v) => ({ ...v, [field.key]: e.target.value })); }}
          placeholder={configured ? "•••••• — leave blank to keep" : field.placeholder}
          autoComplete={field.secret ? "new-password" : "off"}
          className="w-full px-3 py-2 bg-mycel-surface border border-mycel-border rounded-md text-sm text-mycel-text placeholder:text-mycel-muted focus:border-mycel-accent focus:outline-none transition-colors"
        />
      </div>
    );
  };

  const startQRPairing = async () => {
    setPairState("loading");
    setError(null);
    try {
      const data = await api.startAppAuth(instanceName);
      if (data.state === "connected") { setPairState("connected"); onConnected(); return; }
      if (data.state === "error") { setError(data.error ?? "Failed to start pairing"); setPairState("error"); return; }
      if (data.qr_data_url) { setQrDataUrl(data.qr_data_url); setPairState("qr_ready"); }
      // Poll for connection.
      const pollId = setInterval(() => {
        void api.getAppAuthStatus(instanceName).then((s) => {
          if (s.state === "connected") {
            clearInterval(pollId); qrPollRef.current = null;
            setPairState("connected"); onConnected();
          } else if (s.state === "error") {
            clearInterval(pollId); qrPollRef.current = null;
            setPairState("error"); setError(s.error ?? "Pairing failed");
          }
        }).catch(() => { /* transient poll error — keep polling */ });
      }, 2000);
      qrPollRef.current = pollId;
      // Stop polling after 2 minutes.
      const timeoutId = setTimeout(() => { clearInterval(pollId); qrPollRef.current = null; }, 120000);
      qrTimeoutRef.current = timeoutId;
    } catch (e) { setError(e instanceof Error ? e.message : String(e)); setPairState("error"); }
  };

  /** Begin the browser sign-in (OAuth). Plain fields already typed into
   *  the form (e.g. the OAuth client ID) ride along and persist with the
   *  instance; the server drives the flow and stores the credentials, so
   *  on "complete" we advance straight to the agents step. */
  const startOAuth = async () => {
    if (!descriptor) return;
    setOauthState("starting");
    setError(null);
    try {
      const config: Record<string, string> = {};
      for (const field of descriptor.fields) {
        if (field.secret) continue;
        const val = (values[field.key] ?? "").trim();
        if (val !== "") config[field.key] = val;
      }
      const session = await api.beginAppOAuth(instanceName, config);
      setOauthSession(session);
      setOauthState("pending");
      // One click really means one click: open the system browser the
      // moment the session exists, instead of making the user find and
      // click a second link. openExternal itself picks Wails'
      // BrowserOpenURL vs window.open, so this works in the desktop app
      // too. The "Reopen browser" control below covers popup blockers /
      // an accidental close.
      const authURL = session.verification_url ?? session.auth_url;
      if (authURL) openExternal(authURL);
      // Poll for completion. The plugin rate-limits upstream calls to the
      // provider's interval, so a snappy local poll is safe.
      const pollId = setInterval(() => {
        void api.getAppOAuthStatus(instanceName, session.id).then((s) => {
          if (s.state === "complete") {
            clearInterval(pollId); oauthPollRef.current = null;
            onConnected();
            setStep("agents");
          } else if (s.state === "error") {
            clearInterval(pollId); oauthPollRef.current = null;
            setOauthState("error"); setError(s.error ?? "Sign-in failed");
          }
        }).catch(() => { /* transient poll error — keep polling */ });
      }, 2000);
      oauthPollRef.current = pollId;
      // Stop polling after 10 minutes (device codes expire well before).
      const timeoutId = setTimeout(() => { clearInterval(pollId); oauthPollRef.current = null; }, 600000);
      oauthTimeoutRef.current = timeoutId;
    } catch (e) { setError(e instanceof Error ? e.message : String(e)); setOauthState("error"); }
  };

  const handleSave = async () => {
    if (!descriptor) return;
    setSaving(true);
    setError(null);
    try {
      const config: Record<string, string> = {};
      for (const field of descriptor.fields) {
        const val = (values[field.key] ?? "").trim();
        if (val === "") {
          // A stored value stays valid when the input is left blank —
          // secrets by replace-only semantics, plain fields because the
          // server merges from the existing instance config.
          const stored = field.secret
            ? hasStoredSecret(field.key)
            : Boolean(existing?.config?.[field.key]);
          if (field.required && !stored) {
            setError(`${field.label} is required`);
            setSaving(false);
            return;
          }
          continue;
        }
        config[field.key] = val;
      }

      await api.connectApp(instanceName, { app: descriptor.id, enabled: true, config });
      setStep("agents");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save");
    }
    setSaving(false);
  };

  const handleAgentsDone = () => {
    onConnected();
    onClose();
  };

  if (!descriptor) {
    return createPortal(
      <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-mycel-overlay backdrop-blur-sm">
        <div className="bg-mycel-surface-2 border border-mycel-border rounded-lg p-6 max-w-md w-full mx-4 shadow-mycel-lg">
          <p className="text-mycel-muted">{catalogLoading ? "Loading app catalog…" : `Unknown app: ${appId}`}</p>
          <button type="button" onClick={onClose} className="mt-4 text-sm text-mycel-accent">
            Close
          </button>
        </div>
      </div>,
      document.body,
    );
  }

  const isQR = descriptor.auth === "qr";
  const oauthAvailable = !isQR && descriptor.oauth_available === true;

  return createPortal(
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-mycel-overlay backdrop-blur-sm" style={{ animation: "fadeIn 120ms ease-out" }}>
      <div className="bg-mycel-surface-2 border border-mycel-border rounded-lg max-w-lg w-full mx-4 max-h-[85vh] overflow-auto shadow-mycel-lg">
        {/* Header */}
        <div className="flex items-center justify-between p-4 border-b border-mycel-border">
          <h2 className="text-base font-semibold text-mycel-text flex items-center gap-2">
            <AppGlyph appId={appId} size={18} />
            {step === "setup" ? `Connect ${descriptor.label}` : `${descriptor.label} Setup`}
            {existing?.connected && <StatusDot status="connected" title="Connected" />}
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="w-8 h-8 flex items-center justify-center rounded-lg text-mycel-muted hover:text-mycel-text hover:bg-mycel-surface-hover transition-colors"
            aria-label="Close"
          >
            <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
              <path d="M4 4l8 8M12 4l-8 8" />
            </svg>
          </button>
        </div>

        {/* Step indicator */}
        <div className="px-4 pt-3 flex items-center gap-2">
          <span
            className="text-[10px] font-semibold px-2 py-0.5 rounded-full uppercase tracking-wider"
            style={{
              color: step === "setup" ? "var(--mycel-accent)" : "var(--mycel-success)",
              background: step === "setup" ? "var(--mycel-accent-subtle)" : "var(--mycel-success-subtle)",
            }}
          >
            Step 1
          </span>
          <span className="text-xs text-mycel-muted">
            {step === "setup" ? (isQR ? "Pair device" : "Configure") : "Connected"}
          </span>
          <span className="text-mycel-muted mx-1">&rarr;</span>
          <span
            className="text-[10px] font-semibold px-2 py-0.5 rounded-full uppercase tracking-wider"
            style={{
              color: step === "agents" ? "var(--mycel-accent)" : "var(--mycel-muted)",
              background: step === "agents" ? "var(--mycel-accent-subtle)" : "transparent",
              border: step === "agents" ? "none" : "1px solid var(--mycel-border)",
            }}
          >
            Step 2
          </span>
          <span className="text-xs text-mycel-muted">Add agents</span>
        </div>

        {step === "setup" ? (
          <>
            {/* Setup docs from the descriptor. When one-click sign-in is
                available these move into the Advanced disclosure below,
                collapsed alongside the manual fields — a wall of numbered
                setup steps must never out-compete a working Sign in
                button for attention. */}
            {!oauthAvailable && descriptor.docs.length > 0 && (
              <div className="p-4 border-b border-mycel-border">
                <h3 className="text-[11px] font-medium text-mycel-muted uppercase tracking-[0.08em] mb-2">
                  Setup Steps
                </h3>
                <ol className="space-y-1.5">
                  {descriptor.docs.map((docStep, i) => (
                    <li key={i} className="flex gap-2 text-xs text-mycel-text-2">
                      <span className="text-mycel-accent tabular-nums shrink-0">{i + 1}.</span>
                      <span>{linkifyDoc(docStep)}</span>
                    </li>
                  ))}
                </ol>
              </div>
            )}

            {/* QR pairing flow */}
            {isQR ? (
              <div className="p-6 flex flex-col items-center gap-4">
                {pairState === "idle" && (
                  <button
                    type="button"
                    onClick={() => { void startQRPairing(); }}
                    className="inline-flex items-center h-9 px-6 bg-mycel-accent hover:bg-mycel-accent-hover text-mycel-accent-fg rounded-md font-medium text-sm shadow-mycel-sm transition-colors"
                  >
                    Generate QR Code
                  </button>
                )}
                {pairState === "loading" && (
                  <div className="text-mycel-muted text-sm animate-pulse">Generating QR code...</div>
                )}
                {pairState === "error" && (
                  <button
                    type="button"
                    onClick={() => { void startQRPairing(); }}
                    className="inline-flex items-center h-9 px-6 bg-mycel-accent hover:bg-mycel-accent-hover text-mycel-accent-fg rounded-md font-medium text-sm shadow-mycel-sm transition-colors"
                  >
                    Try again
                  </button>
                )}
                {pairState === "qr_ready" && qrDataUrl && (
                  <div className="flex flex-col items-center gap-3">
                    <img src={qrDataUrl} alt={`${descriptor.label} QR code`} className="w-56 h-56 rounded-lg border border-mycel-border" />
                    <p className="text-xs text-mycel-muted text-center">
                      Open {descriptor.label} &rarr; Linked Devices &rarr; Link a Device<br />
                      Scan this QR code with your phone
                    </p>
                    <div className="flex items-center gap-2 text-xs text-mycel-muted">
                      <span className="w-2 h-2 bg-mycel-warning rounded-full animate-pulse" />
                      Waiting for scan...
                    </div>
                  </div>
                )}
                {pairState === "connected" && (
                  <div className="flex flex-col items-center gap-3">
                    <div className="flex items-center gap-2 text-sm text-mycel-success">
                      <span className="text-lg">✓</span> {descriptor.label} connected!
                    </div>
                    <button
                      type="button"
                      onClick={() => { setStep("agents"); }}
                      className="inline-flex items-center h-9 px-3 text-sm text-mycel-accent-fg bg-mycel-accent hover:bg-mycel-accent-hover shadow-mycel-sm rounded-md font-medium transition-colors"
                    >
                      Add agents &rarr;
                    </button>
                  </div>
                )}
              </div>
            ) : (
              /* Descriptor-driven config fields */
              <div className="p-4 space-y-3">
                {descriptor.multi && (
                  <div>
                    <label className="block text-sm font-medium text-mycel-text-2 mb-1">
                      Instance label
                      <span className="text-mycel-muted ml-1 font-normal">(optional — for multiple {descriptor.label} connections)</span>
                    </label>
                    <input
                      type="text"
                      value={label}
                      onChange={(e) => { setLabel(e.target.value); }}
                      placeholder="alerts"
                      data-testid="instance-label-input"
                      className="w-full px-3 py-2 bg-mycel-surface border border-mycel-border rounded-md text-sm text-mycel-text placeholder:text-mycel-muted focus:border-mycel-accent focus:outline-none transition-colors"
                    />
                    {cleanLabel !== "" && (
                      <p className="mt-1 text-[11px] text-mycel-muted font-mono">{instanceName}</p>
                    )}
                  </div>
                )}

                {/* Browser sign-in (OAuth) — offered when the backend
                    plugin implements the flow; manual fields stay below. */}
                {oauthAvailable && (
                  <div data-testid="oauth-panel" className="rounded-md border border-mycel-border bg-mycel-surface p-4">
                    {oauthState === "pending" && oauthSession ? (
                      (() => {
                        const authURL = oauthSession.verification_url ?? oauthSession.auth_url ?? "";
                        return (
                          <div className="flex flex-col items-center gap-3">
                            {oauthSession.user_code ? (
                              <>
                                <p className="text-xs text-mycel-muted text-center">
                                  Enter this code in your browser to authorize mycel
                                </p>
                                <div
                                  data-testid="oauth-user-code"
                                  className="font-mono text-2xl font-semibold tracking-[0.25em] text-mycel-text bg-mycel-surface-2 border border-mycel-border rounded-md px-5 py-2.5 select-all"
                                >
                                  {oauthSession.user_code}
                                </div>
                              </>
                            ) : (
                              <p className="text-xs text-mycel-muted text-center">
                                Continue the sign-in in your browser
                              </p>
                            )}
                            {authURL !== "" && (
                              <a
                                href={authURL}
                                onClick={(e) => { e.preventDefault(); openExternal(authURL); }}
                                className="inline-flex items-center justify-center h-11 min-w-[44px] px-4 bg-mycel-accent hover:bg-mycel-accent-hover text-mycel-accent-fg rounded-md font-medium text-sm shadow-mycel-sm transition-colors"
                              >
                                Open {shortURLLabel(authURL)} &rarr;
                              </a>
                            )}
                            <div role="status" className="flex items-center gap-2 text-xs text-mycel-muted">
                              <span className="w-2 h-2 bg-mycel-warning rounded-full animate-pulse" aria-hidden />
                              Waiting for authorization…
                              {authURL !== "" && (
                                <>
                                  <span aria-hidden>·</span>
                                  <button
                                    type="button"
                                    onClick={() => { openExternal(authURL); }}
                                    className="text-mycel-accent hover:underline font-medium"
                                  >
                                    Reopen browser
                                  </button>
                                </>
                              )}
                            </div>
                          </div>
                        );
                      })()
                    ) : (
                      <div className="flex flex-col items-center gap-2">
                        <button
                          type="button"
                          onClick={() => { void startOAuth(); }}
                          disabled={oauthState === "starting"}
                          className="inline-flex items-center justify-center h-11 min-w-[44px] px-6 bg-mycel-accent hover:bg-mycel-accent-hover text-mycel-accent-fg rounded-md font-medium text-sm shadow-mycel-sm transition-colors disabled:opacity-50"
                        >
                          {oauthState === "starting" ? "Starting sign-in..." : oauthState === "error" ? `Retry sign in with ${descriptor.label}` : `Sign in with ${descriptor.label}`}
                        </button>
                        <p className="text-[11px] text-mycel-muted text-center">
                          Authorize in your browser — no token pasting needed
                        </p>
                      </div>
                    )}
                  </div>
                )}
                {/* Manual config fields. When browser sign-in is available
                    they collapse under an Advanced disclosure so the
                    one-click path stays front-and-centre; otherwise they are
                    the primary path and render inline. */}
                {oauthAvailable ? (
                  <details data-testid="manual-config" className="mycel-disclosure">
                    <summary className="flex items-center gap-3 text-[10px] uppercase tracking-[0.08em] text-mycel-muted cursor-pointer select-none list-none marker:content-none hover:text-mycel-text-2 transition-colors">
                      <span className="flex-1 border-t border-mycel-border" />
                      <span className="whitespace-nowrap">Advanced — configure manually or paste a token</span>
                      <span className="flex-1 border-t border-mycel-border" />
                    </summary>
                    <div className="space-y-3 pt-3">
                      {descriptor.docs.length > 0 && (
                        <ol className="space-y-1.5 mb-1">
                          {descriptor.docs.map((docStep, i) => (
                            <li key={i} className="flex gap-2 text-xs text-mycel-text-2">
                              <span className="text-mycel-accent tabular-nums shrink-0">{i + 1}.</span>
                              <span>{linkifyDoc(docStep)}</span>
                            </li>
                          ))}
                        </ol>
                      )}
                      {descriptor.fields.map(renderField)}
                    </div>
                  </details>
                ) : (
                  <>
                    {descriptor.fields.map(renderField)}
                    {descriptor.fields.length === 0 && (
                      <p className="text-xs text-mycel-muted">
                        No configuration needed — connect to start receiving events.
                      </p>
                    )}
                  </>
                )}
              </div>
            )}

            {/* Error */}
            {error && (
              <div role="alert" className="mx-4 mb-3 px-3 py-2 bg-mycel-error-subtle border border-mycel-error rounded-md text-xs text-mycel-error">
                {error}
              </div>
            )}

            {/* Actions */}
            {!isQR && (
              <div className="flex justify-end gap-2 p-4 border-t border-mycel-border">
                <button
                  type="button"
                  onClick={onClose}
                  className="inline-flex items-center h-9 px-3 text-sm bg-mycel-surface border border-mycel-border text-mycel-text-2 hover:text-mycel-text hover:bg-mycel-surface-hover rounded-md transition-colors"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  onClick={() => { void handleSave(); }}
                  disabled={saving}
                  className="inline-flex items-center h-9 px-3 text-sm text-mycel-accent-fg bg-mycel-accent hover:bg-mycel-accent-hover shadow-mycel-sm rounded-md font-medium transition-colors disabled:opacity-50"
                >
                  {saving ? "Connecting..." : isUpdate ? "Save & reconnect" : "Connect"}
                </button>
              </div>
            )}
          </>
        ) : (
          <AgentSubscriptionStep
            instanceName={instanceName}
            appLabel={descriptor.label}
            onDone={handleAgentsDone}
          />
        )}
      </div>
    </div>,
    document.body,
  );
}
