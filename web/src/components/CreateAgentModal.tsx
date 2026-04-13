import { useState, useCallback, useEffect, useRef } from "react";
import { AgentIcon } from "./agent-ui";
import type { AgentShape } from "./agent-ui";

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
  const adj = ADJECTIVES[Math.floor(Math.random() * ADJECTIVES.length)];
  const animal = ANIMALS[Math.floor(Math.random() * ANIMALS.length)];
  return `${adj}-${animal}-${String(Math.floor(Math.random() * 1000))}`;
}

// ── Types ─────────────────────────────────────────────────────────────────────

interface ExistingAgent {
  name: string;
  tool?: string;
  runtime_backend?: string;
}

interface CreateAgentModalProps {
  open: boolean;
  onClose: () => void;
  existingNames: string[];
  existingAgents?: ExistingAgent[];
}

type Template = "feature-dev" | "reviewer" | "manager" | "blank";
type Provider = "claude" | "gemini" | "cursor" | "codex";
type Runtime = "docker" | "tmux";

const SHAPES: AgentShape[] = ["hexagon", "circle", "square"];

const INPUT_CLS =
  "w-full bg-bc-bg border border-bc-border rounded px-3 py-2 text-sm text-bc-text " +
  "placeholder:text-bc-muted outline-none focus:border-bc-accent transition-colors";

const MONO =
  "'JetBrains Mono', 'Fira Code', 'Space Mono', ui-monospace, SFMono-Regular, Menlo, Consolas, monospace";

// ── Component ─────────────────────────────────────────────────────────────────

export function CreateAgentModal({
  open,
  onClose,
  existingNames,
  existingAgents = [],
}: CreateAgentModalProps) {
  const [name, setName] = useState(() => generateName(existingNames));
  const [shape, setShape] = useState<AgentShape>(
    () => SHAPES[Math.floor(Math.random() * SHAPES.length)] ?? "hexagon",
  );
  const [template, setTemplate] = useState<Template>("feature-dev");
  const [provider, setProvider] = useState<Provider>("claude");
  const [runtime, setRuntime] = useState<Runtime>("docker");
  const [task, setTask] = useState("");
  const [cloneFrom, setCloneFrom] = useState("");

  const firstInputRef = useRef<HTMLInputElement>(null);

  // Reset on open
  useEffect(() => {
    if (open) {
      const newName = generateName(existingNames);
      setName(newName);
      setShape(SHAPES[Math.floor(Math.random() * SHAPES.length)] ?? "hexagon");
      setTemplate("feature-dev");
      setProvider("claude");
      setRuntime("docker");
      setTask("");
      setCloneFrom("");
      requestAnimationFrame(() => firstInputRef.current?.focus());
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, existingNames]);

  // Close on Escape
  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [open, onClose]);

  // When clone-from selected, populate provider/runtime
  useEffect(() => {
    if (!cloneFrom) return;
    const source = existingAgents.find((a) => a.name === cloneFrom);
    if (source) {
      if (source.tool) setProvider(source.tool as Provider);
      if (source.runtime_backend) setRuntime(source.runtime_backend as Runtime);
    }
  }, [cloneFrom, existingAgents]);

  const handleRegenerate = useCallback(() => {
    setName(generateName(existingNames));
  }, [existingNames]);

  const handleCreate = useCallback(() => {
    const values = { name, shape, template, provider, runtime, task, cloneFrom: cloneFrom || undefined };
    console.log("[CreateAgentModal] create agent:", values);
    onClose();
  }, [name, shape, template, provider, runtime, task, cloneFrom, onClose]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-[200] flex items-center justify-center p-4"
      onClick={onClose}
      role="presentation"
    >
      <div className="absolute inset-0 bg-black/60" />

      <div
        className="relative w-full max-w-md rounded-lg border border-bc-border bg-bc-surface shadow-2xl max-h-[90vh] overflow-y-auto"
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
          {/* Shape preview */}
          <div className="flex justify-center">
            <AgentIcon shape={shape} state="working" size={64} />
          </div>

          {/* Name + regen */}
          <div className="flex flex-col gap-1.5">
            <label className="text-xs font-medium text-bc-muted uppercase tracking-wider" style={{ fontFamily: MONO }}>
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
                <svg width="13" height="13" viewBox="0 0 13 13" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M11.5 2A6 6 0 1 0 12 6.5" />
                  <path d="M8 2h3.5V5.5" />
                </svg>
              </button>
            </div>
          </div>

          {/* Shape dropdown */}
          <div className="flex flex-col gap-1.5">
            <label className="text-xs font-medium text-bc-muted uppercase tracking-wider" style={{ fontFamily: MONO }}>
              Shape
            </label>
            <select
              value={shape}
              onChange={(e) => setShape(e.target.value as AgentShape)}
              className={INPUT_CLS}
              style={{ fontFamily: MONO }}
            >
              <option value="hexagon">hexagon</option>
              <option value="circle">circle</option>
              <option value="square">square</option>
            </select>
          </div>

          {/* Template */}
          <div className="flex flex-col gap-1.5">
            <label className="text-xs font-medium text-bc-muted uppercase tracking-wider" style={{ fontFamily: MONO }}>
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

          {/* Clone from existing agent */}
          {existingAgents.length > 0 && (
            <div className="flex flex-col gap-1.5">
              <label className="text-xs font-medium text-bc-muted uppercase tracking-wider" style={{ fontFamily: MONO }}>
                Clone config from{" "}
                <span className="normal-case font-normal text-bc-muted/70">(optional)</span>
              </label>
              <select
                value={cloneFrom}
                onChange={(e) => setCloneFrom(e.target.value)}
                className={INPUT_CLS}
                style={{ fontFamily: MONO }}
              >
                <option value="">— none —</option>
                {existingAgents.map((a) => (
                  <option key={a.name} value={a.name}>
                    {a.name}
                  </option>
                ))}
              </select>
            </div>
          )}

          {/* Provider + Runtime */}
          <div className="grid grid-cols-2 gap-3">
            <div className="flex flex-col gap-1.5">
              <label className="text-xs font-medium text-bc-muted uppercase tracking-wider" style={{ fontFamily: MONO }}>
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
              <label className="text-xs font-medium text-bc-muted uppercase tracking-wider" style={{ fontFamily: MONO }}>
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
            <label className="text-xs font-medium text-bc-muted uppercase tracking-wider" style={{ fontFamily: MONO }}>
              Initial Task{" "}
              <span className="normal-case font-normal text-bc-muted/70">(optional)</span>
            </label>
            <textarea
              value={task}
              onChange={(e) => setTask(e.target.value)}
              rows={3}
              placeholder="Describe the first task for this agent..."
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
            Create agent
          </button>
        </div>
      </div>
    </div>
  );
}
