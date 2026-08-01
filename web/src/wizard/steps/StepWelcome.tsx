import { useEffect, useState } from "react";
import { api } from "../../api/client";
import { useTheme } from "../../context/ThemeContext";
import { WizardFooter } from "../WizardFooter";
import type { StepProps } from "../types";

/* Step 0 — Welcome & you. Name (prefilled) and theme. No avatar: the
 * mushroom characters belong to agents, not the operator. */

type ThemeChoice = "system" | "light" | "dark";

const THEME_CHOICES: { id: ThemeChoice; label: string; hint: string }[] = [
  { id: "system", label: "System", hint: "Match your OS" },
  { id: "light", label: "Light", hint: "Bright" },
  { id: "dark", label: "Dark", hint: "Dim" },
];

function systemPrefersDark(): boolean {
  return typeof window !== "undefined" && !!window.matchMedia?.("(prefers-color-scheme: dark)").matches;
}

export function StepWelcome({ nav, setDraft, settings, reloadSettings }: StepProps) {
  const { mode, setTheme } = useTheme();
  const [name, setName] = useState("");
  const [theme, setThemeChoice] = useState<ThemeChoice>(mode);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (settings?.user?.name) setName(settings.user.name);
  }, [settings]);

  const pickTheme = (choice: ThemeChoice) => {
    setThemeChoice(choice);
    if (choice === "system") setTheme(systemPrefersDark() ? "dark" : "light");
    else setTheme(choice);
  };

  const persist = async () => {
    setSaving(true);
    setDraft({ name: name.trim() });
    try {
      const resolved = theme === "system" ? (systemPrefersDark() ? "dark" : "light") : theme;
      await api.updateSettings({
        user: { name: name.trim() },
        ui: {
          theme: resolved,
          mode: theme === "system" ? "auto" : resolved,
          default_view: settings?.ui?.default_view || "dashboard",
        },
      });
      await reloadSettings();
    } catch {
      /* non-fatal — the wizard is skippable and resumable */
    } finally {
      setSaving(false);
      nav.next();
    }
  };

  return (
    <div className="flex flex-col gap-6">
      <p className="text-[14px] leading-relaxed text-mycel-text-2 max-w-prose">
        mycel runs a network of AI coding agents — each in its own git worktree, working in
        parallel. This quick setup gets your machine ready and spawns your first one. You can skip
        any step and pick up later.
      </p>

      <div className="flex flex-col gap-2">
        <label htmlFor="wiz-name" className="text-[13px] font-medium text-mycel-text">
          What should agents call you?
        </label>
        <input
          id="wiz-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          maxLength={30}
          placeholder="Your name"
          className="w-full max-w-sm bg-mycel-bg border border-mycel-border rounded-md px-3 py-2 text-[14px] text-mycel-text placeholder:text-mycel-muted outline-none focus:border-mycel-accent transition-colors"
        />
        <p className="text-[11px] text-mycel-muted">Used to address you in the UI. Optional.</p>
      </div>

      <div className="flex flex-col gap-2">
        <span className="text-[13px] font-medium text-mycel-text">Theme</span>
        <div className="flex gap-2 flex-wrap">
          {THEME_CHOICES.map((c) => {
            const active = theme === c.id;
            return (
              <button
                key={c.id}
                type="button"
                onClick={() => pickTheme(c.id)}
                aria-pressed={active}
                className={`flex flex-col items-start gap-0.5 px-3.5 py-2 rounded-lg border text-left transition-colors ${
                  active
                    ? "border-mycel-accent bg-mycel-accent-subtle"
                    : "border-mycel-border bg-mycel-surface hover:border-mycel-accent"
                }`}
              >
                <span className="text-[13px] font-medium text-mycel-text">{c.label}</span>
                <span className="text-[11px] text-mycel-muted">{c.hint}</span>
              </button>
            );
          })}
        </div>
      </div>

      <WizardFooter
        nav={nav}
        primaryLabel={saving ? "Saving…" : "Get started"}
        onPrimary={persist}
        primaryDisabled={saving}
        hideSkip
      />
    </div>
  );
}
