import { useCallback, useEffect, useState } from "react";
import { api } from "../../api/client";
import type { MCPServer } from "../../api/client";
import { MONO } from "../../utils/typography";

interface McpEnvEditorProps {
  /** Names of MCP servers to expose — typically the agent's subscribed MCPs. */
  serverNames: string[];
}

// Edits the env map for each MCP server in `serverNames`. The env is
// global to the MCP config (shared by every agent that uses it) — the
// label makes that explicit so edits don't feel agent-local.
//
// Expands one server at a time; the collapsed row shows the server name,
// enabled badge, and a compact count of env keys. Expansion reveals an
// editable key/value table. Save is explicit (button) rather than on
// blur so a partial edit can't accidentally persist.
export function McpEnvEditor({ serverNames }: McpEnvEditorProps) {
  const [servers, setServers] = useState<MCPServer[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [expanded, setExpanded] = useState<string | null>(null);

  const load = useCallback(() => {
    setError(null);
    api
      .listMCP()
      .then((all) => {
        const filtered = all.filter((s) => serverNames.includes(s.name));
        setServers(filtered);
      })
      .catch((err: unknown) => {
        setServers([]);
        setError(err instanceof Error ? err.message : "Failed to load MCP env");
      });
  }, [serverNames]);

  useEffect(() => {
    load();
  }, [load]);

  if (servers === null) {
    return (
      <p className="text-[11px] text-mycel-muted" style={{ fontFamily: MONO }}>
        Loading MCP env…
      </p>
    );
  }
  if (servers.length === 0) {
    return (
      <p className="text-[11px] text-mycel-muted" style={{ fontFamily: MONO }}>
        {serverNames.length === 0
          ? "No MCP servers attached."
          : "Attached servers aren't in the MCP registry yet."}
      </p>
    );
  }

  return (
    <div className="flex flex-col gap-2">
      {error && (
        <p className="text-[11px] text-mycel-error" style={{ fontFamily: MONO }}>
          {error}
        </p>
      )}
      {servers.map((srv) => {
        const isOpen = expanded === srv.name;
        const envCount = Object.keys(srv.env ?? {}).length;
        return (
          <div
            key={srv.name}
            className="rounded border border-mycel-border/40 bg-mycel-surface/20"
          >
            <button
              type="button"
              onClick={() => setExpanded(isOpen ? null : srv.name)}
              className="w-full flex items-center justify-between px-3 py-2 text-left hover:bg-mycel-surface/40 transition-colors"
            >
              <span className="flex items-center gap-2">
                <span className="font-medium text-mycel-text text-[12px]" style={{ fontFamily: MONO }}>
                  {srv.name}
                </span>
                {srv.enabled === false && (
                  <span className="text-[10px] uppercase tracking-wide text-mycel-muted">
                    disabled
                  </span>
                )}
                <span className="text-[10px] text-mycel-muted">
                  {envCount === 0 ? "no env" : `${envCount} env ${envCount === 1 ? "var" : "vars"}`}
                </span>
              </span>
              <svg
                width="10"
                height="10"
                viewBox="0 0 10 10"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.5"
                className={`text-mycel-muted transition-transform ${isOpen ? "rotate-90" : ""}`}
              >
                <path d="M3 1l4 4-4 4" strokeLinecap="round" strokeLinejoin="round" />
              </svg>
            </button>
            {isOpen && (
              <EnvEditor
                server={srv}
                onSaved={(updated) =>
                  setServers((prev) =>
                    prev ? prev.map((s) => (s.name === updated.name ? updated : s)) : prev,
                  )
                }
              />
            )}
          </div>
        );
      })}
      <p className="text-[10px] text-mycel-muted leading-relaxed" style={{ fontFamily: MONO }}>
        MCP env is shared across every agent that uses the server. To override a
        value per-agent, set it in the agent's Environment section above.
      </p>
    </div>
  );
}

// EnvEditor is the inner form for one MCP server. Kept local so
// collapsed rows don't hold on to per-row state.
function EnvEditor({
  server,
  onSaved,
}: {
  server: MCPServer;
  onSaved: (updated: MCPServer) => void;
}) {
  const toPairs = (env: Record<string, string> | undefined) =>
    Object.entries(env ?? {}).map(([key, value]) => ({ key, value }));

  const [pairs, setPairs] = useState(toPairs(server.env));
  const [newKey, setNewKey] = useState("");
  const [newValue, setNewValue] = useState("");
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  // Reset when the server identity changes (different expansion, or
  // updated record from a save).
  useEffect(() => {
    setPairs(toPairs(server.env));
    setDirty(false);
    setSaveError(null);
  }, [server.name, server.env]);

  const update = (i: number, field: "key" | "value", val: string) => {
    setPairs((prev) => prev.map((p, idx) => (idx === i ? { ...p, [field]: val } : p)));
    setDirty(true);
  };

  const remove = (i: number) => {
    setPairs((prev) => prev.filter((_, idx) => idx !== i));
    setDirty(true);
  };

  const addNew = () => {
    if (!newKey.trim()) return;
    setPairs((prev) => [...prev, { key: newKey.trim(), value: newValue }]);
    setNewKey("");
    setNewValue("");
    setDirty(true);
  };

  const save = () => {
    setSaving(true);
    setSaveError(null);
    const envMap: Record<string, string> = {};
    for (const p of pairs) {
      if (!p.key) continue;
      envMap[p.key] = p.value;
    }
    api
      .updateMCPEnv(server.name, envMap)
      .then((updated) => {
        onSaved(updated);
        setDirty(false);
      })
      .catch((err: unknown) => {
        setSaveError(err instanceof Error ? err.message : "Failed to save env");
      })
      .finally(() => setSaving(false));
  };

  return (
    <div className="px-3 pb-3 border-t border-mycel-border/40">
      {pairs.length === 0 && (
        <p className="py-2 text-[11px] text-mycel-muted" style={{ fontFamily: MONO }}>
          No env variables set.
        </p>
      )}
      {pairs.map((p, i) => (
        <div key={i} className="flex gap-2 mt-2">
          <input
            type="text"
            value={p.key}
            onChange={(e) => update(i, "key", e.target.value)}
            className="flex-1 bg-mycel-bg border border-mycel-border/40 rounded px-2 py-1 text-[12px] text-mycel-text font-mono outline-none focus:border-mycel-accent"
          />
          <input
            type="text"
            value={p.value}
            onChange={(e) => update(i, "value", e.target.value)}
            className="flex-1 bg-mycel-bg border border-mycel-border/40 rounded px-2 py-1 text-[12px] text-mycel-text font-mono outline-none focus:border-mycel-accent"
          />
          <button
            type="button"
            onClick={() => remove(i)}
            className="px-2 py-1 text-[11px] text-mycel-muted hover:text-mycel-error"
            style={{ fontFamily: MONO }}
          >
            ✕
          </button>
        </div>
      ))}

      <div className="flex gap-2 mt-2 pt-2 border-t border-mycel-border/15">
        <input
          type="text"
          value={newKey}
          onChange={(e) => setNewKey(e.target.value)}
          placeholder="KEY"
          className="flex-1 bg-mycel-bg border border-mycel-border/40 rounded px-2 py-1 text-[12px] text-mycel-muted font-mono outline-none focus:border-mycel-accent"
        />
        <input
          type="text"
          value={newValue}
          onChange={(e) => setNewValue(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") addNew();
          }}
          placeholder="value"
          className="flex-1 bg-mycel-bg border border-mycel-border/40 rounded px-2 py-1 text-[12px] text-mycel-muted font-mono outline-none focus:border-mycel-accent"
        />
        <button
          type="button"
          onClick={addNew}
          disabled={!newKey.trim()}
          className="px-3 py-1 text-[11px] rounded border border-mycel-border/40 text-mycel-muted hover:text-mycel-text hover:border-mycel-border disabled:opacity-30"
          style={{ fontFamily: MONO }}
        >
          Add
        </button>
      </div>

      <div className="flex items-center justify-between mt-3">
        {saveError ? (
          <span className="text-[11px] text-mycel-error" style={{ fontFamily: MONO }}>
            {saveError}
          </span>
        ) : (
          <span className="text-[10px] text-mycel-muted" style={{ fontFamily: MONO }}>
            {dirty ? "Unsaved changes" : "Synced"}
          </span>
        )}
        <button
          type="button"
          onClick={save}
          disabled={!dirty || saving}
          className="px-3 py-1 text-[11px] rounded border border-mycel-accent/40 text-mycel-accent hover:bg-mycel-accent/10 disabled:opacity-30"
          style={{ fontFamily: MONO }}
        >
          {saving ? "Saving…" : "Save"}
        </button>
      </div>
    </div>
  );
}
