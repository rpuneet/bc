import { useState, useEffect, useCallback, useRef } from "react";

import { MONO } from "../utils/typography";

const INPUT_CLS =
  "w-full bg-mycel-bg border border-mycel-border rounded px-3 py-2 text-sm text-mycel-text " +
  "placeholder:text-mycel-muted outline-none focus:border-mycel-accent transition-colors";

interface LoopConfig {
  enabled: boolean;
  prompt: string;
}

// ── API-backed loop config (server-side, persists without browser) ──

async function getLoopConfig(agentName: string): Promise<LoopConfig> {
  try {
    const r = await fetch(`/api/agents/${encodeURIComponent(agentName)}/loop`);
    if (r.ok) {
      const data: unknown = await r.json();
      if (
        data !== null &&
        typeof data === "object" &&
        "enabled" in data &&
        "prompt" in data &&
        typeof (data as Record<string, unknown>).enabled === "boolean" &&
        typeof (data as Record<string, unknown>).prompt === "string"
      ) {
        return data as LoopConfig;
      }
    }
  } catch {
    /* ignore */
  }
  return { enabled: false, prompt: "" };
}

async function setLoopConfig(agentName: string, config: LoopConfig): Promise<void> {
  await fetch(`/api/agents/${encodeURIComponent(agentName)}/loop`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(config),
  });
}

// ── Loop Icon Button (used in detail header) ──

export function LoopIconButton({
  agentName,
  agentState,
  onClick,
}: {
  agentName: string;
  agentState?: string;
  onClick: () => void;
}) {
  const [active, setActive] = useState(false);
  const [iterCount, setIterCount] = useState<number | null>(null);

  useEffect(() => {
    void getLoopConfig(agentName).then((cfg) => {
      setActive(cfg.enabled && cfg.prompt.trim().length > 0);
    });
  }, [agentName]);

  useEffect(() => {
    fetch(`/api/agents/${encodeURIComponent(agentName)}/activity?limit=500`)
      .then((r) => (r.ok ? (r.json() as Promise<Array<{ event: string }>>) : Promise.resolve([])))
      .then((items) => {
        const count = items.filter((i) => i.event === "ralph.loop").length;
        if (count > 0) setIterCount(count);
      })
      .catch(() => {/* ignore */});
  }, [agentName]);

  // Pulse when loop is active AND agent is actively running
  const isRunning = agentState === "working" || agentState === "starting";
  const shouldPulse = active && isRunning;

  return (
    <button
      type="button"
      onClick={onClick}
      title={
        active
          ? `Ralph Loop active${isRunning ? " — running" : " — waiting for agent to stop"} · click to edit`
          : "Ralph Loop — auto-reprompts agent when it stops"
      }
      className={`shrink-0 flex items-center gap-0.5 transition-colors ${
        active
          ? "text-green-400 hover:text-green-300"
          : "text-mycel-muted/30 hover:text-mycel-muted/60"
      }`}
    >
      <span className={`flex items-center gap-0.5 ${shouldPulse ? "animate-pulse" : ""}`}>
        <svg
          width="13"
          height="13"
          viewBox="0 0 14 14"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.8"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <path d="M11 3.5A5 5 0 1 0 12 7" />
          <path d="M8 3.5h3V.5" />
        </svg>
        {active && (
          <span
            className="text-[9px] font-bold leading-none"
            style={{ fontFamily: MONO }}
          >
            ↻
          </span>
        )}
        {active && iterCount != null && iterCount > 0 && (
          <span
            className="text-[8px] font-bold leading-none"
            style={{ fontFamily: MONO }}
          >
            {iterCount}
          </span>
        )}
      </span>
    </button>
  );
}

// ── Loop Modal ──

