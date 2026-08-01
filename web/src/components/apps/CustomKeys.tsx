/**
 * CustomKeysSection — encrypted vault keys agents reference via
 * ${secret:NAME}, rendered as a section of the Apps home. App
 * credentials live with their app (app:<instance>:<field> keys, managed
 * by the connect flow); this section is for the user's own custom keys.
 * Absorbed from the old standalone /secrets page — same add / update /
 * delete flows against /api/secrets.
 */

import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../../api/client";
import type { Secret } from "../../api/client";
import { usePolling } from "../../hooks/usePolling";
import { LoadingSkeleton } from "../LoadingSkeleton";
import { EmptyState } from "../EmptyState";
import { formatRelative } from "../../utils/time";

const timeAgo = (dateStr: string): string => formatRelative(dateStr);

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  const handleCopy = () => {
    navigator.clipboard.writeText(text).catch(() => { /* clipboard permission denied */ }).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  };
  return (
    <button
      type="button"
      onClick={handleCopy}
      title="Copy to clipboard"
      className="ml-1 px-1.5 py-0.5 rounded-md text-[10px] border border-mycel-border text-mycel-muted hover:text-mycel-accent hover:border-mycel-accent transition-colors"
    >
      {copied ? "Copied" : "Copy"}
    </button>
  );
}

// --- Add Secret Form ---

function AddSecretForm({
  open,
  onClose,
  onCreated,
}: {
  open: boolean;
  onClose: () => void;
  onCreated: () => void;
}) {
  const [name, setName] = useState("");
  const [value, setValue] = useState("");
  const [description, setDescription] = useState("");
  const [showValue, setShowValue] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmedName = name.trim().toUpperCase().replace(/[^A-Z0-9_]/g, "_");
    const trimmedValue = value.trim();
    if (!trimmedName || !trimmedValue) {
      setError(!trimmedName ? "Name is required." : "Value is required.");
      return;
    }

    setSaving(true);
    setError(null);
    try {
      await api.createSecret(trimmedName, trimmedValue, description.trim());
      setName("");
      setValue("");
      setDescription("");
      setShowValue(false);
      onClose();
      onCreated();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create secret");
    } finally {
      setSaving(false);
    }
  };

  if (!open) return null;

  return (
    <form
      onSubmit={handleSubmit}
      className="rounded-lg border border-mycel-border bg-mycel-surface p-5 space-y-4 shadow-mycel"
    >
      <div className="flex items-center justify-between">
        <h2 className="text-base font-semibold text-mycel-text">New Custom Key</h2>
        <button
          type="button"
          onClick={() => { onClose(); setError(null); }}
          className="px-3 py-1 rounded-md text-xs text-mycel-muted hover:text-mycel-text border border-mycel-border hover:border-mycel-muted bg-mycel-bg transition-colors"
        >
          Cancel
        </button>
      </div>

      <div className="space-y-3">
        <div className="space-y-1">
          <label className="block text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">
            Name
          </label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value.toUpperCase().replace(/[^A-Z0-9_]/g, "_"))}
            placeholder="MY_API_KEY"
            className="w-full px-3 py-2 rounded-md border border-mycel-border bg-mycel-bg text-mycel-text text-sm font-mono focus:outline-none focus:ring-2 focus:ring-mycel-accent"
          />
        </div>

        <div className="space-y-1">
          <label className="block text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">
            Value
          </label>
          <div className="relative">
            <input
              type={showValue ? "text" : "password"}
              value={value}
              onChange={(e) => setValue(e.target.value)}
              placeholder="Enter secret value"
              className="w-full px-3 py-2 pr-16 rounded-md border border-mycel-border bg-mycel-bg text-mycel-text text-sm font-mono focus:outline-none focus:ring-2 focus:ring-mycel-accent"
            />
            <button
              type="button"
              onClick={() => setShowValue(!showValue)}
              className="absolute right-2 top-1/2 -translate-y-1/2 px-2 py-0.5 text-[11px] text-mycel-muted hover:text-mycel-text transition-colors"
            >
              {showValue ? "Hide" : "Show"}
            </button>
          </div>
        </div>

        <div className="space-y-1">
          <label className="block text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">
            Description <span className="text-mycel-muted normal-case">(optional)</span>
          </label>
          <input
            type="text"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="What this secret is used for"
            className="w-full px-3 py-2 rounded-md border border-mycel-border bg-mycel-bg text-mycel-text text-sm focus:outline-none focus:ring-2 focus:ring-mycel-accent"
          />
        </div>
      </div>

      {error && (
        <p role="alert" className="text-xs text-mycel-error">{error}</p>
      )}

      <button
        type="submit"
        disabled={saving || !name.trim() || !value.trim()}
        className="inline-flex items-center h-9 px-3 rounded-md bg-mycel-accent text-mycel-accent-fg text-sm font-medium hover:bg-mycel-accent-hover shadow-mycel-sm disabled:opacity-50 transition-colors"
      >
        {saving ? "Creating..." : "Create Key"}
      </button>
    </form>
  );
}

// --- Secret Card ---

