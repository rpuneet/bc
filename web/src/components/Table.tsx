import React from "react";
import { EmptyState } from "./EmptyState";

interface Column<T> {
  key: string;
  label: string;
  render: (row: T) => React.ReactNode;
  className?: string;
}

interface TableProps<T> {
  columns: Column<T>[];
  data: T[];
  keyFn: (row: T) => string;
  onRowClick?: (row: T) => void;
  emptyMessage?: string;
  emptyIcon?: string;
  emptyDescription?: string;
  renderRowExpansion?: (row: T) => React.ReactNode | null;
}

export function Table<T>({
  columns,
  data,
  keyFn,
  onRowClick,
  emptyMessage = "No data",
  emptyIcon,
  emptyDescription,
  renderRowExpansion,
}: TableProps<T>) {
  if (!data || data.length === 0) {
    return (
      <EmptyState
        icon={emptyIcon}
        title={emptyMessage}
        description={emptyDescription}
      />
    );
  }

  return (
    <table className="w-full text-sm">
      <thead>
        <tr className="border-b border-mycel-border text-left">
          {columns.map((col) => (
            <th
              key={col.key}
              className={`px-4 py-2 font-medium text-mycel-muted ${col.className ?? ""}`}
            >
              {col.label}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {data.map((row) => {
          const expansion = renderRowExpansion ? renderRowExpansion(row) : null;
          return (
            <React.Fragment key={keyFn(row)}>
              <tr
                onClick={onRowClick ? () => onRowClick(row) : undefined}
                className={`border-b border-mycel-border ${
                  onRowClick ? "cursor-pointer hover:bg-mycel-surface" : ""
                }`}
              >
                {columns.map((col) => (
                  <td
                    key={col.key}
                    className={`px-4 py-2 ${col.className ?? ""}`}
                  >
                    {col.render(row)}
                  </td>
                ))}
              </tr>
              {expansion && (
                <tr>
                  <td colSpan={columns.length} className="p-0">
                    {expansion}
                  </td>
                </tr>
              )}
            </React.Fragment>
          );
        })}
      </tbody>
    </table>
  );
}
