import { useState, useCallback, useEffect, useRef } from "react";
import { AgentIcon, AgentAvatarPicker, colorFromName } from "./agent-ui";

// ── Name generation ───────────────────────────────────────────────────────────

const ADJECTIVES = [
  "amber", "azure", "bold", "brave", "bright", "calm", "cedar", "clean",
  "clever", "cool", "coral", "crisp", "dawn", "deep", "deft", "dense",
  "dusty", "eager", "early", "easy", "echo", "epic", "fair", "fast",
  "fierce", "fine", "firm", "fleet", "fresh", "frost", "gentle", "glad",
  "gold", "grand", "gray", "green", "grim", "hard", "hazy", "iron",
  "jade", "keen", "kind", "lean", "light", "lone", "lucid", "lunar",
  "misty", "noble",
];

const ANIMALS = [
  "albatross", "badger", "bear", "beaver", "bison", "boar", "buffalo",
  "capybara", "cheetah", "cobra", "condor", "cougar", "crane", "crow",
  "dingo", "dolphin", "eagle", "falcon", "ferret", "finch", "fox",
  "gecko", "gorilla", "hawk", "heron", "hippo", "hyena", "ibex",
  "iguana", "jaguar", "kestrel", "kiwi", "koala", "lemur", "leopard",
  "lion", "lizard", "lynx", "marmot", "marten", "meerkat", "moose",
  "narwhal", "orca", "osprey", "otter", "panda", "panther", "parrot",
];

function generateName(existingNames: string[]): string {
  const existing = new Set(existingNames);
  for (let i = 0; i < 20; i++) {
    const adj = ADJECTIVES[Math.floor(Math.random() * ADJECTIVES.length)];
    const animal = ANIMALS[Math.floor(Math.random() * ANIMALS.length)];
    const name = `${adj}-${animal}`;
    if (!existing.has(name)) return name;
  }
  // Fallback: append a short random suffix
  const adj = ADJECTIVES[Math.floor(Math.random() * ADJECTIVES.length)];
  const animal = ANIMALS[Math.floor(Math.random() * ANIMALS.length)];
  return `${adj}-${animal}-${Math.floor(Math.random() * 1000)}`;
}

// ── Types ─────────────────────────────────────────────────────────────────────

interface CreateAgentModalProps {
  open: boolean;
  onClose: () => void;
  existingNames: string[];
}

type Template = "feature-dev" | "reviewer" | "manager" | "blank";
type Provider = "claude" | "gemini" | "cursor" | "codex";
type Runtime = "docker" | "tmux";

// ── Shared input class ────────────────────────────────────────────────────────

const INPUT_CLS =
  "w-full bg-bc-bg border border-bc-border rounded px-3 py-2 text-sm text-bc-text " +
  "placeholder:text-bc-muted outline-none focus:border-bc-accent transition-colors";

const MONO =
  "'JetBrains Mono', 'Fira Code', 'Space Mono', ui-monospace, SFMono-Regular, Menlo, Consolas, monospace";

// ── Avatar variants ───────────────────────────────────────────────────────────

const AVATAR_VARIANTS = ["geometric", "organic", "monogram"] as const;
type AvatarVariant = (typeof AVATAR_VARIANTS)[number];

// ── Component ─────────────────────────────────────────────────────────────────