function SecretCard({ secret, onChanged }: { secret: Secret; onChanged: () => void }) {
  const [editing, setEditing] = useState(false);
  const [newValue, setNewValue] = useState("");
  const [showValue, setShowValue] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [confirming, setConfirming] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  // Close the actions menu when clicking outside.
  useEffect(() => {
    if (!menuOpen) return;
    const handler = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node))
        setMenuOpen(false);
    };
    const keyHandler = (e: KeyboardEvent) => {
      if (e.key === "Escape") setMenuOpen(false);
    };
    document.addEventListener("mousedown", handler);
    document.addEventListener("keydown", keyHandler);
    return () => {
      document.removeEventListener("mousedown", handler);
      document.removeEventListener("keydown", keyHandler);
    };
  }, [menuOpen]);

  const handleUpdate = async () => {
    const trimmed = newValue.trim();
    if (!trimmed) return;
    setSaving(true);
    setSaveError(null);
    try {
      await api.updateSecret(secret.name, trimmed);
      setNewValue("");
      setShowValue(false);
      setEditing(false);
      onChanged();
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : "Failed to update");
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    setDeleting(true);
    try {
      await api.deleteSecret(secret.name);
      onChanged();
    } catch {
      setDeleting(false);
      setConfirming(false);
    }
  };

  const reference = `\${secret:${secret.name}}`;

  return (
    <div className="rounded-lg border border-mycel-border bg-mycel-surface p-4 space-y-3">
      {/* Header row */}
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <h3 className="font-mono font-semibold text-mycel-text text-sm truncate">
            {secret.name}
          </h3>
          {secret.description && (
            <p className="text-xs text-mycel-muted mt-0.5 line-clamp-2">
              {secret.description}
            </p>
          )}
        </div>
        <div className="flex items-center gap-1.5 shrink-0">
          <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-mycel-bg border border-mycel-border text-[11px] text-mycel-muted">
            <svg className="w-3 h-3" viewBox="0 0 16 16" fill="currentColor">
              <path d="M8 1a4 4 0 0 0-4 4v3H3a1 1 0 0 0-1 1v5a1 1 0 0 0 1 1h10a1 1 0 0 0 1-1V9a1 1 0 0 0-1-1h-1V5a4 4 0 0 0-4-4zm2 7H6V5a2 2 0 1 1 4 0v3z"/>
            </svg>
            Encrypted
          </span>
          {/* Three-dot actions menu */}
          {!editing && (
            <div ref={menuRef} className="relative">
              <button
                type="button"
                onClick={() => setMenuOpen((v) => !v)}
                aria-label="Secret actions"
                aria-haspopup="menu"
                aria-expanded={menuOpen}
                className="inline-flex items-center justify-center w-6 h-6 rounded-md text-mycel-muted hover:text-mycel-text hover:bg-mycel-surface-hover transition-colors focus:outline-none focus:ring-1 focus:ring-mycel-accent"
              >
                <svg viewBox="0 0 16 16" width="14" height="14" fill="currentColor" aria-hidden>
                  <path d="M8 9a1.5 1.5 0 1 0 0-3 1.5 1.5 0 0 0 0 3ZM1.5 9a1.5 1.5 0 1 0 0-3 1.5 1.5 0 0 0 0 3Zm13 0a1.5 1.5 0 1 0 0-3 1.5 1.5 0 0 0 0 3Z" />
                </svg>
              </button>
              {menuOpen && (
                <div
                  role="menu"
                  className="absolute right-0 top-full mt-1 z-20 w-40 rounded-lg border border-mycel-border bg-mycel-surface shadow-xl py-1"
                >
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => { setMenuOpen(false); setEditing(true); }}
                    className="w-full text-left px-3 py-1.5 text-sm text-mycel-text hover:bg-mycel-surface-hover transition-colors"
                  >
                    Update value
                  </button>
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => { setMenuOpen(false); setConfirming(true); }}
                    className="w-full text-left px-3 py-1.5 text-sm text-mycel-error hover:bg-mycel-error-subtle transition-colors"
                  >
                    Delete
                  </button>
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      {/* Usage reference */}
      <div className="flex items-center gap-1">
        <code className="text-[11px] font-mono text-mycel-muted bg-mycel-bg px-2 py-0.5 rounded-md border border-mycel-border">
          {reference}
        </code>
        <CopyButton text={reference} />
      </div>

      {/* Timestamps */}
      <div className="flex items-center gap-4 text-[11px] text-mycel-muted">
        <span>Created {timeAgo(secret.created_at)}</span>
      </div>

      {/* Inline update form */}
      {editing && (
        <div className="pt-2 border-t border-mycel-border space-y-2">
          <div className="relative">
            <input
              type={showValue ? "text" : "password"}
              value={newValue}
              onChange={(e) => setNewValue(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") handleUpdate();
                if (e.key === "Escape") {
                  setEditing(false);
                  setNewValue("");
                  setShowValue(false);
                  setSaveError(null);
                }
              }}
              placeholder="Enter new value"
              autoFocus
              className="w-full px-3 py-2 pr-16 rounded-md border border-mycel-border bg-mycel-bg text-mycel-text text-sm font-mono focus:outline-none focus:ring-2 focus:ring-mycel-accent"
              aria-label={`New value for ${secret.name}`}
            />
            <button
              type="button"
              onClick={() => setShowValue(!showValue)}
              className="absolute right-2 top-1/2 -translate-y-1/2 px-2 py-0.5 text-[11px] text-mycel-muted hover:text-mycel-text transition-colors"
            >
              {showValue ? "Hide" : "Show"}
            </button>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={handleUpdate}
              disabled={saving || !newValue.trim()}
              className="inline-flex items-center h-8 px-3 rounded-md bg-mycel-accent text-mycel-accent-fg text-xs font-medium hover:bg-mycel-accent-hover shadow-mycel-sm disabled:opacity-50 transition-colors"
            >
              {saving ? "Saving..." : "Save"}
            </button>
            <button
              type="button"
              onClick={() => {
                setEditing(false);
                setNewValue("");
                setShowValue(false);
                setSaveError(null);
              }}
              className="inline-flex items-center h-8 px-3 rounded-md bg-mycel-surface border border-mycel-border text-mycel-text-2 text-xs hover:text-mycel-text hover:bg-mycel-surface-hover transition-colors"
            >
              Cancel
            </button>
            {saveError && <span role="alert" className="text-xs text-mycel-error">{saveError}</span>}
          </div>
        </div>
      )}

      {/* Confirm-delete row — shown when the three-dot menu triggers deletion. */}
      {!editing && confirming && (
        <div className="flex items-center gap-2 pt-1">
          <button
            type="button"
            onClick={handleDelete}
            disabled={deleting}
            className="inline-flex items-center h-8 px-3 rounded-md bg-mycel-error text-white text-xs font-medium hover:opacity-90 shadow-mycel-sm disabled:opacity-50 transition-opacity"
          >
            {deleting ? "Deleting..." : "Confirm Delete"}
          </button>
          <button
            type="button"
            onClick={() => setConfirming(false)}
            disabled={deleting}
            className="inline-flex items-center h-8 px-3 rounded-md bg-mycel-surface border border-mycel-border text-mycel-text-2 text-xs hover:text-mycel-text hover:bg-mycel-surface-hover transition-colors"
          >
            Cancel
          </button>
        </div>
      )}
    </div>
  );
}

// --- Section (Apps home) ---

export function CustomKeysSection() {
  const fetcher = useCallback(() => api.listSecrets(), []);
  const {
    data: secrets,
    loading,
    error,
    refresh,
    timedOut,
  } = usePolling(fetcher, 30000);
  const [addOpen, setAddOpen] = useState(false);

  const list = secrets ?? [];

  return (
    <section id="custom-keys" aria-label="Custom keys" className="space-y-3 scroll-mt-6">
      {/* Section header — mirrors the channel-group headers above. */}
      <div className="flex items-center gap-2">
        <svg width="13" height="13" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.3" className="text-mycel-muted" aria-hidden>
          <path d="M7 2.5a2 2 0 00-2 2V6H4v4.5h6V6H9V4.5a2 2 0 00-2-2zm0 5.5a.75.75 0 110 1.5.75.75 0 010-1.5z" />
        </svg>
        <h3 className="text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">Custom Keys</h3>
        {secrets && (
          <span className="text-[11px] text-mycel-muted tabular-nums">{list.length}</span>
        )}
        <span className="hidden sm:inline text-[11px] text-mycel-muted">
          · AES-256-GCM encrypted · reference as <code className="font-mono text-mycel-text-2">{"${secret:NAME}"}</code> in agent env
        </span>
        <button
          type="button"
          onClick={() => { setAddOpen((v) => !v); }}
          className="ml-auto shrink-0 inline-flex items-center h-7 px-2.5 rounded-md text-[11px] font-medium bg-mycel-accent text-mycel-accent-fg hover:bg-mycel-accent-hover shadow-mycel-sm transition-colors"
          aria-label="Add custom key"
        >
          + Add Key
        </button>
      </div>

      {addOpen && (
        <AddSecretForm
          open={addOpen}
          onClose={() => { setAddOpen(false); }}
          onCreated={refresh}
        />
      )}

      {loading && !secrets ? (
        <LoadingSkeleton variant="table" rows={2} />
      ) : (timedOut || error) && !secrets ? (
        <EmptyState
          icon="!"
          title="Failed to load custom keys"
          description={error ?? "The server may be unavailable."}
          actionLabel="Retry"
          onAction={refresh}
        />
      ) : list.length === 0 ? (
        <div className="rounded-lg border border-mycel-border p-6 text-center text-xs text-mycel-muted">
          No custom keys yet — add one here or run <code className="font-mono text-mycel-text-2">mycel secret set &lt;name&gt;</code>.
        </div>
      ) : (
        <div className="grid gap-3">
          {list.map((s) => (
            <SecretCard key={s.name} secret={s} onChanged={refresh} />
          ))}
        </div>
      )}
    </section>
  );
}
