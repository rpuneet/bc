import { useState, useCallback } from "react";

interface CopyButtonProps {
  text: string;
  className?: string;
}

export function CopyButton({ text, className = "" }: CopyButtonProps) {
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // Fallback: noop
    }
  }, [text]);

  return (
    <button
      type="button"
      onClick={(e) => {
        e.stopPropagation();
        void handleCopy();
      }}
      className={`inline-flex items-center justify-center w-6 h-6 rounded-md transition-colors hover:bg-mycel-surface-hover focus-visible:ring-2 focus-visible:ring-mycel-accent ${className}`}
      aria-label={copied ? "Copied" : "Copy to clipboard"}
    >
      {copied ? (
        <svg
          className="w-3.5 h-3.5 text-mycel-success"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          strokeWidth={2.5}
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
        </svg>
      ) : (
        <svg
          className="w-3.5 h-3.5 text-mycel-muted hover:text-mycel-text"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          strokeWidth={2}
        >
          <rect x="9" y="9" width="13" height="13" rx="2" />
          <path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1" />
        </svg>
      )}
    </button>
  );
}
