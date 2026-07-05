interface LoadingSkeletonProps {
  rows?: number;
  variant?: "table" | "cards" | "text";
}

function SkeletonBar({
  className = "",
  style,
}: {
  className?: string;
  style?: React.CSSProperties;
}) {
  return (
    <div
      className={`animate-pulse rounded bg-mycel-border ${className}`}
      style={style}
    />
  );
}

function TableSkeleton({ rows }: { rows: number }) {
  return (
    <div className="rounded-lg border border-mycel-border overflow-hidden">
      <div className="border-b border-mycel-border bg-mycel-surface px-4 py-2 flex gap-4">
        <SkeletonBar className="h-4 w-24" />
        <SkeletonBar className="h-4 w-20" />
        <SkeletonBar className="h-4 w-16" />
        <SkeletonBar className="h-4 w-20" />
      </div>
      {Array.from({ length: rows }).map((_, i) => (
        <div
          key={i}
          className="border-b border-mycel-border px-4 py-3 flex gap-4"
        >
          <SkeletonBar className="h-3 w-28" />
          <SkeletonBar className="h-3 w-20" />
          <SkeletonBar className="h-3 w-16" />
          <SkeletonBar className="h-3 w-14" />
        </div>
      ))}
    </div>
  );
}

function CardsSkeleton({ rows }: { rows: number }) {
  return (
    <div className="grid grid-cols-3 gap-4">
      {Array.from({ length: rows }).map((_, i) => (
        <div
          key={i}
          className="rounded-lg border border-mycel-border bg-mycel-surface p-4 space-y-2"
        >
          <SkeletonBar className="h-3 w-16" />
          <SkeletonBar className="h-6 w-24" />
        </div>
      ))}
    </div>
  );
}

function TextSkeleton({ rows }: { rows: number }) {
  return (
    <div className="space-y-3">
      {Array.from({ length: rows }).map((_, i) => (
        <SkeletonBar
          key={i}
          className="h-4"
          style={{ width: `${70 + (i % 3) * 10}%` }}
        />
      ))}
    </div>
  );
}

export function LoadingSkeleton({
  rows = 4,
  variant = "table",
}: LoadingSkeletonProps) {
  switch (variant) {
    case "cards":
      return <CardsSkeleton rows={rows} />;
    case "text":
      return <TextSkeleton rows={rows} />;
    default:
      return <TableSkeleton rows={rows} />;
  }
}
