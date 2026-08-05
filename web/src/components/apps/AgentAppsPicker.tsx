/**
 * AgentAppsPicker — the "Apps" section of the New Agent flow.
 *
 * Lists the connected app instances (platform icon + status dot, same
 * visual system as the drawer tree) with their discovered channels as
 * checkboxes. The caller collects the selected channel keys and wires
 * the subscriptions after the agent is created.
 */

import { useEffect, useState } from "react";
import { api } from "../../api/client";
import type { AppInstance } from "../../api/client";
import { DefaultAppIcon, PLATFORM_ICON_MAP } from "./PlatformIcons";
import { StatusDot } from "./appStatus";

function instanceBase(name: string): string {
  const i = name.indexOf(":");
  return i >= 0 ? name.slice(0, i) : name;
}

function channelLeaf(ch: string): string {
  const i = ch.lastIndexOf(":");
  const leaf = i >= 0 ? ch.slice(i + 1) : ch;
  return leaf === "*" ? "catch-all" : leaf;
}

export function AgentAppsPicker({
  selected,
  onChange,
}: {
  /** Selected bc channel keys ("telegram:alerts:standup"). */
  selected: Set<string>;
  onChange: (next: Set<string>) => void;
}) {
  const [instances, setInstances] = useState<AppInstance[] | null>(null);

  useEffect(() => {
    let cancelled = false;
    api
      .getApps()
      .then((res) => {
        if (cancelled) return;
        setInstances((res.instances ?? []).filter((i) => i.enabled));
      })
      .catch(() => {
        if (!cancelled) setInstances([]);
      });
    return () => { cancelled = true; };
  }, []);

  const toggle = (channel: string) => {
    const next = new Set(selected);
    if (next.has(channel)) next.delete(channel); else next.add(channel);
    onChange(next);
  };

  if (instances === null) {
    return (
      <div className="text-xs text-mycel-muted px-1 py-2" data-testid="agent-apps-picker">
        Loading apps…
      </div>
    );
  }

  if (instances.length === 0) {
    return (
      <div
        className="text-xs text-mycel-muted bg-mycel-bg border border-mycel-border rounded-md px-3 py-2"
        data-testid="agent-apps-picker"
      >
        No apps connected yet — connect Slack, Telegram, WhatsApp and more on the Apps page,
        then wire their channels to agents here.
      </div>
    );
  }

  return (
    <div
      className="rounded-md border border-mycel-border bg-mycel-bg divide-y divide-mycel-border max-h-48 overflow-y-auto"
      data-testid="agent-apps-picker"
    >
      {instances.map((inst) => {
        const base = instanceBase(inst.name);
        const Icon = PLATFORM_ICON_MAP[base] ?? DefaultAppIcon;
        const channels = inst.channels ?? [];
        const pickedHere = channels.filter((c) => selected.has(c)).length;
        return (
          <div key={inst.name} className="px-3 py-2">
            <div className="flex items-center gap-2">
              <Icon size={13} />
              <span className="text-xs font-medium text-mycel-text truncate">{inst.name}</span>
              {inst.bot_name && (
                <span className="text-[10px] text-mycel-muted truncate">· {inst.bot_name}</span>
              )}
              <span className="ml-auto flex items-center gap-2 shrink-0">
                {pickedHere > 0 && (
                  <span className="text-[10px] text-mycel-accent tabular-nums">{pickedHere} selected</span>
                )}
                <StatusDot
                  status={inst.connected ? "connected" : inst.error ? "error" : "idle"}
                  title={inst.connected ? "Connected" : inst.error || "Not connected"}
                />
              </span>
            </div>
            {channels.length === 0 ? (
              <p className="mt-1 ml-[21px] text-[10.5px] text-mycel-muted">
                No channels discovered yet
              </p>
            ) : (
              <div className="mt-1 ml-[13px] space-y-0.5">
                {channels.map((ch) => (
                  <label
                    key={ch}
                    className="flex items-center gap-2 px-2 py-1 rounded-md hover:bg-mycel-surface-hover cursor-pointer transition-colors"
                    title={ch}
                  >
                    <input
                      type="checkbox"
                      checked={selected.has(ch)}
                      onChange={() => { toggle(ch); }}
                      className="shrink-0 accent-[var(--mycel-accent)]"
                    />
                    <span className="text-xs text-mycel-text-2 truncate">{channelLeaf(ch)}</span>
                  </label>
                ))}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
