import { useCallback, useEffect, useState } from "react";
import { motion, AnimatePresence } from "framer-motion";

export type ToastLevel = "error" | "success" | "info";

export interface ToastMessage {
  id: number;
  level: ToastLevel;
  text: string;
}

let nextId = 1;

// Pill-shaped surface with a tinted left accent rail so the level reads
// at a glance without painting the whole toast a single brand color.
const LEVEL_STYLES: Record<ToastLevel, { surface: string; rail: string; icon: string }> = {
  error: {
    surface: "bg-mycel-surface-2 text-mycel-text border border-mycel-border ring-1 ring-mycel-error-subtle",
    rail: "bg-mycel-error",
    icon: "text-mycel-error",
  },
  success: {
    surface: "bg-mycel-surface-2 text-mycel-text border border-mycel-border ring-1 ring-mycel-success-subtle",
    rail: "bg-mycel-success",
    icon: "text-mycel-success",
  },
  info: {
    surface: "bg-mycel-surface-2 text-mycel-text border border-mycel-border ring-1 ring-mycel-accent-subtle",
    rail: "bg-mycel-accent",
    icon: "text-mycel-accent",
  },
};

function LevelIcon({ level, className }: { level: ToastLevel; className: string }) {
  const common = { width: 14, height: 14, viewBox: "0 0 14 14", fill: "none", stroke: "currentColor", strokeWidth: 1.6, strokeLinecap: "round" as const, strokeLinejoin: "round" as const, className };
  if (level === "success") return <svg {...common}><path d="M2.5 7.5l3 3 6-7" /></svg>;
  if (level === "error") return <svg {...common}><circle cx="7" cy="7" r="5.5" /><path d="M7 4.5v3M7 10v.01" /></svg>;
  return <svg {...common}><circle cx="7" cy="7" r="5.5" /><path d="M7 9.5V6.5M7 4.5v.01" /></svg>;
}

function ToastItem({ toast, onDismiss }: { toast: ToastMessage; onDismiss: (id: number) => void }) {
  useEffect(() => {
    const timer = setTimeout(() => onDismiss(toast.id), 5000);
    return () => clearTimeout(timer);
  }, [toast.id, onDismiss]);

  const style = LEVEL_STYLES[toast.level];

  return (
    <motion.div
      role="alert"
      initial={{ opacity: 0, y: 12, scale: 0.96 }}
      animate={{ opacity: 1, y: 0, scale: 1 }}
      exit={{ opacity: 0, y: -8, scale: 0.96, transition: { duration: 0.15 } }}
      transition={{ duration: 0.2, ease: [0.4, 0, 0.2, 1] }}
      layout
      className={`relative flex items-center gap-2.5 pl-3.5 pr-2 py-2 rounded-lg shadow-mycel-lg text-sm max-w-sm overflow-hidden ${style.surface}`}
    >
      <span className={`absolute left-0 top-0 bottom-0 w-1 ${style.rail}`} aria-hidden />
      <LevelIcon level={toast.level} className={`shrink-0 ${style.icon}`} />
      <span className="flex-1 break-words leading-snug">{toast.text}</span>
      <button
        type="button"
        onClick={() => onDismiss(toast.id)}
        className="shrink-0 opacity-60 hover:opacity-100 text-mycel-muted hover:text-mycel-text rounded-md p-0.5 focus-visible:ring-2 focus-visible:ring-mycel-accent transition-opacity"
        aria-label="Dismiss notification"
      >
        <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round"><path d="M3 3l6 6M9 3l-6 6" /></svg>
      </button>
    </motion.div>
  );
}

export function ToastContainer({ toasts, onDismiss }: { toasts: ToastMessage[]; onDismiss: (id: number) => void }) {
  return (
    <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2 pointer-events-none" aria-live="polite">
      <AnimatePresence initial={false}>
        {toasts.map((t) => (
          <div key={t.id} className="pointer-events-auto">
            <ToastItem toast={t} onDismiss={onDismiss} />
          </div>
        ))}
      </AnimatePresence>
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
