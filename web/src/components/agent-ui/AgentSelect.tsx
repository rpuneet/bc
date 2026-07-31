/* ── AgentSelect ─────────────────────────────────────────────────────
   The one dropdown for choosing an agent. A native <select> can only
   show plain text, so anywhere an agent is picked (clone-from, future
   pickers) this custom listbox is used instead — every option is a
   living AgentChip (character + name + status dot), matching how agents
   read everywhere else in the app.

   Small, self-contained: button + popover list, outside-click / Escape
   to close, keyboard-openable. */

import { useCallback, useEffect, useRef, useState } from "react";
import { AgentChip } from "./AgentChip";

export interface AgentOption {
  name: string;
  state?: string;
  tool?: string;
}

export interface AgentSelectProps {
  agents: AgentOption[];
  /** Selected agent name, or "" for none. */
  value: string;
  onChange: (name: string) => void;
  /** Show a "— none —" row that clears the selection. */
  allowNone?: boolean;
  noneLabel?: string;
  placeholder?: string;
  ariaLabel?: string;
  className?: string;
}

export function AgentSelect({
  agents,
  value,
  onChange,
  allowNone = false,
  noneLabel = "— none —",
  placeholder = "Select an agent",
  ariaLabel = "Select an agent",
  className = "",
}: AgentSelectProps) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);

  const close = useCallback(() => setOpen(false), []);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) close();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") close();
    };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open, close]);

  const selected = agents.find((a) => a.name === value);

  const pick = (name: string) => {
    onChange(name);
    close();
  };

  return (
    <div ref={rootRef} className={`relative ${className}`.trim()}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-label={ariaLabel}
        className="w-full flex items-center gap-2 h-9 px-2.5 rounded-md border border-mycel-border-strong bg-mycel-bg text-mycel-text text-[12px] outline-none focus:border-mycel-accent transition-colors"
      >
        <span className="flex-1 min-w-0 flex items-center">
          {selected ? (
            <AgentChip name={selected.name} state={selected.state} size={18} showDot={selected.state !== undefined} />
          ) : (
            <span className="truncate text-mycel-muted">{value || placeholder}</span>
          )}
        </span>
        <svg
          width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"
          strokeLinecap="round" strokeLinejoin="round" aria-hidden
          className="shrink-0 text-mycel-muted"
          style={{ transform: open ? "rotate(180deg)" : "none", transition: "transform 120ms" }}
        >
          <polyline points="6 9 12 15 18 9" />
        </svg>
      </button>

      {open && (
        <div
          role="listbox"
          aria-label={ariaLabel}
          className="absolute left-0 right-0 top-full mt-1 z-50 max-h-64 overflow-y-auto rounded-md border border-mycel-border bg-mycel-surface-2 shadow-mycel-lg py-1"
          style={{ scrollbarWidth: "thin" }}
        >
          {allowNone && (
            <button
              type="button"
              role="option"
              aria-selected={value === ""}
              onClick={() => pick("")}
              className={`w-full flex items-center px-2.5 py-1.5 text-[12px] text-left hover:bg-mycel-surface-hover transition-colors ${
                value === "" ? "text-mycel-accent" : "text-mycel-muted"
              }`}
            >
              {noneLabel}
            </button>
          )}
          {agents.map((a) => (
            <button
              key={a.name}
              type="button"
              role="option"
              aria-selected={a.name === value}
              onClick={() => pick(a.name)}
              className={`w-full flex items-center gap-2 px-2.5 py-1.5 text-left hover:bg-mycel-surface-hover transition-colors ${
                a.name === value ? "bg-mycel-surface-hover" : ""
              }`}
            >
              <AgentChip name={a.name} state={a.state} size={18} showDot={a.state !== undefined} className="min-w-0" />
            </button>
          ))}
          {agents.length === 0 && (
            <div className="px-2.5 py-2 text-[11px] text-mycel-muted italic">No agents</div>
          )}
        </div>
      )}
    </div>
  );
}
