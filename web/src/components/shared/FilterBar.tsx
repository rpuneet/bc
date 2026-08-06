import {
  forwardRef,
  useEffect,
  useRef,
  type InputHTMLAttributes,
  type ReactNode,
} from "react";

/** Shared search field used in Agents / Apps / Marketplace header rows. */
export const LIST_SEARCH_CLS =
  "flex-1 min-w-[96px] max-w-md h-9 px-3 text-sm rounded-md border border-mycel-border bg-mycel-surface text-mycel-text placeholder:text-mycel-muted focus:outline-none focus:ring-1 focus:ring-mycel-accent";

export const FILTER_CHIP_CLS =
  "inline-flex items-center gap-1.5 h-8 px-2.5 rounded-md border text-xs font-medium transition-colors";

export const FILTER_CHIP_ACTIVE =
  "border-mycel-accent text-mycel-text bg-mycel-surface";

export const FILTER_CHIP_IDLE =
  "border-mycel-border bg-mycel-surface text-mycel-muted hover:text-mycel-text hover:border-mycel-accent";

export const FILTER_POPOVER_CLS =
  "absolute right-0 top-full mt-1.5 z-50 w-60 rounded-lg border border-mycel-border bg-mycel-surface-2 shadow-mycel-lg p-3 space-y-2.5 text-sm";

export const FILTER_LABEL_CLS =
  "block mb-1 text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted";

export const FILTER_SELECT_CLS =
  "w-full px-2 py-1.5 text-sm rounded-md border border-mycel-border bg-mycel-bg text-mycel-text focus:outline-none focus:ring-1 focus:ring-mycel-accent";

export const FILTER_CLEAR_CLS =
  "ml-auto px-2 py-1.5 text-xs text-mycel-muted hover:text-mycel-text border border-mycel-border rounded-md focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg";

function FilterIcon() {
  return (
    <svg
      width="12"
      height="12"
      viewBox="0 0 14 14"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.4"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <path d="M1.5 2.5h11l-4.2 5v4l-2.6-1.5V7.5z" />
    </svg>
  );
}

export const ListSearchInput = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(
  function ListSearchInput({ className = "", type = "search", ...rest }, ref) {
    return (
      <input ref={ref} type={type} className={`${LIST_SEARCH_CLS} ${className}`.trim()} {...rest} />
    );
  },
);

/**
 * Filters chip + popover shell shared by Agents and Apps.
 * Caller owns open state and popover body contents.
 */
export function FiltersChip({
  open,
  onOpenChange,
  activeCount = 0,
  children,
  testId,
  label = "Filters",
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  activeCount?: number;
  children: ReactNode;
  testId?: string;
  label?: string;
}) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onMouse = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onOpenChange(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onOpenChange(false);
    };
    document.addEventListener("mousedown", onMouse);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onMouse);
      document.removeEventListener("keydown", onKey);
    };
  }, [open, onOpenChange]);

  return (
    <div className="relative shrink-0" ref={ref}>
      <button
        type="button"
        onClick={() => onOpenChange(!open)}
        aria-label={label}
        aria-haspopup="true"
        aria-expanded={open}
        className={`${FILTER_CHIP_CLS} ${open || activeCount > 0 ? FILTER_CHIP_ACTIVE : FILTER_CHIP_IDLE}`}
      >
        <FilterIcon />
        {label}
        {activeCount > 0 && (
          <span className="inline-flex items-center justify-center min-w-[16px] h-4 px-1 rounded-full bg-mycel-accent text-mycel-accent-fg text-[10px] font-semibold tabular-nums">
            {activeCount}
          </span>
        )}
      </button>
      {open && (
        <div data-testid={testId} className={FILTER_POPOVER_CLS}>
          {children}
        </div>
      )}
    </div>
  );
}

/** Inline filter select button style used by Marketplace type/source chips. */
export const FILTER_INLINE_BTN_CLS =
  "flex items-center gap-1.5 px-2 py-1.5 text-sm rounded-md border border-mycel-border bg-mycel-bg text-mycel-text focus:outline-none focus:ring-1 focus:ring-mycel-accent hover:border-mycel-border-strong transition-colors whitespace-nowrap";