export function RalphLoopModal({
  open,
  agentName,
  onClose,
}: {
  open: boolean;
  agentName: string;
  onClose: () => void;
}) {
  const [enabled, setEnabled] = useState(false);
  const [prompt, setPrompt] = useState("");
  const [loading, setLoading] = useState(true);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    if (open) {
      setLoading(true);
      void getLoopConfig(agentName).then((cfg) => {
        setEnabled(cfg.enabled);
        setPrompt(cfg.prompt);
        setLoading(false);
        requestAnimationFrame(() => textareaRef.current?.focus());
      });
    }
  }, [open, agentName]);

  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [open, onClose]);

  const handleSave = useCallback(() => {
    // "Enable & Save" button: auto-enable if there's a prompt and enabled is currently false
    const saveEnabled = enabled || prompt.trim().length > 0;
    void setLoopConfig(agentName, { enabled: saveEnabled, prompt }).then(() => onClose());
  }, [agentName, enabled, prompt, onClose]);

  const handleDisable = useCallback(() => {
    void setLoopConfig(agentName, { enabled: false, prompt }).then(() => {
      setEnabled(false);
      onClose();
    });
  }, [agentName, prompt, onClose]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-[200] flex items-center justify-center p-4"
      onClick={onClose}
      role="presentation"
    >
      <div className="absolute inset-0 bg-black/60" />

      <div
        className="relative w-full max-w-md rounded-lg border border-mycel-border bg-mycel-surface shadow-2xl"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label="Ralph Loop"
      >
        <div className="flex items-center justify-between border-b border-mycel-border px-5 py-4">
          <div className="flex items-center gap-2">
            <svg
              width="16"
              height="16"
              viewBox="0 0 14 14"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.8"
              strokeLinecap="round"
              strokeLinejoin="round"
              className={enabled ? "text-green-400" : "text-mycel-muted"}
            >
              <path d="M11 3.5A5 5 0 1 0 12 7" />
              <path d="M8 3.5h3V.5" />
            </svg>
            <h2
              className="text-sm font-semibold text-mycel-text tracking-wide uppercase"
              style={{ fontFamily: MONO }}
            >
              Ralph Loop
            </h2>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="text-mycel-muted hover:text-mycel-text transition-colors rounded p-1 -mr-1"
            aria-label="Close"
          >
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
              <path d="M1 1l12 12M13 1L1 13" />
            </svg>
          </button>
        </div>

        <div className="px-5 py-4 flex flex-col gap-4">
          <p className="text-xs text-mycel-muted/70 leading-relaxed">
            When enabled, this prompt is automatically sent to{" "}
            <span className="text-mycel-text/80" style={{ fontFamily: MONO }}>
              {agentName}
            </span>{" "}
            every time the agent stops. Runs server-side — no browser needed.
          </p>

          <div className="flex items-center justify-between">
            <label
              className="text-xs font-medium text-mycel-muted uppercase tracking-wider"
              style={{ fontFamily: MONO }}
            >
              Enabled
            </label>
            <button
              type="button"
              role="switch"
              aria-checked={enabled}
              onClick={() => setEnabled((v) => !v)}
              className={`relative w-10 h-5 rounded-full transition-colors ${
                enabled ? "bg-green-500" : "bg-mycel-border"
              }`}
            >
              <span
                className={`absolute top-0.5 w-4 h-4 rounded-full bg-white shadow transition-transform ${
                  enabled ? "left-[22px]" : "left-0.5"
                }`}
              />
            </button>
          </div>

          <div className="flex flex-col gap-1.5">
            <label
              className="text-xs font-medium text-mycel-muted uppercase tracking-wider"
              style={{ fontFamily: MONO }}
            >
              Loop Prompt
            </label>
            <textarea
              ref={textareaRef}
              value={loading ? "Loading..." : prompt}
              onChange={(e) => setPrompt(e.target.value)}
              disabled={loading}
              rows={4}
              placeholder="continue"
              className={`${INPUT_CLS} resize-none`}
              style={{ fontFamily: MONO }}
            />
            <p className="text-[10px] text-mycel-muted">
              Sent to the agent each time it stops. Runs on the server — works
              even when you close the browser.
            </p>
          </div>
        </div>

        <div className="flex items-center justify-between border-t border-mycel-border px-5 py-4">
          <div>
            {enabled && (
              <button
                type="button"
                onClick={handleDisable}
                className="px-3 py-1.5 rounded text-xs text-mycel-error/80 border border-mycel-error/20 hover:bg-mycel-error/10 transition-colors"
                style={{ fontFamily: MONO }}
              >
                Disable loop
              </button>
            )}
          </div>
          <div className="flex gap-2">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 rounded text-sm text-mycel-muted hover:text-mycel-text border border-mycel-border hover:border-mycel-muted bg-mycel-bg transition-colors"
              style={{ fontFamily: MONO }}
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={handleSave}
              className="px-4 py-2 rounded text-sm font-medium bg-mycel-accent text-mycel-bg hover:opacity-90 transition-opacity"
              style={{ fontFamily: MONO }}
            >
              {enabled ? "Save" : "Enable & Save"}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

