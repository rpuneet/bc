import { useCallback, useState } from "react";
import { api } from "../api/client";
import type { Secret } from "../api/client";
import { usePolling } from "../hooks/usePolling";
import { LoadingSkeleton } from "../components/LoadingSkeleton";
import { EmptyState } from "../components/EmptyState";

import { useHeaderSlot } from "../context/HeaderSlotContext";
import { formatRelative } from "../utils/time";

const timeAgo = (dateStr: string): string => formatRelative(dateStr);

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  const handleCopy = () => {
    navigator.clipboard.writeText(text).then(() => {
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
        <h2 className="text-base font-semibold text-mycel-text">New Secret</h2>
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
        <p className="text-xs text-mycel-error">{error}</p>
      )}

      <button
        type="submit"
        disabled={saving || !name.trim() || !value.trim()}
        className="inline-flex items-center h-9 px-3 rounded-md bg-mycel-accent text-mycel-accent-fg text-sm font-medium hover:bg-mycel-accent-hover shadow-mycel-sm disabled:opacity-50 transition-colors"
      >
        {saving ? "Creating..." : "Create Secret"}
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
            {saveError && <span className="text-xs text-mycel-error">{saveError}</span>}
          </div>
        </div>
      )}

      {/* Actions */}
      {!editing && (
        <div className="flex items-center gap-2 pt-1">
          <button
            type="button"
            onClick={() => setEditing(true)}
            className="inline-flex items-center h-8 px-3 rounded-md bg-mycel-surface border border-mycel-border text-xs text-mycel-text-2 hover:text-mycel-text hover:bg-mycel-surface-hover transition-colors"
          >
            Update Value
          </button>
          {confirming ? (
            <div className="flex items-center gap-2">
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
          ) : (
            <button
              type="button"
              onClick={() => setConfirming(true)}
              className="inline-flex items-center h-8 px-3 rounded-md text-xs text-mycel-error border border-mycel-border hover:bg-mycel-error-subtle hover:border-mycel-error transition-colors"
            >
              Delete
            </button>
          )}
        </div>
      )}
    </div>
  );
}

// --- Main View ---

export function Secrets() {
  const fetcher = useCallback(() => api.listSecrets(), []);
  const {
    data: secrets,
    loading,
    error,
    refresh,
    timedOut,
  } = usePolling(fetcher, 30000);
  const [addOpen, setAddOpen] = useState(false);

  // Header slot — count summary center-left, primary CTA right,
  // following the Agents pattern.
  useHeaderSlot({
    title: secrets ? (
      <span className="text-xs text-mycel-text-2 tabular-nums truncate">
        {String(secrets.length)} secrets
      </span>
    ) : undefined,
    actions: (
      <button
        type="button"
        onClick={() => setAddOpen((v) => !v)}
        className="shrink-0 inline-flex items-center h-8 px-3 rounded-md text-xs font-medium bg-mycel-accent text-mycel-accent-fg hover:bg-mycel-accent-hover shadow-mycel-sm transition-colors"
        aria-label="Add new secret"
      >
        + Add Secret
      </button>
    ),
  });

  if (loading && !secrets) {
    return (
      <div className="p-6 space-y-4">
        <div className="h-6 w-32 animate-pulse rounded-md bg-mycel-surface-hover" />
        <LoadingSkeleton variant="table" rows={3} />
      </div>
    );
  }

  if (timedOut && !secrets) {
    return (
      <div className="p-6">
        <EmptyState
          icon="!"
          title="Secrets took too long to load"
          description="The server may be unavailable. Check your connection and try again."
          actionLabel="Retry"
          onAction={refresh}
        />
      </div>
    );
  }

  if (error && !secrets) {
    return (
      <div className="p-6">
        <EmptyState
          icon="!"
          title="Failed to load secrets"
          description={error}
          actionLabel="Retry"
          onAction={refresh}
        />
      </div>
    );
  }

  const list = secrets ?? [];

  return (
    <div className="p-6 space-y-5">
      {/* Page note (title + count live in the top-bar chip) */}
      <p className="text-xs text-mycel-muted">
        AES-256-GCM encrypted &middot; values never exposed via API
      </p>

      {/* Add form — opened from the + Add Secret button in the top bar */}
      <AddSecretForm
        open={addOpen}
        onClose={() => setAddOpen(false)}
        onCreated={refresh}
      />

      {/* Secret cards */}
      {list.length === 0 ? (
        <EmptyState
          icon="*"
          title="No secrets stored"
          description="Click '+ Add Secret' in the top bar or run 'mycel secret set <name> --value <value>'."
        />
      ) : (
        <div className="grid gap-3">
          {list.map((s) => (
            <SecretCard key={s.name} secret={s} onChanged={refresh} />
          ))}
        </div>
      )}
    </div>
  );
}
