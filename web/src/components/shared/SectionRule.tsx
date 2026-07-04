import { MONO } from "../../utils/typography";

/**
 * Horizontal rule with a label and optional trailing content.
 *
 * Accepts either `label` (string) or `children` (ReactNode) as the label text,
 * plus an optional `trailing` slot for Live indicators, action buttons, etc.
 */
export function SectionRule({
  label,
  children,
  trailing,
}: {
  label?: string;
  children?: React.ReactNode;
  trailing?: React.ReactNode;
}) {
  const text = label ?? children;
  return (
    <div className="flex items-center gap-3 py-2">
      {text && (
        <span
          className="text-[10px] font-semibold text-mycel-muted uppercase tracking-widest whitespace-nowrap"
          style={{ fontFamily: MONO }}
        >
          {text}
        </span>
      )}
      <div className="flex-1 h-px bg-gradient-to-r from-mycel-border to-transparent" />
      {trailing && <div className="flex items-center gap-2">{trailing}</div>}
    </div>
  );
}
