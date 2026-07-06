import { useState, useCallback, useEffect, useRef, memo } from "react";
import { useNavigate } from "react-router-dom";
import { AgentIcon } from "./agent-ui";
import type { AgentShape } from "./agent-ui";
import { EnvVarsEditor, isValidEnvKey } from "./EnvVarsEditor";
import type { EnvRow } from "./EnvVarsEditor";
import { MONO } from "../utils/typography";

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

interface RepoOption {
  path: string;
  name: string;
  agent_count: number;
}

interface RepoCandidate {
  path: string;
  name: string;
}

type Provider = "claude" | "agy" | "cursor" | "codex" | "pi";
type Runtime = "docker" | "tmux";

const DEFAULT_TEMPLATES = ["feature-dev", "reviewer", "manager", "blank"];
const VALID_PROVIDERS = new Set<string>(["claude", "agy", "cursor", "codex", "pi"]);
const VALID_RUNTIMES = new Set<string>(["docker", "tmux"]);

const SHAPES: AgentShape[] = ["hexagon", "circle", "square"];

// Memoized avatar so CSS animation ticks don't cause parent re-renders
const MemoAgentIcon = memo(AgentIcon);

const INPUT_CLS =
  "w-full bg-mycel-bg border border-mycel-border rounded-md px-3 py-2 text-sm text-mycel-text " +
  "placeholder:text-mycel-muted outline-none focus:border-mycel-accent transition-colors";

// ── Component ─────────────────────────────────────────────────────────────────

