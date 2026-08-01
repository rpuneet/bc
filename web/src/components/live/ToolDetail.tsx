import { useCallback, useState } from "react";
import { compactPath, redactSecrets, redactValue } from "./liveHelpers";

/* ── Copy button ───────────────────────────────────────────────────── */

/** Compact icon-only copy control: a clipboard glyph that swaps to a
 *  check on success, then settles back. Shares the row-control sizing so
 *  it reads as part of the expanded row, not a bolted-on button. */
export function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    }).catch(() => {});
  }, [text]);

  return (
    <button
      type="button"
      onClick={(e) => { e.stopPropagation(); handleCopy(); }}
      className={`inline-flex items-center justify-center h-[22px] w-[22px] rounded-md transition-colors shrink-0 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-mycel-accent ${
        copied
          ? "text-mycel-success"
          : "text-mycel-muted hover:text-mycel-text hover:bg-mycel-surface-hover"
      }`}
      aria-label="Copy to clipboard"
      title={copied ? "Copied" : "Copy JSON"}
    >
      {copied ? (
        <svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
          <path d="M3.5 8.5l3 3 6-7" />
        </svg>
      ) : (
        <svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
          <rect x="5.5" y="5.5" width="8" height="8" rx="1.5" />
          <path d="M10.5 5.5V4a1.5 1.5 0 00-1.5-1.5H4A1.5 1.5 0 002.5 4v5A1.5 1.5 0 004 10.5h1.5" />
        </svg>
      )}
    </button>
  );
}

/* ── ToolDetail ──────────────────────────────────────────────────────
   Rich renderers for expanded tool-call rows. Raw JSON blobs read
   terribly in the stream, so each known tool gets a structured view
   mirroring the error state's clarity:

     • Bash      — `$ command` shell line + muted description; output
                   parses stdout/stderr into terminal blocks and shows
                   noise flags (interrupted, isImage, …) only when they
                   are actually anomalous.
     • File ops  — the path is the headline; salient params (line
                   range, pattern, replacement) become labeled rows.
     • MCP/other — a key/value table of top-level fields; long values
                   truncate with an inline expander, nested objects
                   collapse behind a disclosure.

   Every section keeps a `{}` raw toggle (today's JSON, for debugging)
   and the Copy button. All strings pass through secret redaction.
─────────────────────────────────────────────────────────────────── */

/* ── Parsing helpers (exported for tests) ──────────────────────────── */

/** If a string payload is actually JSON, surface the parsed value. */
export function normalizePayload(value: unknown): unknown {
  if (typeof value === "string") {
    const t = value.trim();
    if ((t.startsWith("{") && t.endsWith("}")) || (t.startsWith("[") && t.endsWith("]"))) {
      try {
        return JSON.parse(t);
      } catch {
        return value;
      }
    }
  }
  return value;
}

export interface ParsedBashOutput {
  stdout: string;
  stderr: string;
  /** Flags that are true (anomalous) — false/absent ones are noise. */
  flags: string[];
}

/** Boolean noise flags on Bash tool responses — surfaced only when true. */
const BASH_FLAG_KEYS = ["interrupted", "isImage", "noOutputExpected", "timedOut", "truncated"];

/**
 * Parse a Bash tool_response into stdout/stderr + anomalous flags.
 * Returns null when the payload has no recognizable shell shape (the
 * caller falls back to the generic key/value view).
 */
export function parseBashOutput(output: unknown): ParsedBashOutput | null {
  const v = normalizePayload(output);
  if (typeof v === "string") {
    return { stdout: v, stderr: "", flags: [] };
  }
  if (v && typeof v === "object" && !Array.isArray(v)) {
    const obj = v as Record<string, unknown>;
    if ("stdout" in obj || "stderr" in obj) {
      const flags = BASH_FLAG_KEYS.filter((k) => obj[k] === true);
      return {
        stdout: typeof obj.stdout === "string" ? obj.stdout : "",
        stderr: typeof obj.stderr === "string" ? obj.stderr : "",
        flags,
      };
    }
  }
  return null;
}

/** Extract plain text from MCP-style content blocks: [{type:"text",text}]. */
export function contentBlocksText(value: unknown): string | null {
  if (!Array.isArray(value) || value.length === 0) return null;
  const texts: string[] = [];
  for (const item of value) {
    if (item && typeof item === "object" && (item as Record<string, unknown>).type === "text" && typeof (item as Record<string, unknown>).text === "string") {
      texts.push((item as Record<string, unknown>).text as string);
    } else {
      return null;
    }
  }
  return texts.join("\n");
}

const FILE_TOOLS = new Set(["Read", "Write", "Edit", "NotebookEdit", "Grep", "Glob"]);

