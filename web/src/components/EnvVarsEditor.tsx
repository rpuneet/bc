import { useState, useCallback, useEffect, useMemo, useRef } from "react";
import { api } from "../api/client";
import { MONO } from "../utils/typography";

/* ═══════════════════════════════════════════════════════════════════
   Per-agent environment variables editor with secrets autocomplete.

   Values support `${secret:NAME}` references: typing `${` (or clicking
   the key-icon button on a row) opens a dropdown of vault secret names;
   selecting one inserts the reference. Only secret NAMES are fetched —
   values never leave the daemon.
   ═══════════════════════════════════════════════════════════════════ */

export interface EnvRow {
  key: string;
  value: string;
}

/** Matches a trailing partial secret reference being typed, e.g.
 *  "x=${", "${sec", "${secret:GIT" — capture group 1 is the typed
 *  filter text (may include the "secret:" prefix or part of it). */
const PARTIAL_REF = /\$\{([A-Za-z0-9_:-]*)$/;

/** True when an env var name is valid (mirrors the API rule). */
export function isValidEnvKey(key: string): boolean {
  return /^[A-Za-z_][A-Za-z0-9_]*$/.test(key);
}

/** Build the filter term from a partial `${...` capture: strip any
 *  typed prefix of "secret:" so `${sec` and `${secret:GIT` both filter. */
export function secretFilterTerm(capture: string): string {
  const prefix = "secret:";
  if (capture.toLowerCase().startsWith(prefix)) return capture.slice(prefix.length);
  // A partial prefix like "sec" filters nothing yet — show all.
  if (prefix.startsWith(capture.toLowerCase())) return "";
  return capture;
}

/** Insert `${secret:NAME}` into value, replacing a trailing partial
 *  `${...` if present, otherwise appending. */
export function insertSecretRef(value: string, name: string): string {
  const m = PARTIAL_REF.exec(value);
  const ref = "${secret:" + name + "}";
  if (m) return value.slice(0, m.index) + ref;
  return value + ref;
}

// Module-level cache: the vault's secret names change rarely; one fetch
// per page load is plenty for autocomplete.
let secretNamesCache: string[] | null = null;

function useSecretNames(): string[] {
  const [names, setNames] = useState<string[]>(secretNamesCache ?? []);
  useEffect(() => {
    if (secretNamesCache !== null) return;
    api
      .listSecrets()
      .then((list) => {
        secretNamesCache = list.map((s) => s.name);
        setNames(secretNamesCache);
      })
      .catch(() => {
        /* no vault or no secrets — autocomplete just stays empty */
      });
  }, []);
  return names;
}

const ROW_INPUT_CLS =
  "rounded-md border border-mycel-border-strong bg-mycel-bg px-2.5 py-1.5 text-[11px] text-mycel-text " +
  "placeholder:text-mycel-muted outline-none focus:border-mycel-accent transition-colors";

/* ── Value input with secret autocomplete ──────────────────────────── */

interface SecretValueInputProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
}

