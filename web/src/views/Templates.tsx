import { useCallback, useEffect, useState } from "react";
import { LoadingSkeleton } from "../components/LoadingSkeleton";
import { EmptyState } from "../components/EmptyState";
import { usePolling } from "../hooks/usePolling";
import { ChipList, SectionRule, ConfirmButton, SearchInput } from "../components/shared";
import { MONO } from "../utils/typography";

import { useHeaderSlot } from "../context/HeaderSlotContext";
// ─── Types ───────────────────────────────────────────────────────────────────

interface Template {
  name: string;
  description: string;
  mcps: string[];
  secrets: string[];
  plugins: string[];
}

interface TemplateDetail extends Template {
  system_prompt: string;
}

// ─── API helpers ─────────────────────────────────────────────────────────────

const fetchTemplates = (): Promise<Template[]> =>
  fetch("/api/templates").then((r) => {
    if (!r.ok) throw new Error(`API error: ${r.status}`);
    return r.json() as Promise<Template[]>;
  });

const fetchTemplate = (name: string): Promise<TemplateDetail> =>
  fetch(`/api/templates/${encodeURIComponent(name)}`).then((r) => {
    if (!r.ok) throw new Error(`API error: ${r.status}`);
    return r.json() as Promise<TemplateDetail>;
  });

const createTemplate = (body: {
  name: string;
  description: string;
  system_prompt: string;
  mcps: string[];
  secrets: string[];
  plugins: string[];
}): Promise<Template> =>
  fetch("/api/templates", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  }).then(async (r) => {
    if (!r.ok) {
      let msg = `API error: ${r.status}`;
      try {
        const b = (await r.json()) as { error?: string };
        if (b.error) msg = b.error;
      } catch { /* ignore */ }
      throw new Error(msg);
    }
    return r.json() as Promise<Template>;
  });