/* ── Small building blocks ─────────────────────────────────────────── */

function FlagChip({ label }: { label: string }) {
  return (
    <span className="inline-flex items-center px-1.5 py-0.5 rounded-md bg-mycel-warning-subtle text-mycel-warning text-[10px] font-mono leading-none">
      {label}
    </span>
  );
}

/** Terminal-styled text block for stdout/stderr. */
function TermBlock({ label, text, tone }: { label?: string; text: string; tone?: "error" }) {
  return (
    <div className="min-w-0">
      {label && (
        <span className={`block mb-0.5 text-[9px] uppercase tracking-wide font-semibold ${tone === "error" ? "text-mycel-error" : "text-mycel-muted"}`}>
          {label}
        </span>
      )}
      <pre
        className={`rounded-md bg-mycel-bg border border-mycel-border px-2 py-1.5 whitespace-pre-wrap break-words max-h-48 overflow-y-auto text-[11px] leading-[1.5] ${
          tone === "error" ? "text-mycel-error" : "text-mycel-text-2"
        }`}
      >
        {redactSecrets(text)}
      </pre>
    </div>
  );
}

/** One labeled parameter row: `label   value` (value mono). */
function ParamRow({ label, children, title }: { label: string; children: React.ReactNode; title?: string }) {
  return (
    <div className="flex items-start gap-2 min-w-0" title={title}>
      <span className="shrink-0 w-20 text-[10px] uppercase tracking-wide text-mycel-muted pt-px select-none">{label}</span>
      <span className="min-w-0 flex-1 font-mono text-[11px] text-mycel-text-2 break-words">{children}</span>
    </div>
  );
}

function PathHeadline({ path }: { path: string }) {
  const { dir, base } = compactPath(path);
  return (
    <span className="font-mono text-[12px] min-w-0 break-all" title={path}>
      <span className="text-mycel-muted">{dir}</span>
      <span className="text-mycel-text font-medium">{base}</span>
    </span>
  );
}

/** A string value that truncates with an inline expander. */
function TruncatedText({ text, max = 200 }: { text: string; max?: number }) {
  const [open, setOpen] = useState(false);
  const clean = redactSecrets(text);
  if (clean.length <= max) return <span className="whitespace-pre-wrap break-words">{clean}</span>;
  return (
    <span className="whitespace-pre-wrap break-words">
      {open ? clean : clean.slice(0, max) + "…"}{" "}
      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation();
          setOpen((v) => !v);
        }}
        className="text-mycel-accent hover:underline text-[10px] align-baseline"
      >
        {open ? "less" : `+${(clean.length - max).toLocaleString()} chars`}
      </button>
    </span>
  );
}

/** Nested object/array collapsed behind a disclosure. */
function NestedValue({ value }: { value: unknown }) {
  const [open, setOpen] = useState(false);
  const size = Array.isArray(value) ? `${value.length} item${value.length === 1 ? "" : "s"}` : `${Object.keys(value as object).length} keys`;
  const shape = Array.isArray(value) ? "[…]" : "{…}";
  return (
    <span className="min-w-0">
      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation();
          setOpen((v) => !v);
        }}
        className="text-mycel-muted hover:text-mycel-text transition-colors"
        aria-expanded={open}
      >
        {shape} {size} {open ? "▾" : "▸"}
      </button>
      {open && (
        <pre className="mt-1 rounded-md bg-mycel-bg border border-mycel-border px-2 py-1.5 whitespace-pre-wrap break-all max-h-40 overflow-y-auto text-[11px] text-mycel-muted">
          {JSON.stringify(redactValue(value), null, 2)}
        </pre>
      )}
    </span>
  );
}

/* ── Generic key/value table ───────────────────────────────────────── */

export function KeyValueView({ value }: { value: unknown }) {
  const v = normalizePayload(value);
  // Plain text (or content blocks) → one readable block, not JSON.
  const blockText = contentBlocksText(v);
  if (blockText !== null) return <TermBlock text={blockText} />;
  if (typeof v === "string") return <TermBlock text={v} />;
  if (typeof v === "number" || typeof v === "boolean") return <span className="font-mono text-[11px] text-mycel-text-2">{String(v)}</span>;
  if (v === null || v === undefined) return <span className="text-[11px] text-mycel-muted italic">empty</span>;
  if (Array.isArray(v)) {
    return (
      <pre className="rounded-md bg-mycel-bg border border-mycel-border px-2 py-1.5 whitespace-pre-wrap break-all max-h-48 overflow-y-auto text-[11px] text-mycel-muted">
        {JSON.stringify(redactValue(v), null, 2)}
      </pre>
    );
  }
  const entries = Object.entries(v as Record<string, unknown>);
  if (entries.length === 0) return <span className="text-[11px] text-mycel-muted italic">empty</span>;
  return (
    <div className="space-y-1 min-w-0">
      {entries.map(([k, val]) => (
        <div key={k} className="flex items-start gap-2 min-w-0">
          <span className="shrink-0 max-w-[140px] truncate font-mono text-[10px] text-mycel-muted pt-px" title={k}>
            {k}
          </span>
          <span className="min-w-0 flex-1 font-mono text-[11px] text-mycel-text-2">
            {typeof val === "string" ? (
              <TruncatedText text={val} />
            ) : typeof val === "number" || typeof val === "boolean" ? (
              String(val)
            ) : val === null || val === undefined ? (
              <span className="text-mycel-muted italic">null</span>
            ) : (
              <NestedValue value={val} />
            )}
          </span>
        </div>
      ))}
    </div>
  );
}

