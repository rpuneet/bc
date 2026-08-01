import { useEffect, useState } from "react";
import { api } from "../../api/client";
import { useTheme } from "../../context/ThemeContext";
import { ThemePicker, systemPrefersDark, type ThemeChoice } from "../../settings/controls";
import { WizardFooter } from "../WizardFooter";
import type { StepProps } from "../types";

/* Step 0 — Welcome & you. Name (prefilled) and theme. No avatar: the
 * mushroom characters belong to agents, not the operator. The theme picker
 * is shared with Settings → Profile (web/src/settings/controls). */

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
        <ThemePicker value={theme} onChange={pickTheme} />
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