const updateTemplate = (
  name: string,
  body: {
    description: string;
    system_prompt: string;
    mcps: string[];
    secrets: string[];
    plugins: string[];
  },
): Promise<Template> =>
  fetch(`/api/templates/${encodeURIComponent(name)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  }).then(async (r) => {
    if (!r.ok) {
      let msg = `API error: ${r.status}`;
      try {
        const b = (await r.json()) as { error?: string };
        if (b.error) msg = b.error;
      } catch { /* ignore */ }
      throw new Error(msg);
    }
    return r.json() as Promise<Template>;
  });

const deleteTemplate = (name: string): Promise<void> =>
  fetch(`/api/templates/${encodeURIComponent(name)}`, {
    method: "DELETE",
  }).then((r) => {
    if (!r.ok) throw new Error(`API error: ${r.status}`);
  });

// ─── Detail view ─────────────────────────────────────────────────────────────

type SaveStatus =
  | { type: "idle" }
  | { type: "saving" }
  | { type: "success" }
  | { type: "error"; message: string };

function TemplateDetailPanel({
  name,
  onBack,
  onDeleted,
}: {
  name: string;
  onBack: () => void;
  onDeleted: () => void;
}) {
  const [detail, setDetail] = useState<TemplateDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadErr, setLoadErr] = useState<string | null>(null);

  // Editable fields
  const [editing, setEditing] = useState(false);
  const [description, setDescription] = useState("");
  const [systemPrompt, setSystemPrompt] = useState("");
  const [mcpsRaw, setMcpsRaw] = useState("");
  const [secretsRaw, setSecretsRaw] = useState("");
  const [pluginsRaw, setPluginsRaw] = useState("");

  const [saveStatus, setSaveStatus] = useState<SaveStatus>({ type: "idle" });

  // Delete
  const [deleting, setDeleting] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setLoadErr(null);
    try {
      const t = await fetchTemplate(name);
      setDetail(t);
      setDescription(t.description ?? "");
      setSystemPrompt(t.system_prompt ?? "");
      setMcpsRaw((t.mcps ?? []).join(", "));
      setSecretsRaw((t.secrets ?? []).join(", "));
      setPluginsRaw((t.plugins ?? []).join(", "));
    } catch (err) {
      setLoadErr(err instanceof Error ? err.message : "Failed to load");
    } finally {
      setLoading(false);
    }
  }, [name]);

  useEffect(() => {
    void load();
  }, [load]);

  const splitComma = (s: string) =>
    s
      .split(",")
      .map((v) => v.trim())
      .filter(Boolean);

  const handleSave = async () => {
    if (!detail) return;
    setSaveStatus({ type: "saving" });
    try {
      await updateTemplate(detail.name, {
        description: description.trim(),
        system_prompt: systemPrompt,
        mcps: splitComma(mcpsRaw),
        secrets: splitComma(secretsRaw),
        plugins: splitComma(pluginsRaw),
      });
      setSaveStatus({ type: "success" });
      setEditing(false);
      await load();
      setTimeout(() => setSaveStatus({ type: "idle" }), 2000);
    } catch (err) {
      setSaveStatus({
        type: "error",
        message: err instanceof Error ? err.message : "Save failed",
      });
      setTimeout(() => setSaveStatus({ type: "idle" }), 4000);
    }
  };

  const handleDelete = async () => {
    setDeleting(true);
    try {
      await deleteTemplate(name);
      onDeleted();
    } catch {
      setDeleting(false);
    }
  };

  if (loading) {
    return (
      <div className="p-6 space-y-4">
        <div className="h-5 w-24 animate-pulse rounded-md bg-mycel-surface-hover" />
        <LoadingSkeleton variant="text" rows={6} />
      </div>
    );
  }

  if (loadErr || !detail) {
    return (
      <div className="p-6">
        <EmptyState
          icon="!"
          title="Failed to load template"
          description={loadErr ?? "Unknown error"}
          actionLabel="Back"
          onAction={onBack}
        />
      </div>
    );
  }

  return (
    <div className="p-6 space-y-4">
      {/* Breadcrumb header */}
      <div className="flex items-center gap-3">
        <button
          type="button"
          onClick={onBack}
          className="text-xs text-mycel-muted hover:text-mycel-text transition-colors focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg rounded-md"
          aria-label="Back to templates list"
        >
          ← Templates
        </button>
        <span className="text-mycel-muted">/</span>
        <h1 className="text-xl font-semibold tracking-tight text-mycel-text">
          {detail.name}
        </h1>
      </div>

      <div className="rounded-lg border border-mycel-border bg-mycel-surface p-5 space-y-5">
        {/* Description */}
        <div className="space-y-1">
          <SectionRule label="Description" />
          {editing ? (
            <input
              type="text"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              className="w-full px-3 py-2 rounded-md border border-mycel-border bg-mycel-bg text-mycel-text text-sm focus:outline-none focus:ring-2 focus:ring-mycel-accent"
              placeholder="Short description"
              aria-label="Description"
            />
          ) : (
            <p className="text-sm text-mycel-muted">
              {detail.description || <span className="text-mycel-muted">—</span>}
            </p>
          )}
        </div>

        {/* System prompt */}
        <div className="space-y-1">
          <SectionRule label="System Prompt" />
          {editing ? (
            <textarea
              value={systemPrompt}
              onChange={(e) => setSystemPrompt(e.target.value)}
              className="w-full min-h-[200px] px-3 py-2 rounded-md border border-mycel-border bg-mycel-bg text-mycel-text text-sm resize-y focus:outline-none focus:ring-2 focus:ring-mycel-accent"
              style={{ fontFamily: MONO }}
              placeholder="# Template Name&#10;&#10;System prompt instructions..."
              aria-label="System prompt"
            />
          ) : (
            <pre
              className="text-xs bg-mycel-bg rounded-md p-3 whitespace-pre-wrap text-mycel-text-2 border border-mycel-border min-h-[80px]"
              style={{ fontFamily: MONO }}
            >
              {detail.system_prompt?.trim() || (
                <span className="text-mycel-muted">No system prompt defined.</span>
              )}
            </pre>
          )}
        </div>

        {/* MCPs */}
        <div className="space-y-1">
          <SectionRule label="MCPs" />
          {editing ? (
            <input
              type="text"
              value={mcpsRaw}
              onChange={(e) => setMcpsRaw(e.target.value)}
              className="w-full px-3 py-2 rounded-md border border-mycel-border bg-mycel-bg text-mycel-text text-sm focus:outline-none focus:ring-2 focus:ring-mycel-accent"
              placeholder="bc, github"
              aria-label="MCP servers (comma-separated)"
              style={{ fontFamily: MONO }}
            />
          ) : (
            <ChipList items={detail.mcps ?? []} color="accent" />
          )}
        </div>

        {/* Secrets */}
        <div className="space-y-1">
          <SectionRule label="Secrets" />
          {editing ? (
            <input
              type="text"
              value={secretsRaw}
              onChange={(e) => setSecretsRaw(e.target.value)}
              className="w-full px-3 py-2 rounded-md border border-mycel-border bg-mycel-bg text-mycel-text text-sm focus:outline-none focus:ring-2 focus:ring-mycel-accent"
              placeholder="GITHUB_TOKEN, OPENAI_KEY"
              aria-label="Secrets (comma-separated)"
              style={{ fontFamily: MONO }}
            />
          ) : (
            <ChipList items={detail.secrets ?? []} color="yellow" />
          )}
        </div>

        {/* Plugins */}
        <div className="space-y-1">
          <SectionRule label="Plugins" />
          {editing ? (
            <input
              type="text"
              value={pluginsRaw}
              onChange={(e) => setPluginsRaw(e.target.value)}
              className="w-full px-3 py-2 rounded-md border border-mycel-border bg-mycel-bg text-mycel-text text-sm focus:outline-none focus:ring-2 focus:ring-mycel-accent"
              placeholder="frontend-design"
              aria-label="Plugins (comma-separated)"
              style={{ fontFamily: MONO }}
            />
          ) : (
            <ChipList items={detail.plugins ?? []} color="green" />
          )}
        </div>

        {/* Actions */}
        <div className="flex items-center justify-between pt-2 border-t border-mycel-border">
          <div className="flex items-center gap-3">
            {editing ? (
              <>
                <button
                  type="button"
                  onClick={() => void handleSave()}
                  disabled={saveStatus.type === "saving"}
                  className="inline-flex items-center h-9 px-3 rounded-md bg-mycel-accent text-mycel-accent-fg text-sm font-medium hover:bg-mycel-accent-hover shadow-mycel-sm disabled:opacity-50 transition-colors focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg"
                >
                  {saveStatus.type === "saving" ? "Saving..." : "Save"}
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setEditing(false);
                    setSaveStatus({ type: "idle" });
                    // Reset to loaded values
                    setDescription(detail.description ?? "");
                    setSystemPrompt(detail.system_prompt ?? "");
                    setMcpsRaw((detail.mcps ?? []).join(", "));
                    setSecretsRaw((detail.secrets ?? []).join(", "));
                    setPluginsRaw((detail.plugins ?? []).join(", "));
                  }}
                  className="inline-flex items-center h-9 px-3 rounded-md bg-mycel-surface border border-mycel-border text-mycel-text-2 text-sm hover:text-mycel-text hover:bg-mycel-surface-hover transition-colors focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg"
                >
                  Cancel
                </button>
                {saveStatus.type === "success" && (
                  <span className="text-xs text-mycel-success">Saved</span>
                )}
                {saveStatus.type === "error" && (
                  <span className="text-xs text-mycel-error">
                    {saveStatus.message}
                  </span>
                )}
              </>
            ) : (
              <button
                type="button"
                onClick={() => setEditing(true)}
                className="inline-flex items-center h-9 px-3 rounded-md bg-mycel-surface border border-mycel-border text-mycel-text-2 text-sm hover:text-mycel-text hover:bg-mycel-surface-hover transition-colors focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg"
              >
                Edit
              </button>
            )}
          </div>

          {/* Delete */}
          <ConfirmButton
            label="Delete"
            confirmLabel="Confirm delete"
            onConfirm={() => void handleDelete()}
            loading={deleting}
            variant="danger"
          />
        </div>
      </div>
    </div>
  );
}

// ─── Create form ─────────────────────────────────────────────────────────────

function CreateTemplateForm({ onCreated }: { onCreated: (name: string) => void }) {
  const [expanded, setExpanded] = useState(false);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [systemPrompt, setSystemPrompt] = useState("");
  const [mcpsRaw, setMcpsRaw] = useState("bc");
  const [secretsRaw, setSecretsRaw] = useState("");
  const [pluginsRaw, setPluginsRaw] = useState("");
  const [status, setStatus] = useState<SaveStatus>({ type: "idle" });

  const splitComma = (s: string) =>
    s
      .split(",")
      .map((v) => v.trim())
      .filter(Boolean);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmedName = name.trim();
    if (!trimmedName) return;

    setStatus({ type: "saving" });
    try {
      const t = await createTemplate({
        name: trimmedName,
        description: description.trim(),
        system_prompt: systemPrompt,
        mcps: splitComma(mcpsRaw),
        secrets: splitComma(secretsRaw),
        plugins: splitComma(pluginsRaw),
      });
      setName("");
      setDescription("");
      setSystemPrompt("");
      setMcpsRaw("bc");
      setSecretsRaw("");
      setPluginsRaw("");
      setExpanded(false);
      setStatus({ type: "success" });
      setTimeout(() => setStatus({ type: "idle" }), 2000);
      onCreated(t.name);
    } catch (err) {
      setStatus({
        type: "error",
        message: err instanceof Error ? err.message : "Failed to create",
      });
      setTimeout(() => setStatus({ type: "idle" }), 4000);
    }
  };

  if (!expanded) {
    return (
      <div className="flex items-center gap-3">
        <button
          type="button"
          onClick={() => setExpanded(true)}
          className="inline-flex items-center gap-2 h-9 px-3 rounded-md border border-dashed border-mycel-border text-sm text-mycel-muted hover:text-mycel-accent hover:border-mycel-accent transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg"
        >
          <span className="text-lg leading-none">+</span> Create Template
        </button>
        {status.type === "success" && (
          <span className="text-xs text-mycel-success">Template created</span>
        )}
      </div>
    );
  }

  return (
    <form
      onSubmit={(e) => void handleSubmit(e)}
      className="rounded-lg border border-mycel-border bg-mycel-surface p-5 space-y-4 shadow-mycel"
    >
      <div className="flex items-center justify-between">
        <h2 className="text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">
          Create Template
        </h2>
        <button
          type="button"
          onClick={() => setExpanded(false)}
          className="text-xs text-mycel-muted hover:text-mycel-text transition-colors focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg rounded-md"
          aria-label="Cancel creating template"
        >
          Cancel
        </button>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <div className="space-y-1">
          <label className="block text-sm text-mycel-text" htmlFor="tpl-name">
            Name <span className="text-mycel-error">*</span>
          </label>
          <input
            id="tpl-name"
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="my-template"
            required
            className="w-full px-3 py-2 rounded-md border border-mycel-border bg-mycel-bg text-mycel-text text-sm focus:outline-none focus:ring-2 focus:ring-mycel-accent"
            style={{ fontFamily: MONO }}
          />
        </div>
        <div className="space-y-1">
          <label className="block text-sm text-mycel-text" htmlFor="tpl-desc">
            Description
          </label>
          <input
            id="tpl-desc"
            type="text"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Short description"
            className="w-full px-3 py-2 rounded-md border border-mycel-border bg-mycel-bg text-mycel-text text-sm focus:outline-none focus:ring-2 focus:ring-mycel-accent"
          />
        </div>
        <div className="space-y-1">
          <label className="block text-sm text-mycel-text" htmlFor="tpl-mcps">
            MCPs
          </label>
          <input
            id="tpl-mcps"
            type="text"
            value={mcpsRaw}
            onChange={(e) => setMcpsRaw(e.target.value)}
            placeholder="bc, github"
            className="w-full px-3 py-2 rounded-md border border-mycel-border bg-mycel-bg text-mycel-text text-sm focus:outline-none focus:ring-2 focus:ring-mycel-accent"
            style={{ fontFamily: MONO }}
          />
        </div>
        <div className="space-y-1">
          <label className="block text-sm text-mycel-text" htmlFor="tpl-secrets">
            Secrets
          </label>
          <input
            id="tpl-secrets"
            type="text"
            value={secretsRaw}
            onChange={(e) => setSecretsRaw(e.target.value)}
            placeholder="GITHUB_TOKEN"
            className="w-full px-3 py-2 rounded-md border border-mycel-border bg-mycel-bg text-mycel-text text-sm focus:outline-none focus:ring-2 focus:ring-mycel-accent"
            style={{ fontFamily: MONO }}
          />
        </div>
        <div className="space-y-1 sm:col-span-2">
          <label className="block text-sm text-mycel-text" htmlFor="tpl-plugins">
            Plugins
          </label>
          <input
            id="tpl-plugins"
            type="text"
            value={pluginsRaw}
            onChange={(e) => setPluginsRaw(e.target.value)}
            placeholder="frontend-design"
            className="w-full px-3 py-2 rounded-md border border-mycel-border bg-mycel-bg text-mycel-text text-sm focus:outline-none focus:ring-2 focus:ring-mycel-accent"
            style={{ fontFamily: MONO }}
          />
        </div>
      </div>

      <div className="space-y-1">
        <label className="block text-sm text-mycel-text" htmlFor="tpl-prompt">
          System Prompt
        </label>
        <textarea
          id="tpl-prompt"
          value={systemPrompt}
          onChange={(e) => setSystemPrompt(e.target.value)}
          placeholder="# Template Name&#10;&#10;System prompt instructions in Markdown..."
          className="w-full min-h-[200px] px-3 py-2 rounded-md border border-mycel-border bg-mycel-bg text-mycel-text text-sm resize-y focus:outline-none focus:ring-2 focus:ring-mycel-accent"
          style={{ fontFamily: MONO }}
        />
      </div>

      <div className="flex items-center gap-3">
        <button
          type="submit"
          disabled={status.type === "saving" || !name.trim()}
          className="inline-flex items-center h-9 px-3 rounded-md bg-mycel-accent text-mycel-accent-fg text-sm font-medium hover:bg-mycel-accent-hover shadow-mycel-sm disabled:opacity-50 transition-colors focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg"
        >
          {status.type === "saving" ? "Creating..." : "Create Template"}
        </button>
        {status.type === "success" && (
          <span className="text-xs text-mycel-success">Template created</span>
        )}
        {status.type === "error" && (
          <span className="text-xs text-mycel-error">{status.message}</span>
        )}
      </div>
    </form>
  );
}

// ─── Template list row ────────────────────────────────────────────────────────

function TemplateRow({
  template,
  onClick,
}: {
  template: Template;
  onClick: () => void;
}) {
  return (
    <tr
      className="border-t border-mycel-border hover:bg-mycel-surface-hover transition-colors cursor-pointer"
      onClick={onClick}
      role="button"
      tabIndex={0}
      aria-label={`View template ${template.name}`}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onClick();
        }
      }}
    >
      <td
        className="py-3 pl-4 pr-6 text-sm font-medium text-mycel-text whitespace-nowrap"
        style={{ fontFamily: MONO }}
      >
        {template.name}
      </td>
      <td className="py-3 px-4">
        <ChipList items={template.mcps ?? []} color="accent" />
      </td>
      <td className="py-3 px-4">
        <ChipList items={template.secrets ?? []} color="yellow" />
      </td>
      <td className="py-3 px-4 text-sm text-mycel-muted">
        {template.description || <span className="text-mycel-muted">{"\u2014"}</span>}
      </td>
    </tr>
  );
}

// ─── Main export ─────────────────────────────────────────────────────────────

export function Templates() {
  const [selectedTemplate, setSelectedTemplate] = useState<string | null>(null);
  const [search, setSearch] = useState("");

  const fetcher = useCallback(() => fetchTemplates(), []);
  const {
    data: templates,
    loading,
    error,
    refresh,
    timedOut,
  } = usePolling(fetcher, 30000);

  useHeaderSlot({
    actions:
      templates !== null ? (
        <span className="text-[11px] text-mycel-muted tabular-nums">
          {templates.length}
        </span>
      ) : undefined,
  });

  const filteredTemplates = templates
    ? templates.filter(
        (t) =>
          t.name.toLowerCase().includes(search.toLowerCase()) ||
          (t.description ?? "").toLowerCase().includes(search.toLowerCase()),
      )
    : null;

  // If a template is selected, show detail view
  if (selectedTemplate !== null) {
    return (
      <TemplateDetailPanel
        name={selectedTemplate}
        onBack={() => setSelectedTemplate(null)}
        onDeleted={() => {
          setSelectedTemplate(null);
          refresh();
        }}
      />
    );
  }

  // List view
  return (
    <div className="p-6 space-y-4">
      {/* Search */}
      <SearchInput
        value={search}
        onChange={setSearch}
        placeholder="Search templates..."
        className="w-full max-w-sm"
      />

      {/* Create form */}
      <CreateTemplateForm
        onCreated={(name) => {
          refresh();
          setSelectedTemplate(name);
        }}
      />

      {/* Loading */}
      {loading && !templates && (
        <div className="space-y-2">
          <div className="h-5 w-24 animate-pulse rounded-md bg-mycel-surface-hover" />
          <LoadingSkeleton variant="text" rows={4} />
        </div>
      )}

      {/* Timeout */}
      {timedOut && !templates && (
        <EmptyState
          icon="!"
          title="Templates took too long to load"
          description="The server may be unavailable. Check your connection and try again."
          actionLabel="Retry"
          onAction={refresh}
        />
      )}

      {/* Error */}
      {error && !templates && (
        <EmptyState
          icon="!"
          title="Failed to load templates"
          description={error}
          actionLabel="Retry"
          onAction={refresh}
        />
      )}

      {/* Empty */}
      {!loading && templates !== null && templates.length === 0 && (
        <EmptyState
          icon="T"
          title="No templates defined"
          description="Create a template to quickly spin up agents with pre-configured roles."
        />
      )}

      {/* No search results */}
      {!loading && filteredTemplates !== null && templates !== null && templates.length > 0 && filteredTemplates.length === 0 && (
        <EmptyState
          icon="T"
          title="No matching templates"
          description={`No templates match "${search}".`}
        />
      )}

      {/* Table */}
      {filteredTemplates !== null && filteredTemplates.length > 0 && (
        <div className="rounded-lg border border-mycel-border bg-mycel-surface overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="text-left text-[11px] font-medium text-mycel-muted uppercase tracking-[0.08em]">
                <th className="py-2.5 pl-4 pr-6 font-medium">Name</th>
                <th className="py-2.5 px-4 font-medium">MCPs</th>
                <th className="py-2.5 px-4 font-medium">Secrets</th>
                <th className="py-2.5 px-4 font-medium">Description</th>
              </tr>
            </thead>
            <tbody>
              {filteredTemplates.map((t) => (
                <TemplateRow
                  key={t.name}
                  template={t}
                  onClick={() => setSelectedTemplate(t.name)}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
