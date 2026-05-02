import { MONO } from "../../utils/typography";

export function Chip({
  label,
  color = "accent",
  onRemove,
  disabled,
}: {
  label: string;
  color?: "accent" | "muted" | "yellow" | "green";
  onRemove?: () => void;
  disabled?: boolean;
}) {
  const palette: Record<string, string> = {
    accent: "bg-mycel-accent/10 text-mycel-accent/80",
    muted: "bg-mycel-muted/10 text-mycel-muted/70",
    yellow: "bg-yellow-500/10 text-yellow-400",
    green: "bg-green-500/10 text-green-400",
  };
  return (
    <span
      className={`inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded text-[10px] font-medium ${palette[color] ?? palette.muted}`}
      style={{ fontFamily: MONO }}
    >
      {label}
      {onRemove && (
        <button
          type="button"
          onClick={onRemove}
          disabled={disabled}
          className="ml-0.5 opacity-60 hover:opacity-100 transition-opacity disabled:opacity-30 leading-none"
          aria-label={`Remove ${label}`}
        >
          ×
        </button>
      )}
    </span>
  );
}

export function ChipList({
  items,
  color,
  empty = "\u2014",
  onRemove,
}: {
  items: string[];
  color?: "accent" | "muted" | "yellow" | "green";
  empty?: string;
  onRemove?: (item: string) => void;
}) {
  if (!items || items.length === 0) {
    return <span className="text-xs text-mycel-muted/30">{empty}</span>;
  }
  return (
    <div className="flex flex-wrap gap-1">
      {items.map((v) => (
        <Chip
          key={v}
          label={v}
          color={color}
          onRemove={onRemove ? () => onRemove(v) : undefined}
        />
      ))}
    </div>
  );
}
