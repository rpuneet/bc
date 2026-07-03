import { useState, useCallback, useEffect, useRef, memo } from "react";
import { useNavigate } from "react-router-dom";
import { AgentIcon } from "./agent-ui";
import type { AgentShape } from "./agent-ui";
import { MONO } from "../utils/typography";
import { useWorkspace } from "../context/WorkspaceContext";

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
  state?: string;
}

interface CreateAgentModalProps {
  open: boolean;
  onClose: () => void;
  existingNames: string[];
  existingAgents?: ExistingAgent[];
  /** Pre-select a source agent in the "clone from" dropdown when the
   *  modal opens. Used by the Clone button on AgentDetail. */
  defaultCloneFrom?: string;
}

type Provider = "claude" | "gemini" | "cursor" | "codex";
type Runtime = "docker" | "tmux";

const DEFAULT_TEMPLATES = ["feature-dev", "reviewer", "manager", "blank"];
const VALID_PROVIDERS = new Set<string>(["claude", "gemini", "cursor", "codex"]);
const VALID_RUNTIMES = new Set<string>(["docker", "tmux"]);

const SHAPES: AgentShape[] = ["hexagon", "circle", "square"];

// Memoized avatar so CSS animation ticks don't cause parent re-renders
const MemoAgentIcon = memo(AgentIcon);

const INPUT_CLS =
  "w-full bg-mycel-bg border border-mycel-border rounded px-3 py-2 text-sm text-mycel-text " +
  "placeholder:text-mycel-muted outline-none focus:border-mycel-accent transition-colors";

// ── Component ─────────────────────────────────────────────────────────────────

