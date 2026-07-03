import { useEffect, useRef, useState, useCallback } from "react";
import { useCommandPalette } from "../hooks/useCommandPalette";

export function CommandPalette() {
  const { open, query, setQuery, close, filtered } = useCommandPalette();
  const [activeIndex, setActiveIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  // Reset active index when filtered results change
  useEffect(() => {
    setActiveIndex(0);
  }, [filtered]);

  // Focus input when opened
  useEffect(() => {
    if (open) {
      setActiveIndex(0);
      // Slight delay to ensure the DOM is rendered
      requestAnimationFrame(() => inputRef.current?.focus());
    }
  }, [open]);

  // Scroll active item into view
  useEffect(() => {
    if (!listRef.current) return;
    const active = listRef.current.querySelector("[data-active='true']");
    active?.scrollIntoView({ block: "nearest" });
  }, [activeIndex]);

  const handleSelect = useCallback(
    (index: number) => {
      const item = filtered[index];
      if (item) {
        item.action();
        close();
      }
    },
    [filtered, close],
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      switch (e.key) {
        case "ArrowDown":
          e.preventDefault();
          setActiveIndex((prev) => (prev < filtered.length - 1 ? prev + 1 : 0));
          break;
        case "ArrowUp":
          e.preventDefault();
          setActiveIndex((prev) => (prev > 0 ? prev - 1 : filtered.length - 1));
          break;
        case "Enter":
          e.preventDefault();
          handleSelect(activeIndex);
          break;
        case "Escape":
          e.preventDefault();
          close();
          break;
      }
    },
    [activeIndex, filtered.length, handleSelect, close],
  );

  if (!open) return null;

  // Group items by section
  const sections = new Map<string, typeof filtered>();
  for (const item of filtered) {
    const group = sections.get(item.section) ?? [];
    group.push(item);
    sections.set(item.section, group);
  }

  // Build flat index mapping for sections
  let flatIndex = 0;

  return (
    <div
      className="fixed inset-0 z-[200] flex items-start justify-center pt-[15vh]"
      onClick={close}
      role="presentation"
    >
      {/* Backdrop */}
      <div className="absolute inset-0 bg-mycel-overlay" />

      {/* Palette */}
      <div
        className="relative w-full max-w-lg rounded-lg border border-mycel-border bg-mycel-surface shadow-2xl"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label="Command palette"
      >
        {/* Search input */}
        <div className="flex items-center gap-2 border-b border-mycel-border px-4 py-3">
          <svg
            width="16"
            height="16"
            viewBox="0 0 16 16"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            className="shrink-0 text-mycel-muted"
          >
            <circle cx="6.5" cy="6.5" r="4.5" />
            <path d="M10 10l4 4" />
          </svg>
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Type a command..."
            className="flex-1 bg-transparent text-sm text-mycel-text placeholder:text-mycel-muted outline-none"
            aria-label="Search commands"
          />
          <kbd className="hidden sm:inline-block rounded border border-mycel-border px-1.5 py-0.5 text-[10px] text-mycel-muted">
            ESC
          </kbd>
        </div>

        {/* Results */}
        <div ref={listRef} className="max-h-72 overflow-y-auto py-2">
          {filtered.length === 0 ? (
            <div className="px-4 py-6 text-center text-sm text-mycel-muted">
              No results found
            </div>
          ) : (
            Array.from(sections.entries()).map(([section, items]) => {
              const sectionStart = flatIndex;
              const sectionItems = items.map((item, i) => {
                const itemIndex = sectionStart + i;
                return (
                  <button
                    key={item.id}
                    type="button"
                    data-active={itemIndex === activeIndex}
                    onClick={() => handleSelect(itemIndex)}
                    onMouseEnter={() => setActiveIndex(itemIndex)}
                    className={`flex w-full items-center gap-3 px-4 py-2 text-left text-sm transition-colors ${
                      itemIndex === activeIndex
                        ? "bg-mycel-accent/10 text-mycel-accent"
                        : "text-mycel-text hover:bg-mycel-bg"
                    }`}
                  >
                    <span className="w-5 text-center font-mono text-xs text-mycel-muted">
                      {item.icon}
                    </span>
                    <span className="flex-1">{item.label}</span>
                    {item.section === "Navigate" && (
                      <span className="text-xs text-mycel-muted">Go to</span>
                    )}
                    {item.section === "Action" && (
                      <span className="text-xs text-mycel-muted">Run</span>
                    )}
                  </button>
                );
              });
              flatIndex += items.length;
              return (
                <div key={section}>
                  <div className="px-4 py-1.5 text-xs font-medium text-mycel-muted">
                    {section}
                  </div>
                  {sectionItems}
                </div>
              );
            })
          )}
        </div>

        {/* Footer hint */}
        <div className="flex items-center gap-3 border-t border-mycel-border px-4 py-2 text-[10px] text-mycel-muted">
          <span>
            <kbd className="rounded border border-mycel-border px-1 py-0.5">
              &uarr;&darr;
            </kbd>{" "}
            navigate
          </span>
          <span>
            <kbd className="rounded border border-mycel-border px-1 py-0.5">
              &crarr;
            </kbd>{" "}
            select
          </span>
          <span>
            <kbd className="rounded border border-mycel-border px-1 py-0.5">
              esc
            </kbd>{" "}
            close
          </span>
        </div>
      </div>
    </div>
  );
}
