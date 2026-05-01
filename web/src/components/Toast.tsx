import { useCallback, useEffect, useState } from "react";

export type ToastLevel = "error" | "success" | "info";

export interface ToastMessage {
  id: number;
  level: ToastLevel;
  text: string;
}

let nextId = 1;

const LEVEL_STYLES: Record<ToastLevel, string> = {
  error:   "bg-mycel-error/90 text-white",
  success: "bg-mycel-success/90 text-white",
  info:    "bg-mycel-accent/90 text-white",
};

function ToastItem({ toast, onDismiss }: { toast: ToastMessage; onDismiss: (id: number) => void }) {
  useEffect(() => {
    const timer = setTimeout(() => onDismiss(toast.id), 5000);
    return () => clearTimeout(timer);
  }, [toast.id, onDismiss]);

  return (
    <div
      role="alert"
      className={`flex items-center gap-2 px-3 py-2 rounded shadow-lg text-sm max-w-sm animate-slide-in ${LEVEL_STYLES[toast.level]}`}
    >
      <span className="flex-1 break-words">{toast.text}</span>
      <button
        type="button"
        onClick={() => onDismiss(toast.id)}
        className="shrink-0 opacity-70 hover:opacity-100 text-xs font-bold focus-visible:ring-2 focus-visible:ring-white/50 rounded px-1"
        aria-label="Dismiss notification"
      >
        &times;
      </button>
    </div>
  );
}

export function ToastContainer({ toasts, onDismiss }: { toasts: ToastMessage[]; onDismiss: (id: number) => void }) {
  if (toasts.length === 0) return null;
  return (
    <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2" aria-live="polite">
      {toasts.map((t) => (
        <ToastItem key={t.id} toast={t} onDismiss={onDismiss} />
      ))}
    </div>
  );
}

/** Hook for managing toast state. */
export function useToast() {
  const [toasts, setToasts] = useState<ToastMessage[]>([]);

  const addToast = useCallback((level: ToastLevel, text: string) => {
    setToasts((prev) => [...prev, { id: nextId++, level, text }]);
  }, []);

  const dismiss = useCallback((id: number) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  return { toasts, addToast, dismiss };
}
