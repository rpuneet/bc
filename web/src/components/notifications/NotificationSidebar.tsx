import { useState, useEffect, useCallback } from "react";
import type { NotificationSource, GatewayStatus, NotifySubscription } from "../../api/client";
import { api } from "../../api/client";
import { sourcePlatform } from "./messageUtils";
import { SetupWizard, PlatformChooser, PLATFORM_MAP, PLATFORMS } from "./SetupWizard";

function getMeta(p: string) {
  const def = PLATFORM_MAP[p];
  if (def) return { label: def.label, color: def.color };
  return { label: p, color: "#8c7e72" };
}

function displayName(name: string): string {
  // Show only the leaf channel segment, e.g.:
  //   "discord:server-name:general" → "general"
  //   "slack:engineering"           → "engineering"
  const parts = name.split(":");
  return parts[parts.length - 1] || name;
}

/* ── Platform glyphs ─────────────────────────────────────── */

function SlackGlyph({ size = 11 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" style={{ flexShrink: 0 }}>
      <path d="M6 15a2 2 0 1 1-2-2h2v2zm1 0a2 2 0 0 1 4 0v5a2 2 0 0 1-4 0v-5z" fill="#E01E5A" />
      <path d="M9 6a2 2 0 1 1 2-2v2H9zm0 1a2 2 0 0 1 0 4H4a2 2 0 0 1 0-4h5z" fill="#36C5F0" />
      <path d="M18 9a2 2 0 1 1 2 2h-2V9zm-1 0a2 2 0 0 1-4 0V4a2 2 0 0 1 4 0v5z" fill="#2EB67D" />
      <path d="M15 18a2 2 0 1 1-2 2v-2h2zm0-1a2 2 0 0 1 0-4h5a2 2 0 0 1 0 4h-5z" fill="#ECB22E" />
    </svg>
  );
}

function TelegramGlyph({ size = 11 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" style={{ flexShrink: 0 }}>
      <circle cx="12" cy="12" r="11" fill="#229ED9" />
      <path d="M5.5 11.5l12-4.5-2 12-4-3-2 2-1-3.5 7-5.5-8 3.5-2-1z" fill="#fff" />
    </svg>
  );
}

function DiscordGlyph({ size = 11 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" style={{ flexShrink: 0 }}>
      <circle cx="12" cy="12" r="11" fill="#5865F2" />
      <text x="12" y="16" textAnchor="middle" fill="#fff" fontSize="10" fontWeight="bold">D</text>
    </svg>
  );
}

function GitHubGlyph({ size = 11 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" style={{ flexShrink: 0 }}>
      <path fill="var(--mycel-text)" d="M12 2a10 10 0 0 0-3.16 19.49c.5.09.66-.22.66-.48v-1.7c-2.78.6-3.37-1.34-3.37-1.34-.45-1.15-1.1-1.46-1.1-1.46-.9-.62.07-.6.07-.6 1 .07 1.53 1.03 1.53 1.03.89 1.52 2.34 1.08 2.91.83.09-.65.35-1.08.63-1.33-2.22-.25-4.56-1.11-4.56-4.94 0-1.1.39-1.99 1.03-2.69-.1-.25-.45-1.27.1-2.65 0 0 .84-.27 2.75 1.02a9.5 9.5 0 0 1 5 0c1.91-1.29 2.75-1.02 2.75-1.02.55 1.38.2 2.4.1 2.65.64.7 1.03 1.59 1.03 2.69 0 3.84-2.34 4.69-4.57 4.93.36.31.68.92.68 1.85v2.74c0 .27.16.58.67.48A10 10 0 0 0 12 2z" />
    </svg>
  );
}

const PLATFORM_GLYPHS: Record<string, React.FC<{ size?: number }>> = {
  slack: SlackGlyph,
  telegram: TelegramGlyph,
  discord: DiscordGlyph,
  github: GitHubGlyph,
};

/* ── Channel icon ────────────────────────────────────────── */

function ChannelIcon({ name }: { name: string }) {
  // Bot-style names
  if (name.startsWith("@")) {
    return (
      <span className="shrink-0 flex items-center justify-center" style={{ width: 12, color: "var(--mycel-muted)" }}>
        <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round">
          <rect x="4" y="8" width="16" height="12" rx="2" />
          <circle cx="9" cy="14" r="1" fill="currentColor" />
          <circle cx="15" cy="14" r="1" fill="currentColor" />
          <line x1="12" y1="4" x2="12" y2="8" />
          <circle cx="12" cy="3" r="1" fill="currentColor" />
        </svg>
      </span>
    );
  }
  // Default: hash for channels
  return (
    <span
      className="shrink-0 flex items-center justify-center"
      style={{ width: 12, color: "var(--mycel-muted)", fontFamily: "'JetBrains Mono', ui-monospace, monospace", fontSize: 12 }}
    >
      #
    </span>
  );
}

