import { useCallback, useRef, useState } from "react";
import { api } from "../../api/client";

/** Matches pkg/attachment.MaxFilesPerMessage. */
export const MAX_FILES_PER_MESSAGE = 5;

const FILE_REF_RE = /\[file:[a-zA-Z0-9_-]+\]/g;

export function fileRef(id: string): string {
  return `[file:${id}]`;
}

function fileRefCount(text: string): number {
  return text.match(FILE_REF_RE)?.length ?? 0;
}

function appendFileRef(draft: string, id: string): string {
  const ref = fileRef(id);
  const trimmed = draft.replace(/\s+$/, "");
  return trimmed ? `${trimmed} ${ref}` : ref;
}

type Status = { kind: "error" | "warn" | "info"; text: string } | null;

export function ChannelCompose({
  channelName,
  onSent,
}: {
  channelName: string;
  onSent?: () => void;
}) {
  const [draft, setDraft] = useState("");
  const [sending, setSending] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [dragOver, setDragOver] = useState(false);
  const [status, setStatus] = useState<Status>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const dragDepth = useRef(0);

  const channelLabel = channelName.includes(":")
    ? (channelName.split(":").pop() || channelName)
    : channelName;

  const busy = sending || uploading;
  const canSend = draft.trim().length > 0 && !busy;

  const uploadFiles = useCallback(
    async (files: FileList | File[]) => {
      const list = Array.from(files).filter((f) => f.size > 0);
      if (list.length === 0) return;

      const room = MAX_FILES_PER_MESSAGE - fileRefCount(draft);
      if (room <= 0) {
        setStatus({ kind: "warn", text: `Up to ${MAX_FILES_PER_MESSAGE} files per message` });
        return;
      }
      const batch = list.slice(0, room);
      if (list.length > room) {
        setStatus({ kind: "warn", text: `Up to ${MAX_FILES_PER_MESSAGE} files per message` });
      } else {
        setStatus(null);
      }

      setUploading(true);
      try {
        let next = draft;
        for (const file of batch) {
          const meta = await api.uploadFile(file, channelName);
          next = appendFileRef(next, meta.id);
          setDraft(next);
        }
      } catch (e) {
        setStatus({
          kind: "error",
          text: e instanceof Error ? e.message : "Upload failed",
        });
      } finally {
        setUploading(false);
        textareaRef.current?.focus();
      }
    },
    [channelName, draft],
  );

  const handleSend = useCallback(async () => {
    const text = draft.trim();
    if (!text || busy) return;
    setSending(true);
    setStatus(null);
    try {
      const { sent } = await api.sendChannel(channelName, text);
      if (!sent) {
        setStatus({ kind: "warn", text: "No outbound route for this channel" });
        return;
      }
      setDraft("");
      onSent?.();
    } catch (e) {
      setStatus({
        kind: "error",
        text: e instanceof Error ? e.message : "Send failed",
      });
    } finally {
      setSending(false);
      textareaRef.current?.focus();
    }
  }, [busy, channelName, draft, onSent]);

  const onDrop = (e: React.DragEvent) => {
    e.preventDefault();
    dragDepth.current = 0;
    setDragOver(false);
    if (e.dataTransfer.files?.length) {
      void uploadFiles(e.dataTransfer.files);
    }
  };

  return (
    <div
      data-testid="channel-compose"
      className="shrink-0"
      style={{
        padding: "8px 16px 12px",
        borderTop: "1px solid var(--mycel-border)",
        background: "var(--mycel-surface)",
      }}
      onDragEnter={(e) => {
        e.preventDefault();
        dragDepth.current += 1;
        setDragOver(true);
      }}
      onDragLeave={(e) => {
        e.preventDefault();
        dragDepth.current = Math.max(0, dragDepth.current - 1);
        if (dragDepth.current === 0) setDragOver(false);
      }}
      onDragOver={(e) => {
        e.preventDefault();
        e.dataTransfer.dropEffect = "copy";
      }}
      onDrop={onDrop}
    >
      {status && (
        <div
          role="status"
          style={{
            fontSize: 11,
            marginBottom: 6,
            color:
              status.kind === "error"
                ? "var(--mycel-error)"
                : status.kind === "warn"
                  ? "var(--mycel-warning)"
                  : "var(--mycel-muted)",
          }}
        >
          {status.text}
        </div>
      )}
      <div
        className="flex items-end"
        style={{
          gap: 6,
          padding: "4px 6px 4px 4px",
          borderRadius: 8,
          border: dragOver
            ? "1px dashed var(--mycel-accent)"
            : "1px solid var(--mycel-border)",
          background: "var(--mycel-bg)",
        }}
      >
        <input
          ref={fileInputRef}
          type="file"
          multiple
          data-testid="channel-file-input"
          className="sr-only"
          tabIndex={-1}
          onChange={(e) => {
            const files = e.currentTarget.files;
            e.currentTarget.value = "";
            if (files?.length) void uploadFiles(files);
          }}
        />
        <button
          type="button"
          onClick={() => fileInputRef.current?.click()}
          disabled={busy}
          aria-label="Attach file"
          title="Attach file"
          className="relative flex items-center justify-center outline-none focus-visible:ring-2 focus-visible:ring-mycel-accent before:absolute before:-inset-1 before:content-['']"
          style={{
            width: 28,
            height: 28,
            borderRadius: 6,
            color: "var(--mycel-muted)",
            background: "none",
            border: "none",
            cursor: busy ? "wait" : "pointer",
            flexShrink: 0,
          }}
        >
          {uploading ? (
            <span
              className="inline-block w-3 h-3 border border-current border-t-transparent rounded-full animate-spin"
              aria-hidden
            />
          ) : (
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
              <path d="M21.44 11.05l-9.19 9.19a6 6 0 01-8.49-8.49l9.19-9.19a4 4 0 015.66 5.66l-9.2 9.19a2 2 0 01-2.83-2.83l8.49-8.48" />
            </svg>
          )}
        </button>
        <textarea
          ref={textareaRef}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
              e.preventDefault();
              void handleSend();
            }
          }}
          rows={1}
          disabled={sending}
          placeholder={dragOver ? "Drop to attach" : `Message #${channelLabel}`}
          aria-label="Message"
          style={{
            flex: 1,
            minHeight: 28,
            maxHeight: 120,
            resize: "none",
            background: "none",
            border: "none",
            outline: "none",
            color: "var(--mycel-text)",
            fontSize: 13,
            lineHeight: "20px",
            padding: "4px 0",
            fontFamily: "inherit",
          }}
        />
        <button
          type="button"
          onClick={() => void handleSend()}
          disabled={!canSend}
          aria-label="Send"
          title="Send"
          className="relative flex items-center justify-center outline-none focus-visible:ring-2 focus-visible:ring-mycel-accent before:absolute before:-inset-1 before:content-['']"
          style={{
            width: 28,
            height: 28,
            borderRadius: 6,
            color: canSend ? "var(--mycel-accent)" : "var(--mycel-muted)",
            background: "none",
            border: "none",
            cursor: canSend ? "pointer" : "default",
            flexShrink: 0,
            opacity: canSend ? 1 : 0.5,
          }}
        >
          {sending ? (
            <span
              className="inline-block w-3 h-3 border border-current border-t-transparent rounded-full animate-spin"
              aria-hidden
            />
          ) : (
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
              <line x1="22" y1="2" x2="11" y2="13" />
              <polygon points="22 2 15 22 11 13 2 9 22 2" />
            </svg>
          )}
        </button>
      </div>
    </div>
  );
}