/* ── Bash ──────────────────────────────────────────────────────────── */

function BashInputView({ input }: { input: Record<string, unknown> }) {
  const command = typeof input.command === "string" ? input.command : "";
  const description = typeof input.description === "string" ? input.description : "";
  const chips: string[] = [];
  if (input.run_in_background === true) chips.push("background");
  if (typeof input.timeout === "number") chips.push(`timeout ${Math.round(input.timeout / 1000)}s`);
  return (
    <div className="space-y-1 min-w-0">
      {command && (
        <div className="flex items-start gap-1.5 rounded-md bg-mycel-bg border border-mycel-border px-2 py-1.5 min-w-0">
          <span className="shrink-0 select-none text-mycel-accent font-mono text-[11px] leading-[1.5]">$</span>
          <pre className="min-w-0 flex-1 whitespace-pre-wrap break-words font-mono text-[11px] leading-[1.5] text-mycel-text">
            {redactSecrets(command)}
          </pre>
        </div>
      )}
      {(description || chips.length > 0) && (
        <div className="flex items-center gap-1.5 min-w-0">
          {description && <span className="text-[11px] text-mycel-muted truncate">{redactSecrets(description)}</span>}
          {chips.map((c) => (
            <FlagChip key={c} label={c} />
          ))}
        </div>
      )}
      {!command && !description && <KeyValueView value={input} />}
    </div>
  );
}

function BashOutputView({ parsed }: { parsed: ParsedBashOutput }) {
  const hasOut = parsed.stdout.trim().length > 0;
  const hasErr = parsed.stderr.trim().length > 0;
  return (
    <div className="space-y-1.5 min-w-0">
      {parsed.flags.length > 0 && (
        <div className="flex items-center gap-1.5">
          {parsed.flags.map((f) => (
            <FlagChip key={f} label={f} />
          ))}
        </div>
      )}
      {hasOut && <TermBlock text={parsed.stdout} label={hasErr ? "stdout" : undefined} />}
      {hasErr && <TermBlock text={parsed.stderr} label="stderr" tone="error" />}
      {!hasOut && !hasErr && <span className="text-[11px] text-mycel-muted italic">no output</span>}
    </div>
  );
}

/* ── File tools (Read / Write / Edit / Grep / Glob) ────────────────── */

function FileToolInputView({ toolName, input }: { toolName: string; input: Record<string, unknown> }) {
  const path = typeof input.file_path === "string" ? input.file_path : typeof input.path === "string" ? input.path : "";
  const rows: React.ReactNode[] = [];

  if (toolName === "Read") {
    const offset = typeof input.offset === "number" ? input.offset : undefined;
    const limit = typeof input.limit === "number" ? input.limit : undefined;
    if (offset !== undefined || limit !== undefined) {
      const from = offset ?? 1;
      rows.push(
        <ParamRow key="range" label="lines">
          {limit !== undefined ? `${from}–${from + limit - 1}` : `from ${from}`}
        </ParamRow>,
      );
    }
    if (typeof input.pages === "string") rows.push(<ParamRow key="pages" label="pages">{input.pages}</ParamRow>);
  }
  if (toolName === "Grep" || toolName === "Glob") {
    if (typeof input.pattern === "string") {
      rows.push(
        <ParamRow key="pattern" label="pattern">
          <TruncatedText text={input.pattern} max={160} />
        </ParamRow>,
      );
    }
    if (typeof input.glob === "string") rows.push(<ParamRow key="glob" label="glob">{redactSecrets(input.glob)}</ParamRow>);
    if (typeof input.type === "string") rows.push(<ParamRow key="type" label="type">{redactSecrets(input.type)}</ParamRow>);
    if (typeof input.output_mode === "string") rows.push(<ParamRow key="mode" label="mode">{redactSecrets(input.output_mode)}</ParamRow>);
  }
  if (toolName === "Edit") {
    if (typeof input.old_string === "string") {
      rows.push(
        <ParamRow key="replace" label="replace">
          <TruncatedText text={input.old_string} max={160} />
        </ParamRow>,
      );
    }
    if (typeof input.new_string === "string") {
      rows.push(
        <ParamRow key="with" label="with">
          <TruncatedText text={input.new_string} max={160} />
        </ParamRow>,
      );
    }
  }
  if (toolName === "Write" && typeof input.content === "string") {
    const lines = input.content.split("\n").length;
    rows.push(
      <ParamRow key="content" label="content">
        {`${lines.toLocaleString()} line${lines === 1 ? "" : "s"} · ${input.content.length.toLocaleString()} chars`}{" "}
        <NestedValue value={{ content: input.content }} />
      </ParamRow>,
    );
  }

  // Flags worth a chip when true.
  const chips: string[] = [];
  if (input.replace_all === true) chips.push("replace all");
  if (input["-i"] === true) chips.push("case-insensitive");

  if (!path && rows.length === 0) return <KeyValueView value={input} />;
  return (
    <div className="space-y-1 min-w-0">
      {path && (
        <div className="flex items-center gap-1.5 min-w-0">
          <PathHeadline path={path} />
          {chips.map((c) => (
            <FlagChip key={c} label={c} />
          ))}
        </div>
      )}
      {rows}
    </div>
  );
}

