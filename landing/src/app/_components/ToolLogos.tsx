"use client";

interface Tool {
  name: string;
  url: string;
  logo: string;
}

const TOOLS: Tool[] = [
  {
    name: "Claude Code",
    url: "https://docs.anthropic.com/en/docs/claude-code",
    logo: "https://www.google.com/s2/favicons?domain=claude.ai&sz=128",
  },
  {
    name: "Cursor",
    url: "https://cursor.com",
    logo: "https://cursor.com/apple-touch-icon.png",
  },
  {
    name: "Codex",
    url: "https://openai.com/codex",
    logo: "https://www.google.com/s2/favicons?domain=openai.com&sz=128",
  },
  {
    name: "Gemini",
    url: "https://gemini.google.com",
    logo: "https://www.gstatic.com/lamda/images/gemini_sparkle_v002_d4735304ff6292a690345.svg",
  },
];

function ToolChip({ tool }: { tool: Tool }) {
  const isExternal = tool.url.startsWith("http");

  return (
    <a
      href={tool.url}
      target={isExternal ? "_blank" : undefined}
      rel={isExternal ? "noopener noreferrer" : undefined}
      className="group inline-flex items-center gap-3 transition-opacity duration-200 opacity-70 hover:opacity-100"
    >
      <div className="h-10 w-10 shrink-0 rounded-xl overflow-hidden flex items-center justify-center">
        {tool.logo ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={tool.logo}
            alt=""
            width={40}
            height={40}
            className="h-10 w-10 object-contain"
            loading="lazy"
          />
        ) : (
          <div className="h-10 w-10 rounded-xl bg-border/40 flex items-center justify-center">
            <svg
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth={2}
              className="h-5 w-5 text-muted-foreground/60"
            >
              <path d="M12 5v14M5 12h14" />
            </svg>
          </div>
        )}
      </div>
      <span className="text-sm font-medium text-foreground/60 group-hover:text-foreground whitespace-nowrap transition-colors duration-200">
        {tool.name}
      </span>
    </a>
  );
}

export function ToolMarquee() {
  return (
    <div className="py-4">
      <div className="flex items-center justify-center gap-12 sm:gap-16">
        {TOOLS.map((tool) => (
          <ToolChip key={tool.name} tool={tool} />
        ))}
      </div>
    </div>
  );
}

/** Backward compat export */
export const TOOL_ICONS: Record<string, () => React.ReactElement> = {};