export function NotificationSidebar({
  channels,
  selected,
  onSelect,
}: {
  channels: NotificationSource[];
  selected: string | null;
  onSelect: (name: string) => void;
}) {
  const [gateways, setGateways] = useState<GatewayStatus[]>([]);
  const [allSubs, setAllSubs] = useState<NotifySubscription[]>([]);
  const [setupPlatform, setSetupPlatform] = useState<string | null>(null);
  const [expandedGw, setExpandedGw] = useState<Set<string>>(new Set(["slack", "telegram", "discord"]));

  const fetchData = useCallback(async () => {
    try {
      const [gw, subs] = await Promise.all([
        api.listGateways(),
        api.listSubscriptions().catch(() => []),
      ]);
      setGateways(gw ?? []);
      setAllSubs(subs ?? []);
    } catch { /* */ }
  }, []);

  useEffect(() => {
    void fetchData();
    const interval = setInterval(() => void fetchData(), 12000);
    return () => clearInterval(interval);
  }, [fetchData]);

  const toggleGw = (p: string) => {
    setExpandedGw((prev) => {
      const next = new Set(prev);
      if (next.has(p)) next.delete(p); else next.add(p);
      return next;
    });
  };

  // Subscription counts
  const subCountMap = new Map<string, number>();
  for (const sub of allSubs) {
    subCountMap.set(sub.channel, (subCountMap.get(sub.channel) ?? 0) + 1);
  }

  // Build gateway buckets
  const gwMap = new Map<string, GatewayStatus>();
  for (const gw of gateways) gwMap.set(gw.platform, gw);

  const bucketMap = new Map<string, NotificationSource[]>();
  for (const ch of channels) {
    const p = sourcePlatform(ch.name);
    if (p === "internal") continue;
    const list = bucketMap.get(p) ?? [];
    list.push(ch);
    bucketMap.set(p, list);
  }
  for (const gw of gateways) {
    if (!bucketMap.has(gw.platform)) bucketMap.set(gw.platform, []);
  }

  const configuredPlatforms = new Set(bucketMap.keys());
  const unconfigured = PLATFORMS.filter(p => !configuredPlatforms.has(p.key));

  return (
    <nav
      className="shrink-0 flex flex-col h-full overflow-hidden"
      style={{
        width: 228,
        minWidth: 228,
        background: "var(--mycel-surface)",
        borderRight: "1px solid var(--mycel-border)",
        scrollbarWidth: "thin",
        scrollbarColor: "var(--mycel-border) transparent",
      }}
    >
      {/* Scrollable channel tree */}
      <div
        className="flex-1 overflow-auto"
        style={{ padding: "4px 8px 8px", scrollbarWidth: "thin", scrollbarColor: "var(--mycel-border) transparent" }}
      >
        {[...bucketMap.entries()].map(([platform, chs]) => {
          const meta = getMeta(platform);
          const gwStatus = gwMap.get(platform);
          const isConnected = (gwStatus?.enabled && (gwStatus?.channels?.length ?? 0) > 0) || chs.length > 0;
          const isExpanded = expandedGw.has(platform);
          const Glyph = PLATFORM_GLYPHS[platform];

          return (
            <div key={platform}>
              {/* Platform header row */}
              <button
                type="button"
                onClick={() => toggleGw(platform)}
                className="w-full flex items-center gap-2"
                style={{
                  padding: "5px 8px 2px",
                  fontSize: 11,
                  color: "var(--mycel-muted)",
                  textTransform: "uppercase",
                  letterSpacing: "0.08em",
                  fontWeight: 500,
                  background: "none",
                  border: "none",
                  cursor: "pointer",
                }}
              >
                {Glyph && <Glyph size={11} />}
                <span>{meta.label}</span>
                {/* Connection status dot */}
                {isConnected && (
                  <span
                    className="ml-auto shrink-0"
                    style={{
                      width: 5,
                      height: 5,
                      borderRadius: 999,
                      background: "var(--mycel-success)",
                      boxShadow: "0 0 5px color-mix(in oklab, var(--mycel-success) 50%, transparent)",
                    }}
                  />
                )}
              </button>

              {/* Channel list */}
              {isExpanded && (
                <div
                  style={{
                    paddingLeft: 10,
                    marginLeft: 9,
                    borderLeft: "1px solid var(--mycel-border)",
                    marginTop: 2,
                    marginBottom: 4,
                  }}
                >
                  {chs.length === 0 && (
                    <div style={{ padding: "4px 8px", fontSize: 11, color: "var(--mycel-muted)", fontStyle: "italic" }}>
                      No notifications
                    </div>
                  )}
                  {chs.map((ch) => {
                    const isActive = selected === ch.name;
                    const count = subCountMap.get(ch.name) ?? 0;
                    const name = displayName(ch.name);

                    return (
                      <button
                        key={ch.name}
                        onClick={() => onSelect(ch.name)}
                        className="w-full flex items-center"
                        style={{
                          gap: 8,
                          height: 26,
                          padding: "0 8px",
                          borderRadius: 6,
                          fontSize: 13,
                          color: isActive ? "var(--mycel-text)" : count > 0 ? "var(--mycel-text)" : "var(--mycel-text-2)",
                          background: isActive ? "var(--mycel-accent-subtle)" : "transparent",
                          fontWeight: isActive ? 600 : count > 0 ? 500 : 400,
                          cursor: "pointer",
                          border: "none",
                          marginBottom: 1,
                          textAlign: "left",
                        }}
                      >
                        <ChannelIcon name={name} />
                        <span
                          style={{
                            flex: 1,
                            minWidth: 0,
                            overflow: "hidden",
                            textOverflow: "ellipsis",
                            whiteSpace: "nowrap",
                          }}
                        >
                          {name}
                        </span>
                        {count > 0 && (
                          <span
                            style={{
                              fontSize: 10.5,
                              fontWeight: 500,
                              color: "var(--mycel-text-2)",
                              fontVariantNumeric: "tabular-nums",
                              padding: "1px 5px",
                              borderRadius: 999,
                              background: "var(--mycel-surface-hover)",
                            }}
                          >
                            {count}
                          </span>
                        )}
                      </button>
                    );
                  })}
                </div>
              )}
            </div>
          );
        })}

        {/* Unconfigured platforms */}
        {unconfigured.length > 0 && (
          <div style={{ marginTop: 4 }}>
            {unconfigured.slice(0, 3).map((p) => (
              <button
                key={p.key}
                type="button"
                onClick={() => setSetupPlatform(p.key)}
                className="w-full flex items-center"
                style={{
                  gap: 8,
                  height: 24,
                  padding: "0 8px",
                  borderRadius: 6,
                  fontSize: 11,
                  color: "var(--mycel-muted)",
                  cursor: "pointer",
                  background: "none",
                  border: "none",
                  textAlign: "left",
                }}
              >
                <span style={{ width: 12, textAlign: "center" }}>+</span>
                <span style={{ textTransform: "uppercase", letterSpacing: "0.08em", fontWeight: 500 }}>
                  {p.label}
                </span>
              </button>
            ))}
          </div>
        )}
      </div>

      {/* Bottom buttons */}
      <div
        style={{
          borderTop: "1px solid var(--mycel-border)",
          padding: "8px",
          display: "flex",
          flexDirection: "column",
          gap: 4,
          flexShrink: 0,
        }}
      >
        {/* Connect app button */}
        <button
          type="button"
          onClick={() => setSetupPlatform("_choose")}
          className="w-full flex items-center"
          style={{
            gap: 8,
            height: 28,
            padding: "0 8px",
            borderRadius: 6,
            fontSize: 12,
            color: "var(--mycel-muted)",
            cursor: "pointer",
            border: "1px dashed var(--mycel-border)",
            background: "none",
            whiteSpace: "nowrap",
          }}
        >
          <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
            <line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" />
          </svg>
          <span>Connect app</span>
        </button>
      </div>

      {/* Setup wizard */}
      {setupPlatform && setupPlatform !== "_choose" && (
        <SetupWizard platform={setupPlatform} onClose={() => setSetupPlatform(null)} onConnected={() => void fetchData()} />
      )}
      {setupPlatform === "_choose" && (
        <PlatformChooser
          onSelect={(key) => setSetupPlatform(key)}
          onClose={() => setSetupPlatform(null)}
        />
      )}
    </nav>
  );
}