/* ── Dispatchers ───────────────────────────────────────────────────── */

export function RichToolInput({ toolName, input }: { toolName: string; input: unknown }) {
  const v = normalizePayload(input);
  if (v && typeof v === "object" && !Array.isArray(v)) {
    const obj = v as Record<string, unknown>;
    if (toolName === "Bash" || toolName === "bash") return <BashInputView input={obj} />;
    if (FILE_TOOLS.has(toolName)) return <FileToolInputView toolName={toolName} input={obj} />;
  }
  return <KeyValueView value={v} />;
}

export function RichToolOutput({ toolName, output }: { toolName: string; output: unknown }) {
  if (toolName === "Bash" || toolName === "bash" || toolName === "BashOutput") {
    const parsed = parseBashOutput(output);
    if (parsed) return <BashOutputView parsed={parsed} />;
  }
  return <KeyValueView value={output} />;
}

/* ── Section wrapper: label · raw toggle · copy ────────────────────── */

export function DetailSection({
  label,
  tone,
  json,
  toolName,
  children,
}: {
  label: string;
  tone: "muted" | "success";
  /** Pretty-printed redacted JSON — the raw view + the copy payload. */
  json: string;
  toolName: string;
  children: React.ReactNode;
}) {
  const [raw, setRaw] = useState(false);
  const lowerLabel = label.toLowerCase();
  return (
    <div className="text-[11px] font-mono px-3 py-2 bg-mycel-surface mx-3 mb-1 rounded-md overflow-x-auto">
      <div className="flex items-center justify-between mb-1 gap-2">
        <span className={`text-[10px] uppercase tracking-wide font-semibold ${tone === "success" ? "text-mycel-success" : "text-mycel-muted"}`}>
          {label}
        </span>
        {/* One tidy control set: a segmented Formatted/Raw view switch and
            an icon copy — grouped so they read as part of the row. */}
        <span className="flex items-center gap-1">
          <span
            role="group"
            aria-label={`${toolName} ${lowerLabel} view`}
            className="inline-flex items-center rounded-md border border-mycel-border overflow-hidden text-[10px] leading-none"
          >
            <button
              type="button"
              onClick={(e) => { e.stopPropagation(); setRaw(false); }}
              aria-pressed={!raw}
              title="Formatted view"
              className={`px-1.5 py-1 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-inset ${
                raw ? "text-mycel-muted hover:text-mycel-text hover:bg-mycel-surface-hover" : "bg-mycel-accent-subtle text-mycel-accent"
              }`}
            >
              Formatted
            </button>
            <button
              type="button"
              onClick={(e) => { e.stopPropagation(); setRaw(true); }}
              aria-pressed={raw}
              aria-label={`Toggle raw JSON for ${toolName} ${lowerLabel}`}
              title="Raw JSON"
              className={`px-1.5 py-1 border-l border-mycel-border transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-inset ${
                raw ? "bg-mycel-accent-subtle text-mycel-accent" : "text-mycel-muted hover:text-mycel-text hover:bg-mycel-surface-hover"
              }`}
            >
              Raw
            </button>
          </span>
          <CopyButton text={json} />
        </span>
      </div>
      {raw ? (
        <pre className="whitespace-pre-wrap break-all text-mycel-muted max-h-48 overflow-y-auto">{json}</pre>
      ) : (
        children
      )}
    </div>
  );
}
