/**
 * Reusable search input with magnifying glass icon and clear button.
 */
export function SearchInput({
  value,
  onChange,
  placeholder = "Search...",
  className,
}: {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  className?: string;
}) {
  return (
    <div className={`relative flex items-center ${className ?? ""}`}>
      {/* Magnifying glass icon */}
      <svg
        className="absolute left-2.5 w-3.5 h-3.5 text-mycel-muted pointer-events-none"
        viewBox="0 0 16 16"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        aria-hidden="true"
      >
        <circle cx="6.5" cy="6.5" r="4.5" />
        <path d="M10.5 10.5l3 3" strokeLinecap="round" />
      </svg>

      <input
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        aria-label={placeholder}
        className="w-full pl-8 pr-7 py-1.5 rounded-md border border-mycel-border bg-mycel-surface text-sm text-mycel-text placeholder:text-mycel-muted focus:outline-none focus:border-mycel-accent transition-colors"
      />

      {/* Clear button */}
      {value && (
        <button
          type="button"
          onClick={() => onChange("")}
          className="absolute right-2 text-mycel-muted hover:text-mycel-muted transition-colors leading-none"
          aria-label="Clear search"
        >
          <svg
            className="w-3 h-3"
            viewBox="0 0 12 12"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
            strokeLinecap="round"
            aria-hidden="true"
          >
            <path d="M2 2l8 8M10 2l-8 8" />
          </svg>
        </button>
      )}
    </div>
  );
}
