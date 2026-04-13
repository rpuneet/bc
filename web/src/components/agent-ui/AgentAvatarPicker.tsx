import { AVATAR_COLORS } from "./utils/colorFromName";

interface AgentAvatarPickerProps {
  variant: string;
  color: number;
  onVariantChange: (v: string) => void;
  onColorChange: (c: number) => void;
}

const VARIANTS = ["geometric", "organic", "monogram"] as const;

const VARIANT_LABELS: Record<string, string> = {
  geometric: "Hex",
  organic: "Blob",
  monogram: "Letter",
};

export function AgentAvatarPicker({
  variant,
  color,
  onVariantChange,
  onColorChange,
}: AgentAvatarPickerProps) {
  return (
    <div className="flex flex-col gap-3">
      <div className="flex gap-1.5">
        {VARIANTS.map((v) => (
          <button
            key={v}
            type="button"
            onClick={() => onVariantChange(v)}
            className={`px-2.5 py-1 text-xs rounded font-medium transition-colors ${
              variant === v
                ? "bg-bc-accent text-white"
                : "bg-bc-surface text-bc-muted hover:text-bc-text"
            }`}
          >
            {VARIANT_LABELS[v]}
          </button>
        ))}
      </div>
      <div className="flex gap-2">
        {AVATAR_COLORS.map((hex, i) => (
          <button
            key={hex}
            type="button"
            onClick={() => onColorChange(i)}
            className={`w-6 h-6 rounded-full transition-all ${
              color === i ? "ring-2 ring-bc-accent ring-offset-2 ring-offset-bc-bg scale-110" : "hover:scale-105"
            }`}
            style={{ backgroundColor: hex }}
            aria-label={`Color ${i + 1}`}
          />
        ))}
      </div>
    </div>
  );
}