export function CreateAgentModal({
  open,
  onClose,
  existingNames,
  existingAgents = [],
  defaultCloneFrom = "",
}: CreateAgentModalProps) {
  const [name, setName] = useState(() => generateName(existingNames));
  const [shape, setShape] = useState<AgentShape>(
    () => SHAPES[Math.floor(Math.random() * SHAPES.length)] ?? "hexagon",
  );
  const [template, setTemplate] = useState("feature-dev");
  const [templates, setTemplates] = useState<string[]>(DEFAULT_TEMPLATES);
  const [provider, setProvider] = useState<Provider>("claude");
  // Model for the selected provider. "" = provider default (no flag).
  const [model, setModel] = useState("");
  // Curated model lists per provider, from GET /api/providers.
  const [providerModels, setProviderModels] = useState<Record<string, string[]>>({});
  const [runtime, setRuntime] = useState<Runtime>("docker");
  const [task, setTask] = useState("");
  // Environment variables — collapsed row editor. Values may hold
  // ${secret:NAME} references resolved from the vault at spawn.
  const [envOpen, setEnvOpen] = useState(false);
  const [envRows, setEnvRows] = useState<EnvRow[]>([]);
  const [cloneFrom, setCloneFrom] = useState("");
  // Repo path the new agent binds to. Defaults to the daemon's default
  // repo (GET /api/repos) once loaded; known repos populate a dropdown.
  const [repo, setRepo] = useState("");
  const [knownRepos, setKnownRepos] = useState<RepoOption[]>([]);
  const [defaultRepo, setDefaultRepo] = useState("");
  // Browse: scan a base directory for git repos via
  // POST /api/repos/discover/local and present a simple picker list.
  const [browseOpen, setBrowseOpen] = useState(false);
  const [browseRoot, setBrowseRoot] = useState("");
  const [scanning, setScanning] = useState(false);
  const [scanError, setScanError] = useState<string | null>(null);
  const [candidates, setCandidates] = useState<RepoCandidate[] | null>(null);
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

  // Fetch per-provider model lists whenever the modal opens.
  useEffect(() => {
    if (!open) return;
    fetch("/api/providers")
      .then((r) => r.json())
      .then((list: unknown) => {
        if (!Array.isArray(list)) return;
        const map: Record<string, string[]> = {};
        for (const p of list as Array<{ name?: unknown; models?: unknown }>) {
          if (typeof p.name === "string" && Array.isArray(p.models)) {
            map[p.name] = p.models.filter((m): m is string => typeof m === "string");
          }
        }
        setProviderModels(map);
      })
      .catch(() => {
        /* model list is a convenience — "default" always works */
      });
  }, [open]);

  // Fetch known repos + the default repo whenever the modal opens.
  useEffect(() => {
    if (!open) return;
    fetch("/api/repos")
      .then((r) => r.json())
      .then((data: { repos?: RepoOption[]; default?: string }) => {
        const list = Array.isArray(data.repos) ? data.repos : [];
        setKnownRepos(list);
        const def = typeof data.default === "string" ? data.default : "";
        setDefaultRepo(def);
        // Only fill the field if the user hasn't typed a path yet.
        setRepo((prev) => (prev === "" ? def : prev));
      })
      .catch(() => {
        /* repos list is a convenience — the text input still works */
      });
  }, [open]);

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
      setModel("");
      setRuntime("docker");
      setTask("");
      setEnvOpen(false);
      setEnvRows([]);
      setSubmitError(null);
      setSubmitting(false);
      // When opened from the Clone action, pre-select the source agent
      // so the clone-from effect populates provider/runtime automatically.
      setCloneFrom(defaultCloneFrom);
      setRepo(defaultRepo);
      setBrowseOpen(false);
      setCandidates(null);
      setScanError(null);
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

  // Scan browseRoot for git repos (POST /api/repos/discover/local).
  const handleScan = useCallback(async () => {
    const root = browseRoot.trim();
    if (!root) {
      setScanError("Enter a directory to scan.");
      return;
    }
    setScanning(true);
    setScanError(null);
    setCandidates(null);
    try {
      const res = await fetch("/api/repos/discover/local", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ root }),
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({})) as { error?: string };
        setScanError(err.error ?? "Scan failed");
        return;
      }
      const data = await res.json() as { candidates?: RepoCandidate[] };
      setCandidates(Array.isArray(data.candidates) ? data.candidates : []);
    } catch (e) {
      setScanError(e instanceof Error ? e.message : "Scan failed");
    } finally {
      setScanning(false);
    }
  }, [browseRoot]);

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
    const repoPath = repo.trim();
    if (!repoPath) {
      setSubmitError("Repo path is required.");
      return;
    }
    // Build the env map from filled rows; reject malformed names early
    // (mirrors the API rule) instead of round-tripping for a 400.
    const env: Record<string, string> = {};
    for (const row of envRows) {
      const key = row.key.trim();
      if (key === "" && row.value === "") continue; // ignore blank rows
      if (!isValidEnvKey(key)) {
        setSubmitError(`Invalid env var name "${key}": use letters, digits, underscore; must not start with a digit.`);
        return;
      }
      env[key] = row.value;
    }
    setSubmitError(null);
    setSubmitting(true);
    try {
      const res = await fetch("/api/agents", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: trimmed,
          template,
          tool: provider,
          model: model || undefined,
          runtime_backend: runtime,
          repo: repoPath,
          task: task || undefined,
          env: Object.keys(env).length > 0 ? env : undefined,
        }),
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
  }, [name, template, provider, model, runtime, task, repo, envRows, existingNames, onClose, navigate]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-[200] flex items-center justify-center p-4"
      onClick={onClose}
      role="presentation"
    >
      <div className="absolute inset-0 bg-mycel-overlay" />

      <div
        className="relative w-full max-w-md rounded-lg border border-mycel-border bg-mycel-surface-2 shadow-mycel-lg max-h-[90vh] overflow-y-auto"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label="Create agent"
      >
        {/* Header */}
        <div className="flex items-center justify-between border-b border-mycel-border px-5 py-4">
          <h2 className="text-base font-semibold text-mycel-text">
            Create Agent
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="text-mycel-muted hover:text-mycel-text transition-colors rounded-md p-1 -mr-1"
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
            <label className="text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">
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
                className="shrink-0 flex items-center justify-center w-8 h-8 rounded-md border border-mycel-border bg-mycel-bg text-mycel-muted hover:text-mycel-accent hover:border-mycel-accent transition-colors"
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
            <label className="text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">
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

          {/* Repo — required. The repo is a property on the agent:
              every new agent binds to a git repo path. Defaults to the
              repo bcd was booted against. */}
          <div className="flex flex-col gap-1.5">
            <label className="text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">
              Repo <span className="text-mycel-error">*</span>
            </label>
            <div className="flex items-center gap-2">
              <input
                type="text"
                value={repo}
                onChange={(e) => setRepo(e.target.value)}
                className={INPUT_CLS}
                style={{ fontFamily: MONO }}
                placeholder="/absolute/path/to/repo"
                spellCheck={false}
                autoComplete="off"
                required
              />
              <button
                type="button"
                onClick={() => {
                  setBrowseOpen((prev) => !prev);
                  // Seed the scan root with the parent of the current
                  // path so one Scan click usually finds siblings.
                  if (!browseOpen && browseRoot === "" && repo) {
                    setBrowseRoot(repo.replace(/\/[^/]*$/, "") || "/");
                  }
                }}
                aria-pressed={browseOpen}
                className={`shrink-0 inline-flex items-center px-3 h-8 rounded-md border text-xs font-medium transition-colors ${
                  browseOpen
                    ? "border-mycel-accent text-mycel-accent bg-mycel-bg"
                    : "border-mycel-border bg-mycel-bg text-mycel-muted hover:text-mycel-accent hover:border-mycel-accent"
                }`}
              >
                Browse
              </button>
            </div>
            {knownRepos.length > 0 && (
              <select
                value={knownRepos.some((r) => r.path === repo) ? repo : ""}
                onChange={(e) => {
                  if (e.target.value) setRepo(e.target.value);
                }}
                className={INPUT_CLS}
                style={{ fontFamily: MONO }}
                aria-label="Known repos"
              >
                <option value="">— known repos —</option>
                {knownRepos.map((r) => (
                  <option key={r.path} value={r.path}>
                    {r.name} · {r.path}
                    {r.path === defaultRepo ? " (default)" : ""}
                  </option>
                ))}
              </select>
            )}
            {browseOpen && (
              <div className="flex flex-col gap-2 rounded-md border border-mycel-border bg-mycel-bg p-2">
                <div className="flex items-center gap-2">
                  <input
                    type="text"
                    value={browseRoot}
                    onChange={(e) => setBrowseRoot(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") {
                        e.preventDefault();
                        void handleScan();
                      }
                    }}
                    className={INPUT_CLS}
                    style={{ fontFamily: MONO }}
                    placeholder="/directory/to/scan"
                    spellCheck={false}
                    autoComplete="off"
                  />
                  <button
                    type="button"
                    onClick={() => { void handleScan(); }}
                    disabled={scanning}
                    className="shrink-0 inline-flex items-center px-3 h-8 rounded-md border border-mycel-border bg-mycel-surface text-xs font-medium text-mycel-muted hover:text-mycel-accent hover:border-mycel-accent transition-colors disabled:opacity-50"
                  >
                    {scanning ? "Scanning..." : "Scan"}
                  </button>
                </div>
                {scanError && (
                  <div className="text-xs text-mycel-error">
                    {scanError}
                  </div>
                )}
                {candidates && candidates.length === 0 && (
                  <div className="text-xs text-mycel-muted">
                    No git repos found under that directory.
                  </div>
                )}
                {candidates && candidates.length > 0 && (
                  <ul className="max-h-36 overflow-y-auto flex flex-col">
                    {candidates.map((c) => (
                      <li key={c.path}>
                        <button
                          type="button"
                          onClick={() => {
                            setRepo(c.path);
                            setBrowseOpen(false);
                          }}
                          className="w-full text-left px-2 py-1 rounded-md text-xs text-mycel-text hover:bg-mycel-surface hover:text-mycel-accent transition-colors truncate"
                          style={{ fontFamily: MONO }}
                          title={c.path}
                        >
                          {c.name} <span className="text-mycel-muted">{c.path}</span>
                        </button>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            )}
          </div>

          {/* Template */}
          <div className="flex flex-col gap-1.5">
            <label className="text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">
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
              <label className="text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">
                Clone config from{" "}
                <span className="normal-case font-normal text-mycel-muted">(optional)</span>
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
              <label className="text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">
                Provider
              </label>
              <select
                value={provider}
                onChange={(e) => {
                  setProvider(e.target.value as Provider);
                  // Model lists are per-provider — reset to default.
                  setModel("");
                }}
                className={INPUT_CLS}
                style={{ fontFamily: MONO }}
              >
                <option value="claude">claude</option>
                <option value="agy">agy</option>
                <option value="cursor">cursor</option>
                <option value="codex">codex</option>
                <option value="pi">pi</option>
              </select>
            </div>
            <div className="flex flex-col gap-1.5">
              <label className="text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">
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
            {/* Model — third grid cell lands directly under Provider.
                "default" (empty) means the provider default: no model
                flag is injected. Options repopulate per provider. */}
            <div className="flex flex-col gap-1.5">
              <label className="text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">
                Model
              </label>
              <select
                value={model}
                onChange={(e) => setModel(e.target.value)}
                className={INPUT_CLS}
                style={{ fontFamily: MONO }}
                aria-label="Model"
              >
                <option value="">default</option>
                {(providerModels[provider] ?? []).map((m) => (
                  <option key={m} value={m}>{m}</option>
                ))}
              </select>
            </div>
          </div>

          {/* Environment — collapsible key/value editor with secret
              reference autocomplete. Collapsed by default. */}
          <div className="flex flex-col gap-1.5">
            <button
              type="button"
              onClick={() => setEnvOpen((prev) => !prev)}
              aria-expanded={envOpen}
              className="flex items-center gap-1.5 text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted hover:text-mycel-text transition-colors w-fit"
            >
              <svg
                width="8"
                height="8"
                viewBox="0 0 8 8"
                fill="currentColor"
                className={`transition-transform ${envOpen ? "rotate-90" : ""}`}
                aria-hidden="true"
              >
                <path d="M2 0l4 4-4 4z" />
              </svg>
              Environment{" "}
              <span className="normal-case font-normal">
                (optional{envRows.some((r) => r.key.trim() !== "") ? ` · ${envRows.filter((r) => r.key.trim() !== "").length}` : ""})
              </span>
            </button>
            {envOpen && <EnvVarsEditor rows={envRows} onChange={setEnvRows} />}
          </div>

          {/* Initial task */}
          <div className="flex flex-col gap-1.5">
            <label className="text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">
              Initial Task{" "}
              <span className="normal-case font-normal text-mycel-muted">(optional)</span>
            </label>
            <textarea
              value={task}
              onChange={(e) => setTask(e.target.value)}
              rows={3}
              placeholder="Describe the first task for this agent..."
              className={`${INPUT_CLS} resize-none`}
            />
          </div>
        </div>

        {/* Footer */}
        <div className="border-t border-mycel-border px-5 py-4 flex flex-col gap-2">
          {submitError && (
            <div
              role="alert"
              className="rounded-md border border-mycel-border bg-mycel-error-subtle px-3 py-2 text-xs text-mycel-error"
            >
              {submitError}
            </div>
          )}
          <div className="flex items-center justify-end gap-3">
            <button
              type="button"
              onClick={onClose}
              disabled={submitting}
              className="inline-flex items-center h-9 px-3 rounded-md text-sm bg-mycel-surface border border-mycel-border text-mycel-text-2 hover:text-mycel-text hover:bg-mycel-surface-hover transition-colors disabled:opacity-50"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={() => { void handleCreate(); }}
              disabled={submitting || !name.trim()}
              className="inline-flex items-center h-9 px-3 rounded-md text-sm font-medium bg-mycel-accent text-mycel-accent-fg hover:bg-mycel-accent-hover shadow-mycel-sm transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {submitting ? "Creating..." : "Create agent"}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