export function CreateAgentModal({
  open,
  onClose,
  existingNames,
  existingAgents = [],
  defaultCloneFrom = "",
}: CreateAgentModalProps) {
  const { workspaces, workspace: activeWorkspace } = useWorkspace();
  const [name, setName] = useState(() => generateName(existingNames));
  const [shape, setShape] = useState<AgentShape>(
    () => SHAPES[Math.floor(Math.random() * SHAPES.length)] ?? "hexagon",
  );
  const [template, setTemplate] = useState("feature-dev");
  const [templates, setTemplates] = useState<string[]>(DEFAULT_TEMPLATES);
  const [provider, setProvider] = useState<Provider>("claude");
  const [runtime, setRuntime] = useState<Runtime>("docker");
  const [task, setTask] = useState("");
  const [cloneFrom, setCloneFrom] = useState("");
  const [workspaceId, setWorkspaceId] = useState<string>(activeWorkspace?.id ?? "");
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const firstInputRef = useRef<HTMLInputElement>(null);
  const navigate = useNavigate();

  // Fetch templates from API on mount
  useEffect(() => {
    fetch("/api/templates")
      .then((r) => r.json())
      .then((list: unknown) => {
        if (Array.isArray(list) && list.length > 0) {
          const names = (list as Array<{ name?: unknown }>)
            .map((t) => (typeof t.name === "string" ? t.name : null))
            .filter((n): n is string => n !== null);
          if (names.length > 0) setTemplates(names);
        }
      })
      .catch(() => {
        /* fall back to defaults */
      });
  }, []);

  // Reset form only when the modal is first opened (open transitions to true).
  // We use a ref to track the previous open state so that changes to
  // existingNames or defaultCloneFrom while the modal is already open
  // do not wipe form fields the user is actively typing in.
  const prevOpenRef = useRef(false);
  useEffect(() => {
    if (open && !prevOpenRef.current) {
      const newName = generateName(existingNames);
      setName(newName);
      setShape(SHAPES[Math.floor(Math.random() * SHAPES.length)] ?? "hexagon");
      setTemplate("feature-dev");
      setProvider("claude");
      setRuntime("docker");
      setTask("");
      setSubmitError(null);
      setSubmitting(false);
      // When opened from the Clone action, pre-select the source agent
      // so the clone-from effect populates provider/runtime automatically.
      setCloneFrom(defaultCloneFrom);
      setWorkspaceId(activeWorkspace?.id ?? "");
      requestAnimationFrame(() => firstInputRef.current?.focus());
    }
    prevOpenRef.current = open;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  // Close on Escape
  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [open, onClose]);

  // When clone-from selected, populate provider/runtime (with validation)
  useEffect(() => {
    if (!cloneFrom) return;
    const source = existingAgents.find((a) => a.name === cloneFrom);
    if (source) {
      if (source.tool && VALID_PROVIDERS.has(source.tool)) {
        setProvider(source.tool as Provider);
      }
      if (source.runtime_backend && VALID_RUNTIMES.has(source.runtime_backend)) {
        setRuntime(source.runtime_backend as Runtime);
      }
    }
  }, [cloneFrom, existingAgents]);

  const handleRegenerate = useCallback(() => {
    setName(generateName(existingNames));
  }, [existingNames]);

  const handleCreate = useCallback(async () => {
    const trimmed = name.trim();
    if (!trimmed) {
      setSubmitError("Agent name is required.");
      return;
    }
    if (existingNames.includes(trimmed)) {
      setSubmitError(`Agent "${trimmed}" already exists. Pick a different name.`);
      return;
    }
    if (!workspaceId) {
      setSubmitError("Workspace is required.");
      return;
    }
    setSubmitError(null);
    setSubmitting(true);
    try {
      const url = `/api/agents?workspace=${encodeURIComponent(workspaceId)}`;
      const res = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: trimmed, template, tool: provider, runtime_backend: runtime, task: task || undefined }),
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({})) as { error?: string };
        setSubmitError(err.error ?? "Failed to create agent");
        setSubmitting(false);
        return;
      }
      onClose();
      navigate(`/agents/${encodeURIComponent(trimmed)}`);
    } catch (e) {
      setSubmitError(e instanceof Error ? e.message : "Failed to create agent");
      setSubmitting(false);
    }
  }, [name, template, provider, runtime, task, workspaceId, existingNames, onClose, navigate]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-[200] flex items-center justify-center p-4"
      onClick={onClose}
      role="presentation"
    >
      <div className="absolute inset-0 bg-mycel-overlay" />

      <div
        className="relative w-full max-w-md rounded-lg border border-mycel-border bg-mycel-surface shadow-2xl max-h-[90vh] overflow-y-auto"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label="Create agent"
      >
        {/* Header */}
        <div className="flex items-center justify-between border-b border-mycel-border px-5 py-4">
          <h2
            className="text-sm font-semibold text-mycel-text tracking-wide uppercase"
            style={{ fontFamily: MONO }}
          >
            Create Agent
          </h2>
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

        {/* Body */}
        <div className="px-5 py-4 flex flex-col gap-4">
          {/* Shape preview */}
          <div className="flex justify-center">
            <MemoAgentIcon shape={shape} state="idle" size={64} tool={provider} />
          </div>

          {/* Name + regen */}
          <div className="flex flex-col gap-1.5">
            <label className="text-xs font-medium text-mycel-muted uppercase tracking-wider" style={{ fontFamily: MONO }}>
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
                className="shrink-0 flex items-center justify-center w-8 h-8 rounded border border-mycel-border bg-mycel-bg text-mycel-muted hover:text-mycel-accent hover:border-mycel-accent transition-colors"
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
            <label className="text-xs font-medium text-mycel-muted uppercase tracking-wider" style={{ fontFamily: MONO }}>
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

          {/* Workspace — required. Workspace is a property on the
              agent (workspace-as-property model), so every new agent
              must be bound to one. */}
          <div className="flex flex-col gap-1.5">
            <label className="text-xs font-medium text-mycel-muted uppercase tracking-wider" style={{ fontFamily: MONO }}>
              Workspace <span className="text-mycel-error">*</span>
            </label>
            <select
              value={workspaceId}
              onChange={(e) => setWorkspaceId(e.target.value)}
              className={INPUT_CLS}
              style={{ fontFamily: MONO }}
              required
            >
              <option value="">— select workspace —</option>
              {workspaces.map((ws) => (
                <option key={ws.id} value={ws.id}>
                  {ws.name || ws.path.split("/").pop() || ws.id}
                </option>
              ))}
            </select>
          </div>

          {/* Template */}
          <div className="flex flex-col gap-1.5">
            <label className="text-xs font-medium text-mycel-muted uppercase tracking-wider" style={{ fontFamily: MONO }}>
              Template
            </label>
            <select
              value={template}
              onChange={(e) => setTemplate(e.target.value)}
              className={INPUT_CLS}
              style={{ fontFamily: MONO }}
            >
              {templates.map((t) => (
                <option key={t} value={t}>{t}</option>
              ))}
            </select>
          </div>

          {/* Clone from existing agent */}
          {existingAgents.length > 0 && (
            <div className="flex flex-col gap-1.5">
              <label className="text-xs font-medium text-mycel-muted uppercase tracking-wider" style={{ fontFamily: MONO }}>
                Clone config from{" "}
                <span className="normal-case font-normal text-mycel-muted/70">(optional)</span>
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
                    {a.name}{a.state ? ` · ${a.state}` : ""}
                  </option>
                ))}
              </select>
            </div>
          )}

          {/* Provider + Runtime */}
          <div className="grid grid-cols-2 gap-3">
            <div className="flex flex-col gap-1.5">
              <label className="text-xs font-medium text-mycel-muted uppercase tracking-wider" style={{ fontFamily: MONO }}>
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
                <option value="pi">pi</option>
                <option value="codex">codex</option>
                <option value="pi">pi</option>
              </select>
            </div>
            <div className="flex flex-col gap-1.5">
              <label className="text-xs font-medium text-mycel-muted uppercase tracking-wider" style={{ fontFamily: MONO }}>
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
            <label className="text-xs font-medium text-mycel-muted uppercase tracking-wider" style={{ fontFamily: MONO }}>
              Initial Task{" "}
              <span className="normal-case font-normal text-mycel-muted/70">(optional)</span>
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
        <div className="border-t border-mycel-border px-5 py-4 flex flex-col gap-2">
          {submitError && (
            <div
              role="alert"
              className="rounded border border-mycel-error/40 bg-mycel-error/10 px-3 py-2 text-xs text-mycel-error"
              style={{ fontFamily: MONO }}
            >
              {submitError}
            </div>
          )}
          <div className="flex items-center justify-end gap-3">
            <button
              type="button"
              onClick={onClose}
              disabled={submitting}
              className="px-4 py-2 rounded text-sm text-mycel-muted hover:text-mycel-text border border-mycel-border hover:border-mycel-muted bg-mycel-bg transition-colors disabled:opacity-50"
              style={{ fontFamily: MONO }}
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={() => { void handleCreate(); }}
              disabled={submitting || !name.trim()}
              className="px-4 py-2 rounded text-sm font-medium bg-mycel-accent text-mycel-bg hover:opacity-90 transition-opacity disabled:opacity-50 disabled:cursor-not-allowed"
              style={{ fontFamily: MONO }}
            >
              {submitting ? "Creating..." : "Create agent"}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
