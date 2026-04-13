const MONO =
  "'JetBrains Mono', 'Fira Code', 'Space Mono', ui-monospace, SFMono-Regular, Menlo, Consolas, monospace";

interface Template {
  name: string;
  description: string;
  provider: string;
  mcps: string[];
}

const MOCK_TEMPLATES: Template[] = [
  {
    name: "feature-dev",
    description: "Full-stack feature development",
    provider: "claude",
    mcps: ["bc", "github"],
  },
  {
    name: "reviewer",
    description: "Code review specialist",
    provider: "claude",
    mcps: ["bc"],
  },
  {
    name: "manager",
    description: "Task orchestration and delegation",
    provider: "gemini",
    mcps: ["bc"],
  },
  {
    name: "blank",
    description: "Empty starting point",
    provider: "claude",
    mcps: ["bc"],
  },
];

function McpChip({ name }: { name: string }) {
  return (
    <span
      className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium bg-bc-accent/10 text-bc-accent/70"
      style={{ fontFamily: MONO }}
    >
      {name}
    </span>
  );
}

function TemplateRow({ template }: { template: Template }) {
  return (
    <tr
      className="border-t border-bc-border/40 hover:bg-bc-surface/60 transition-colors cursor-pointer"
    >
      <td
        className="py-3 pl-4 pr-6 text-sm font-medium text-bc-text whitespace-nowrap"
        style={{ fontFamily: MONO }}
      >
        {template.name}
      </td>
      <td
        className="py-3 px-4 text-xs text-bc-muted whitespace-nowrap"
        style={{ fontFamily: MONO }}
      >
        {template.provider}
      </td>
      <td className="py-3 px-4">
        <div className="flex flex-wrap gap-1">
          {template.mcps.map((mcp) => (
            <McpChip key={mcp} name={mcp} />
          ))}
        </div>
      </td>
      <td className="py-3 px-4 text-sm text-bc-muted">{template.description}</td>
    </tr>
  );
}

export function Templates() {
  return (
    <div className="p-6 space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h1 className="text-xl font-bold text-bc-text">Templates</h1>
          <span className="text-sm text-bc-muted">
            {MOCK_TEMPLATES.length}
          </span>
        </div>
        <button
          type="button"
          className="px-4 py-2 rounded bg-bc-accent text-white text-sm font-medium hover:opacity-90 transition-opacity focus-visible:ring-2 focus-visible:ring-bc-accent focus-visible:ring-offset-1 focus-visible:ring-offset-bc-bg"
        >
          + Create Template
        </button>
      </div>

      {/* Table */}
      <div className="rounded border border-bc-border bg-bc-surface overflow-hidden">
        <table className="w-full">
          <thead>
            <tr className="text-left text-[11px] font-medium text-bc-muted/60 uppercase tracking-wide">
              <th className="py-2.5 pl-4 pr-6 font-medium">Name</th>
              <th className="py-2.5 px-4 font-medium">Provider</th>
              <th className="py-2.5 px-4 font-medium">MCPs</th>
              <th className="py-2.5 px-4 font-medium">Description</th>
            </tr>
          </thead>
          <tbody>
            {MOCK_TEMPLATES.map((t) => (
              <TemplateRow key={t.name} template={t} />
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
