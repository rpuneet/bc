/* Shared types for the first-run setup wizard. */

import type { SettingsConfig } from "../api/client";

export const WIZARD_STEPS = [
  { id: "welcome", eyebrow: "Welcome", title: "Welcome to mycel" },
  { id: "system", eyebrow: "System check", title: "Get your machine ready" },
  { id: "runtime", eyebrow: "Runtime", title: "Where your agents run" },
  { id: "providers", eyebrow: "Agent tools", title: "Pick an agent tool" },
  { id: "preferences", eyebrow: "Preferences", title: "A few defaults" },
  { id: "apps", eyebrow: "Connect", title: "Connect an app" },
  { id: "agent", eyebrow: "First agent", title: "Spawn your first agent" },
  { id: "done", eyebrow: "Ready", title: "You're all set" },
] as const;

export type StepId = (typeof WIZARD_STEPS)[number]["id"];

export function stepIndex(id: StepId): number {
  return WIZARD_STEPS.findIndex((s) => s.id === id);
}

/** Cross-step selections carried while the wizard is open. */
export interface WizardDraft {
  provider: string;
  model: string;
  runtime: "docker" | "tmux";
  name: string;
}

/** Navigation + lifecycle handed to every step. */
export interface WizardNav {
  next: () => void;
  back: () => void;
  /** Advance past this step without acting on it. */
  skip: () => void;
  goTo: (id: StepId) => void;
  /** Mark the wizard complete and return to the dashboard. */
  finish: () => void;
  /** Leave setup for the dashboard, keeping resume position. */
  exit: () => void;
  isFirst: boolean;
  isLast: boolean;
}

export interface StepProps {
  nav: WizardNav;
  draft: WizardDraft;
  setDraft: (patch: Partial<WizardDraft>) => void;
  /** Live prefs snapshot, loaded once by the shell. Null until first load. */
  settings: SettingsConfig | null;
  /** Re-fetch prefs after a step writes to them. */
  reloadSettings: () => Promise<void>;
}