export function SecretValueInput({ value, onChange, placeholder = "value" }: SecretValueInputProps) {
  const secretNames = useSecretNames();
  const [open, setOpen] = useState(false);
  const [highlight, setHighlight] = useState(0);
  const wrapRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  // Filter: when a partial `${...` is being typed, narrow by it.
  const options = useMemo(() => {
    const m = PARTIAL_REF.exec(value);
    const term = m ? secretFilterTerm(m[1] ?? "").toLowerCase() : "";
    return secretNames.filter((n) => n.toLowerCase().includes(term));
  }, [secretNames, value]);

  // Close on outside click.
  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false);
    };
    window.addEventListener("mousedown", handler);
    return () => window.removeEventListener("mousedown", handler);
  }, [open]);

  const pick = useCallback(
    (name: string) => {
      onChange(insertSecretRef(value, name));
      setOpen(false);
      inputRef.current?.focus();
    },
    [value, onChange],
  );

  return (
    <div ref={wrapRef} className="relative flex-1 min-w-0 flex items-center gap-1">
      <input
        ref={inputRef}
        type="text"
        value={value}
        onChange={(e) => {
          const next = e.target.value;
          onChange(next);
          // Typing `${` opens the secrets dropdown.
          if (PARTIAL_REF.test(next)) {
            setOpen(true);
            setHighlight(0);
          } else {
            setOpen(false);
          }
        }}
        onKeyDown={(e) => {
          if (!open || options.length === 0) {
            if (e.key === "Escape") setOpen(false);
            return;
          }
          if (e.key === "ArrowDown") {
            e.preventDefault();
            setHighlight((h) => (h + 1) % options.length);
          } else if (e.key === "ArrowUp") {
            e.preventDefault();
            setHighlight((h) => (h - 1 + options.length) % options.length);
          } else if (e.key === "Enter") {
            e.preventDefault();
            const name = options[highlight];
            if (name) pick(name);
          } else if (e.key === "Escape") {
            setOpen(false);
          }
        }}
        placeholder={placeholder}
        spellCheck={false}
        autoComplete="off"
        className={`w-full ${ROW_INPUT_CLS}`}
        style={{ fontFamily: MONO }}
        aria-label="Environment variable value"
        aria-expanded={open}
        role="combobox"
        aria-controls="secret-autocomplete"
      />
      <button
        type="button"
        onClick={() => {
          setOpen((prev) => !prev);
          setHighlight(0);
          inputRef.current?.focus();
        }}
        title="Insert a secret reference"
        aria-label="Insert a secret reference"
        className="shrink-0 flex items-center justify-center w-7 h-7 rounded-md border border-mycel-border bg-mycel-bg text-mycel-muted hover:text-mycel-accent hover:border-mycel-accent transition-colors"
      >
        {/* key icon */}
        <svg width="12" height="12" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round">
          <circle cx="4.5" cy="9.5" r="3" />
          <path d="M6.8 7.2L12.5 1.5M10 4l2 2M8 6l1.5 1.5" />
        </svg>
      </button>

      {open && (
        <ul
          id="secret-autocomplete"
          role="listbox"
          className="absolute left-0 right-8 top-full z-50 mt-1 max-h-40 overflow-y-auto rounded-md border border-mycel-border bg-mycel-surface-2 shadow-mycel-lg py-1"
        >
          {options.length === 0 ? (
            <li className="px-2.5 py-1.5 text-[11px] text-mycel-muted italic">
              No secrets in the vault
            </li>
          ) : (
            options.map((n, i) => (
              <li key={n} role="option" aria-selected={i === highlight}>
                <button
                  type="button"
                  // mousedown so the pick fires before the input blurs
                  onMouseDown={(e) => {
                    e.preventDefault();
                    pick(n);
                  }}
                  onMouseEnter={() => setHighlight(i)}
                  className={`w-full text-left px-2.5 py-1.5 text-[11px] transition-colors truncate ${
                    i === highlight
                      ? "bg-mycel-accent-subtle text-mycel-accent"
                      : "text-mycel-text hover:bg-mycel-surface-hover"
                  }`}
                  style={{ fontFamily: MONO }}
                  title={"${secret:" + n + "}"}
                >
                  {n}
                </button>
              </li>
            ))
          )}
        </ul>
      )}
    </div>
  );
}

/* ── Row editor ────────────────────────────────────────────────────── */

interface EnvVarsEditorProps {
  rows: EnvRow[];
  onChange: (rows: EnvRow[]) => void;
}

export function EnvVarsEditor({ rows, onChange }: EnvVarsEditorProps) {
  return (
    <div className="flex flex-col gap-2">
      {rows.map((row, i) => (
        <div key={i} className="flex items-center gap-2">
          <input
            type="text"
            value={row.key}
            onChange={(e) => {
              const next = rows.slice();
              next[i] = { ...row, key: e.target.value };
              onChange(next);
            }}
            placeholder="KEY"
            spellCheck={false}
            autoComplete="off"
            className={`w-32 shrink-0 ${ROW_INPUT_CLS} ${
              row.key !== "" && !isValidEnvKey(row.key) ? "border-mycel-error" : ""
            }`}
            style={{ fontFamily: MONO }}
            aria-label="Environment variable name"
            aria-invalid={row.key !== "" && !isValidEnvKey(row.key)}
          />
          <span className="text-mycel-muted text-[11px]" style={{ fontFamily: MONO }}>=</span>
          <SecretValueInput
            value={row.value}
            onChange={(value) => {
              const next = rows.slice();
              next[i] = { ...row, value };
              onChange(next);
            }}
          />
          <button
            type="button"
            onClick={() => onChange(rows.filter((_, idx) => idx !== i))}
            className="shrink-0 text-[13px] leading-none text-mycel-muted hover:text-mycel-error transition-colors px-1"
            aria-label={`Remove ${row.key || "row"}`}
            title="Remove"
          >
            ×
          </button>
        </div>
      ))}
      <div>
        <button
          type="button"
          onClick={() => onChange([...rows, { key: "", value: "" }])}
          className="inline-flex items-center px-2.5 py-1 rounded-md border border-mycel-border bg-mycel-bg text-xs font-medium text-mycel-muted hover:text-mycel-accent hover:border-mycel-accent transition-colors"
        >
          + Add variable
        </button>
      </div>
      <p className="text-xs text-mycel-muted leading-relaxed">
        Type <span style={{ fontFamily: MONO }}>{"${"}</span> in a value (or use the key
        button) to reference a vault secret — resolved at spawn, never stored in plain text.
      </p>
    </div>
  );
}
