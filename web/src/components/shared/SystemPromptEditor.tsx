import { useRef, useState } from "react";
import { MONO } from "../../utils/typography";

interface SystemPromptEditorProps {
  value: string;
  loading?: boolean;
  onSave?: (newValue: string) => Promise<void>;
  className?: string;
}

export function SystemPromptEditor({
  value,
  loading,
  onSave,
  className,
}: SystemPromptEditorProps) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const [saving, setSaving] = useState(false);
  const [saveStatus, setSaveStatus] = useState<"idle" | "saving" | "saved" | "error">("idle");
  const [saveError, setSaveError] = useState("");
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const handleEdit = () => {
    setDraft(value);
    setSaveStatus("idle");
    setSaveError("");
    setEditing(true);
  };

  const handleCancel = () => {
    setDraft(value);
    setSaveStatus("idle");
    setSaveError("");
    setEditing(false);
  };

  const handleSave = () => {
    if (!onSave) return;
    setSaving(true);
    setSaveStatus("saving");
    setSaveError("");
    onSave(draft)
      .then(() => {
        setEditing(false);
        setSaveStatus("saved");
        if (saveTimerRef.current !== null) clearTimeout(saveTimerRef.current);
        saveTimerRef.current = setTimeout(() => setSaveStatus("idle"), 2000);
      })
      .catch((err: unknown) => {
        const msg = err instanceof Error ? err.message : "unknown error";
        setSaveStatus("error");
        setSaveError(msg);
      })
      .finally(() => {
        setSaving(false);
      });
  };

  return (
    <section className={className}>
      <div className="mb-4 flex items-center gap-3">
        <span
          className="text-[10px] font-bold uppercase tracking-[0.2em] text-mycel-muted/70"
          style={{ fontFamily: MONO }}
        >
          System Prompt
        </span>
        <span className="flex-1 h-px bg-gradient-to-r from-mycel-border/50 to-transparent" />
        {!loading && (
          <div className="flex items-center gap-2">
            {saveStatus === "saved" && (
              <span
                className="text-[11px] text-green-400 transition-opacity"
                style={{ fontFamily: MONO }}
              >
                Saved
              </span>
            )}
            {saveStatus === "error" && (
              <span
                className="text-[11px] text-mycel-error"
                style={{ fontFamily: MONO }}
                title={saveError}
              >
                Error: {saveError}
              </span>
            )}
            {editing ? (
              <>
                <span
                  className="text-[10px] text-mycel-accent/60 italic"
                  style={{ fontFamily: MONO }}
                >
                  Editing...
                </span>
                <button
                  type="button"
                  onClick={handleCancel}
                  disabled={saving}
                  className="px-2.5 py-1 rounded border border-mycel-border/40 text-[11px] text-mycel-muted hover:text-mycel-text hover:border-mycel-border transition-colors disabled:opacity-40"
                  style={{ fontFamily: MONO }}
                >
                  Cancel
                </button>
                <button
                  type="button"
                  onClick={handleSave}
                  disabled={saving}
                  className="px-2.5 py-1 rounded border border-mycel-accent/30 bg-mycel-accent/10 text-[11px] text-mycel-accent hover:bg-mycel-accent/20 transition-colors disabled:opacity-40"
                  style={{ fontFamily: MONO }}
                >
                  {saving ? "Saving…" : "Save"}
                </button>
              </>
            ) : (
              onSave && (
                <button
                  type="button"
                  onClick={handleEdit}
                  className="px-2.5 py-1 rounded border border-mycel-border/40 text-[11px] text-mycel-muted hover:text-mycel-text hover:border-mycel-border transition-colors"
                  style={{ fontFamily: MONO }}
                >
                  Edit
                </button>
              )
            )}
          </div>
        )}
      </div>

      {loading ? (
        <div className="rounded-md border border-mycel-border/40 bg-mycel-surface/20 p-4">
          <p
            className="text-xs text-mycel-muted italic"
            style={{ fontFamily: MONO }}
          >
            Loading…
          </p>
        </div>
      ) : editing ? (
        <textarea
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          className="w-full min-h-[180px] max-h-[250px] rounded-md border border-mycel-accent/50 bg-mycel-bg/80 p-4 text-xs text-mycel-text/90 leading-relaxed resize-y outline-none focus:border-mycel-accent/60 transition-colors"
          style={{ fontFamily: MONO }}
          spellCheck={false}
        />
      ) : (
        <textarea
          value={value}
          readOnly
          className="w-full min-h-[180px] max-h-[250px] rounded-md border border-mycel-border/40 bg-mycel-bg p-4 text-xs text-mycel-text/70 leading-relaxed resize-y outline-none cursor-default"
          style={{ fontFamily: MONO }}
        />
      )}
    </section>
  );
}