export function CreateAgentModal({
  open,
  onClose,
  existingNames,
}: CreateAgentModalProps) {
  const [name, setName] = useState(() => generateName(existingNames));
  const [template, setTemplate] = useState<Template>("feature-dev");
  const [provider, setProvider] = useState<Provider>("claude");
  const [runtime, setRuntime] = useState<Runtime>("docker");
  const [task, setTask] = useState("");
  const [variant, setVariant] = useState<AvatarVariant>(
    () => AVATAR_VARIANTS[Math.floor(Math.random() * AVATAR_VARIANTS.length)] ?? "geometric"
  );
  const [color, setColor] = useState(() => colorFromName(generateName(existingNames)));

  const firstInputRef = useRef<HTMLInputElement>(null);

  // Re-generate name when modal opens
  useEffect(() => {
    if (open) {
      const newName = generateName(existingNames);
      setName(newName);
      setTemplate("feature-dev");
      setProvider("claude");
      setRuntime("docker");
      setTask("");
      setVariant(AVATAR_VARIANTS[Math.floor(Math.random() * AVATAR_VARIANTS.length)] ?? "geometric");
      setColor(colorFromName(newName));
      requestAnimationFrame(() => firstInputRef.current?.focus());
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, existingNames]);

  // Auto-update color when name changes
  useEffect(() => {
    setColor(colorFromName(name));
  }, [name]);

  // Close on Escape
  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [open, onClose]);

  const handleRegenerate = useCallback(() => {
    setName(generateName(existingNames));
  }, [existingNames]);

  const handleCreate = useCallback(() => {
    const values = { name, template, provider, runtime, task, avatar: { variant, color } };
    console.log("[CreateAgentModal] create agent:", values);
    onClose();
  }, [name, template, provider, runtime, task, variant, color, onClose]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-[200] flex items-center justify-center p-4"
      onClick={onClose}
      role="presentation"
    >
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black/60" />

      {/* Card */}
      <div
        className="relative w-full max-w-md rounded-lg border border-bc-border bg-bc-surface shadow-2xl"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label="Create agent"
      >
        {/* Header */}
        <div className="flex items-center justify-between border-b border-bc-border px-5 py-4">
          <h2
            className="text-sm font-semibold text-bc-text tracking-wide uppercase"
            style={{ fontFamily: MONO }}
          >
            Create Agent
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="text-bc-muted hover:text-bc-text transition-colors rounded p-1 -mr-1"
            aria-label="Close"
          >
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
              <path d="M1 1l12 12M13 1L1 13" />
            </svg>
          </button>
        </div>

        {/* Body */}
        <div className="px-5 py-4 flex flex-col gap-4">

          {/* Avatar preview */}
          <div className="flex justify-center">
            <AgentIcon name={name} variant={variant} color={color} state="idle" size={48} />
          </div>

          {/* Name */}
          <div className="flex flex-col gap-1.5">
            <label className="text-xs font-medium text-bc-muted uppercase tracking-wider">
              Name
            </label>
            <div className="flex items-center gap-2">
              <input
                ref={firstInputRef}
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                className={INPUT_CLS}
                style={{ fontFamily: MONO }}
                placeholder="agent-name"
                spellCheck={false}
                autoComplete="off"
              />
              <button
                type="button"
                onClick={handleRegenerate}
                title="Regenerate name"
                className="shrink-0 flex items-center justify-center w-8 h-8 rounded border border-bc-border bg-bc-bg text-bc-muted hover:text-bc-accent hover:border-bc-accent transition-colors"
              >
                {/* Refresh icon */}
                <svg width="13" height="13" viewBox="0 0 13 13" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M11.5 2A6 6 0 1 0 12 6.5" />
                  <path d="M8 2h3.5V5.5" />
                </svg>
              </button>
            </div>
          </div>

          {/* Avatar picker */}
          <AgentAvatarPicker
            variant={variant}
            color={color}
            onVariantChange={(v) => setVariant(v as AvatarVariant)}
            onColorChange={setColor}
          />

          {/* Template */}
          <div className="flex flex-col gap-1.5">
            <label className="text-xs font-medium text-bc-muted uppercase tracking-wider">
              Template
            </label>
            <select
              value={template}
              onChange={(e) => setTemplate(e.target.value as Template)}
              className={INPUT_CLS}
              style={{ fontFamily: MONO }}
            >
              <option value="feature-dev">feature-dev</option>
              <option value="reviewer">reviewer</option>
              <option value="manager">manager</option>
              <option value="blank">blank</option>
            </select>
          </div>

          {/* Provider + Runtime side by side */}
          <div className="grid grid-cols-2 gap-3">
            <div className="flex flex-col gap-1.5">
              <label className="text-xs font-medium text-bc-muted uppercase tracking-wider">
                Provider
              </label>
              <select
                value={provider}
                onChange={(e) => setProvider(e.target.value as Provider)}
                className={INPUT_CLS}
                style={{ fontFamily: MONO }}
              >
                <option value="claude">claude</option>
                <option value="gemini">gemini</option>
                <option value="cursor">cursor</option>
                <option value="codex">codex</option>
              </select>
            </div>
            <div className="flex flex-col gap-1.5">
              <label className="text-xs font-medium text-bc-muted uppercase tracking-wider">
                Runtime
              </label>
              <select
                value={runtime}
                onChange={(e) => setRuntime(e.target.value as Runtime)}
                className={INPUT_CLS}
                style={{ fontFamily: MONO }}
              >
                <option value="docker">docker</option>
                <option value="tmux">tmux</option>
              </select>
            </div>
          </div>

          {/* Initial task */}
          <div className="flex flex-col gap-1.5">
            <label className="text-xs font-medium text-bc-muted uppercase tracking-wider">
              Initial Task{" "}
              <span className="normal-case font-normal text-bc-muted/70">(optional)</span>
            </label>
            <textarea
              value={task}
              onChange={(e) => setTask(e.target.value)}
              rows={3}
              placeholder="Describe the first task for this agent…"
              className={`${INPUT_CLS} resize-none`}
              style={{ fontFamily: MONO }}
            />
          </div>
        </div>

        {/* Footer */}
        <div className="flex items-center justify-end gap-3 border-t border-bc-border px-5 py-4">
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-2 rounded text-sm text-bc-muted hover:text-bc-text border border-bc-border hover:border-bc-muted bg-bc-bg transition-colors"
            style={{ fontFamily: MONO }}
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={handleCreate}
            className="px-4 py-2 rounded text-sm font-medium bg-bc-accent text-bc-bg hover:opacity-90 transition-opacity"
            style={{ fontFamily: MONO }}
          >
            Create agent →
          </button>
        </div>
      </div>
    </div>
  );
}
