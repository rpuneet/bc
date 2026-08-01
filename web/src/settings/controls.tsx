/* Shared setup controls — the single source of truth for the option groups
 * that appear in BOTH the first-run wizard (web/src/wizard) and the Settings
 * page (web/src/views/Settings). Keeping them here means the two surfaces
 * can never drift: the wizard's Theme and Runtime pickers and the matching
 * Settings sections render the exact same field components.
 *
 * These are presentational + controlled: parents own the value and decide
 * how to persist it (the wizard writes prefs inline; Settings routes it
 * through the dirty-tracked save bar). */

export type ThemeChoice = "system" | "light" | "dark";

const THEME_CHOICES: { id: ThemeChoice; label: string; hint: string }[] = [
  { id: "system", label: "System", hint: "Match your OS" },
  { id: "light", label: "Light", hint: "Bright" },
  { id: "dark", label: "Dark", hint: "Dim" },
];

/** Resolve the "system" choice to a concrete theme using the OS preference. */
export function systemPrefersDark(): boolean {
  return typeof window !== "undefined" && !!window.matchMedia?.("(prefers-color-scheme: dark)").matches;
}

/** Segmented theme picker: System / Light / Dark. */
export function ThemePicker({
  value,
  onChange,
}: {
  value: ThemeChoice;
  onChange: (choice: ThemeChoice) => void;
}) {
  return (
    <div className="flex gap-2 flex-wrap">
      {THEME_CHOICES.map((c) => {
        const active = value === c.id;
        return (
          <button
            key={c.id}
            type="button"
            onClick={() => onChange(c.id)}
            aria-pressed={active}
            className={`relative flex flex-col items-start gap-0.5 px-3.5 py-2 rounded-lg border text-left cursor-pointer transition-all active:scale-[0.98] ${
              active
                ? "border-mycel-accent bg-mycel-accent-subtle shadow-mycel-sm"
                : "border-mycel-border bg-mycel-surface hover:border-mycel-accent hover:bg-mycel-surface-hover"
            }`}
          >
            {active && (
              <span className="absolute top-2 right-2 text-mycel-accent" aria-hidden>
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={3} strokeLinecap="round" strokeLinejoin="round"><path d="M20 6L9 17l-5-5" /></svg>
              </span>
            )}
            <span className="text-[13px] font-medium text-mycel-text pr-4">{c.label}</span>
            <span className="text-[11px] text-mycel-muted">{c.hint}</span>
          </button>
        );
      })}
    </div>
  );
}

export type RuntimeChoice = "docker" | "tmux";

/** Segmented runtime picker: Docker (recommended) or tmux. */
export function RuntimePicker({
  value,
  onChange,
}: {
  value: RuntimeChoice;
  onChange: (choice: RuntimeChoice) => void;
}) {
  const Option = ({
    id,
    title,
    body,
    recommended,
  }: {
    id: RuntimeChoice;
    title: string;
    body: string;
    recommended?: boolean;
  }) => {
    const active = value === id;
    return (
      <button
        type="button"
        onClick={() => onChange(id)}
        aria-pressed={active}
        className={`flex-1 text-left flex flex-col gap-1 rounded-lg border p-4 cursor-pointer transition-all active:scale-[0.99] ${
          active ? "border-mycel-accent bg-mycel-accent-subtle shadow-mycel-sm" : "border-mycel-border bg-mycel-surface hover:border-mycel-accent hover:bg-mycel-surface-hover"
        }`}
      >
        <div className="flex items-center gap-2">
          <span className={`grid place-items-center w-4 h-4 rounded-full border-2 shrink-0 transition-colors ${active ? "border-mycel-accent bg-mycel-accent" : "border-mycel-border-strong"}`} aria-hidden>
            {active && <svg width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="var(--mycel-accent-fg)" strokeWidth={4} strokeLinecap="round" strokeLinejoin="round"><path d="M20 6L9 17l-5-5" /></svg>}
          </span>
          <span className="text-[14px] font-semibold text-mycel-text">{title}</span>
          {recommended && (
            <span className="text-[10px] font-medium px-1.5 py-0.5 rounded-full bg-mycel-accent-subtle text-mycel-accent">
              Recommended
            </span>
          )}
        </div>
        <span className="text-[12px] text-mycel-text-2 leading-relaxed pl-6">{body}</span>
      </button>
    );
  };

  return (
    <div className="flex flex-col sm:flex-row gap-3">
      <Option id="docker" title="Docker" recommended body="Each agent gets a clean, isolated container. Safest default." />
      <Option id="tmux" title="tmux" body="Agents run in local shell sessions on this machine. Lightweight, no Docker needed." />
    </div>
  );
}

/** Progressive-disclosure expander — the wizard's "▸ Advanced settings"
 *  affordance, shared so Settings and the wizard reveal power-user options
 *  the same way. Controlled by the parent. */
export function AdvancedToggle({
  open,
  onToggle,
  label = "Advanced settings",
}: {
  open: boolean;
  onToggle: () => void;
  label?: string;
}) {
  return (
    <button
      type="button"
      onClick={onToggle}
      className="self-start text-[12px] text-mycel-muted hover:text-mycel-text transition-colors"
    >
      {open ? `▾ Hide ${label.toLowerCase()}` : `▸ ${label}`}
    </button>
  );
}
